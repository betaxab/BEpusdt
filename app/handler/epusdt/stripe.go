package epusdt

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/callback"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/stripe"
)

// StripeNotify 接收 Stripe 异步支付回调，并交给后台任务处理验签和订单状态推进。
func (Epusdt) StripeNotify(ctx *gin.Context) {
	callback.HandleStripeNotify(ctx)
}

// StripeReturn 接收 Stripe 浏览器同步成功回跳，再跳转到订单原始 ReturnUrl。
func (Epusdt) StripeReturn(ctx *gin.Context) {
	redirectStripeOrder(ctx, false)
}

// StripeCancel 接收 Stripe 浏览器取消回跳，再跳转到订单原始 ReturnUrl。
func (Epusdt) StripeCancel(ctx *gin.Context) {
	redirectStripeOrder(ctx, true)
}

// ensureStripePayment 为待支付的 Stripe 订单创建 Checkout Session。
// 创建成功后将结算链接、通道标识和 Stripe 会话凭据写回订单。
func (Epusdt) ensureStripePayment(ctx *gin.Context, order model.Order) (model.Order, error) {
	if !model.IsStripeTradeType(order.TradeType) || order.Status != model.OrderStatusWaiting {
		return order, nil
	}

	channel, config, err := findStripeOrderChannel(order)
	if err != nil {
		return order, err
	}

	successURL := strings.TrimSpace(config.SuccessURL)
	if successURL == "" {
		successURL = buildStripeReturnURL(ctx, order, false)
	}
	cancelURL := strings.TrimSpace(config.CancelURL)
	if cancelURL == "" {
		cancelURL = buildStripeReturnURL(ctx, order, true)
	}

	createConfig := *config
	if order.TradeType == model.StripeAll {
		createConfig.PaymentMethodTypes = nil
	}
	result, err := stripe.CreatePayment(ctx.Request.Context(), &createConfig, stripe.CreateInput{
		OrderNo:            order.TradeId,
		MerchantOrderID:    order.OrderId,
		Amount:             order.Money,
		Currency:           string(order.Fiat),
		Description:        order.Name,
		SuccessURL:         successURL,
		CancelURL:          cancelURL,
		PaymentMethodTypes: stripePaymentMethodTypes(order.TradeType),
	})
	if err != nil {
		log.Warn(fmt.Sprintf("Stripe: payment create failed for order %s channel %s amount %s: %v", order.TradeId, channel.Name, order.Money, err))
		return order, err
	}

	order.QrcodeURL = result.URL
	order.Address = channel.MatchQr
	order.RefFrom = channel.MatchQr
	order.RefOrderNo = result.SessionID
	if strings.TrimSpace(result.PaymentIntentID) != "" {
		order.RefHash = result.PaymentIntentID
	}
	if err := model.Db.Save(&order).Error; err != nil {
		return order, err
	}

	log.Info(fmt.Sprintf("Stripe: checkout session created for order %s by channel %s session %s", order.TradeId, channel.Name, result.SessionID))
	return order, nil
}

// stripePaymentMethodTypes 根据交易类型返回 Stripe Checkout 限定的付款方式。
// stripe.all 返回空列表，由 Stripe 根据账户配置展示可用付款方式。
func stripePaymentMethodTypes(tradeType model.TradeType) []string {
	switch tradeType {
	case model.StripeAlipay:
		return []string{stripe.MethodAlipay}
	case model.StripeWechatPay:
		return []string{stripe.MethodWechatPay}
	case model.StripeCard:
		return []string{stripe.MethodCard}
	default:
		return nil
	}
}

// buildStripeReturnURL 构造 Stripe 完成付款或取消付款后的本地桥接地址。
func buildStripeReturnURL(ctx *gin.Context, order model.Order, canceled bool) string {
	path := "/api/v1/pay/stripe/return/"
	if canceled {
		path = "/api/v1/pay/stripe/cancel/"
	}
	return strings.TrimRight(requestHost(ctx), "/") + path + url.PathEscape(strings.TrimSpace(order.TradeId))
}

// redirectStripeOrder 根据交易编号加载订单，并将浏览器回跳到订单原始 ReturnUrl。
// 回跳参数用于商户区分 Stripe 正常返回与用户取消，不作为支付成功依据。
func redirectStripeOrder(ctx *gin.Context, canceled bool) {
	tradeID := strings.TrimSpace(ctx.Param("trade_id"))
	if tradeID == "" {
		ctx.String(http.StatusBadRequest, "trade_id is required")
		return
	}

	order, ok := model.GetTradeOrder(tradeID)
	if !ok {
		ctx.String(http.StatusNotFound, "order not found")
		return
	}
	redirect := order.RedirectUrl()
	if redirect == "" {
		ctx.String(http.StatusOK, "ok")
		return
	}
	params := url.Values{}
	if canceled {
		params.Set("stripe_cancel", "1")
	} else {
		params.Set("stripe_return", "1")
	}
	params.Set("trade_id", order.TradeId)
	sep := "?"
	if strings.Contains(redirect, "?") {
		sep = "&"
	}
	ctx.Redirect(http.StatusFound, redirect+sep+params.Encode())
}

// findStripeOrderChannel 根据订单交易类型和地址标识匹配启用的 Stripe 通道配置。
// 无效或不完整的通道配置会被跳过，直到找到与订单绑定通道一致的配置。
func findStripeOrderChannel(order model.Order) (model.Channel, *stripe.Config, error) {
	channels, err := listStripeChannels(order.TradeType)
	if err != nil {
		return model.Channel{}, nil, err
	}
	for _, channel := range channels {
		config, err := stripe.ParseConfigText(channel.Config)
		if err != nil {
			log.Warn(fmt.Sprintf("Stripe: invalid config for channel %s: %v", channel.Name, err))
			continue
		}
		if err := stripe.ValidateConfig(config); err != nil {
			log.Warn(fmt.Sprintf("Stripe: invalid config for channel %s: %v", channel.Name, err))
			continue
		}
		if strings.TrimSpace(channel.MatchQr) == strings.TrimSpace(order.Address) {
			return channel, config, nil
		}
	}
	return model.Channel{}, nil, fmt.Errorf("no enabled stripe channel matches order %s", order.TradeId)
}

// listStripeChannels 返回指定 Stripe 交易类型下所有启用的支付通道。
func listStripeChannels(tradeType model.TradeType) ([]model.Channel, error) {
	if !model.IsStripeTradeType(tradeType) {
		return nil, errors.New("stripe trade type is invalid")
	}
	channels := make([]model.Channel, 0)
	err := model.Db.Where("trade_type = ? and status = ?", tradeType, model.ClStatusEnable).Find(&channels).Error
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, errors.New("enabled stripe channel not found")
	}
	return channels, nil
}
