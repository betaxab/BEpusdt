package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/callback"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/stripe"
	"github.com/v03413/bepusdt/app/task/notify"
)

// stripeTask 负责 Stripe Webhook 处理、超时订单清理和确认状态推进。
type stripeTask struct{}

var stripeRunner stripeTask

// stripeInit 注册 Stripe Webhook 处理器和后台定时任务。
func stripeInit() {
	stripeRunner = stripeTask{}
	callback.RegisterStripeNotify(stripeRunner.notify)
	Register(Task{
		Duration: 30 * time.Second,
		Callback: stripeRunner.expireWaitingOrders,
	})
	Register(Task{
		Duration: 5 * time.Second,
		Callback: stripeRunner.tradeConfirmHandle,
	})
}

// notify 处理 Stripe 异步支付通知。
// 流程包括读取请求体、匹配通道验签、校验订单与金额，并根据事件状态推进本地订单。
func (s stripeTask) notify(ctx *gin.Context) {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.Warn("StripeNotify: read body failed:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}

	result, err := s.parseWebhook(ctx.Request.Header, body)
	if err != nil {
		log.Warn("StripeNotify: parse webhook failed:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}

	order, ok := model.GetTradeOrder(result.OrderNo)
	if !ok {
		log.Warn(fmt.Sprintf("StripeNotify: order not found: %s", result.OrderNo))
		ctx.String(http.StatusOK, "fail")
		return
	}
	if !model.IsStripeTradeType(order.TradeType) {
		log.Warn(fmt.Sprintf("StripeNotify: order %s trade_type mismatch: %s", order.TradeId, order.TradeType))
		ctx.String(http.StatusOK, "fail")
		return
	}
	if result.Amount != "" && !stripeAmountEqual(order.Money, result.Amount) {
		log.Warn(fmt.Sprintf("StripeNotify: order %s amount mismatch: local=%s upstream=%s", order.TradeId, order.Money, result.Amount))
		ctx.String(http.StatusOK, "fail")
		return
	}

	switch result.Status {
	case stripe.StatusSuccess:
		s.markOrderConfirming(order, result)
	case stripe.StatusExpired:
		if order.Status == model.OrderStatusWaiting {
			order.SetExpired()
			notify.Bepusdt(order)
		}
	case stripe.StatusFailed:
		if order.Status == model.OrderStatusWaiting {
			order.SetFailed()
			notify.Bepusdt(order)
		}
	default:
		log.Info(fmt.Sprintf("StripeNotify: order %s pending event %s", order.TradeId, result.EventType))
	}

	ctx.String(http.StatusOK, "success")
}

// parseWebhook 使用启用的 Stripe 通道配置依次验签并解析 Webhook。
// 多通道共用回调地址时，通过各通道的 webhook_secret 找到实际接收事件的通道。
func (s stripeTask) parseWebhook(headers http.Header, body []byte) (*stripe.WebhookResult, error) {
	channels, err := s.listChannels()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, channel := range channels {
		config, err := stripe.ParseConfigText(channel.Config)
		if err != nil {
			lastErr = err
			continue
		}
		result, err := stripe.VerifyAndParseWebhook(config, headers, body, time.Now())
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("enabled stripe channel not found")
}

// markOrderConfirming 将 Stripe 已支付订单标记为确认中，并保存上游会话和支付凭据。
func (s stripeTask) markOrderConfirming(order model.Order, result *stripe.WebhookResult) {
	if order.Status == model.OrderStatusSuccess || order.Status == model.OrderStatusConfirming {
		return
	}
	if order.Status != model.OrderStatusWaiting {
		log.Warn(fmt.Sprintf("StripeNotify: order %s status invalid: %d", order.TradeId, order.Status))
		return
	}
	confirmedAt := time.Now()
	if result.PaidAt != nil {
		confirmedAt = *result.PaidAt
	}
	refOrderNo := firstNonEmpty(result.SessionID, order.RefOrderNo)
	refHash := firstNonEmpty(result.PaymentIntentID, result.ProviderRef, result.EventID)
	order.FromAddress = "stripe"
	order.MarkChannelConfirming(refOrderNo, result.EventID, refHash, confirmedAt)
	log.Info(fmt.Sprintf("StripeNotify: order %s marked confirming with provider ref %s", order.TradeId, result.ProviderRef))
}

// expireWaitingOrders 将超过有效期且仍未支付的 Stripe 订单标记为过期。
func (s stripeTask) expireWaitingOrders(context.Context) {
	var orders []model.Order
	if err := model.Db.Where("trade_type in (?) and status = ?", stripeTradeTypes(), model.OrderStatusWaiting).Find(&orders).Error; err != nil {
		log.Task.Error("Stripe: waiting order query failed", err)
		return
	}

	now := time.Now()
	for _, order := range orders {
		if now.Before(order.ExpiredAt) {
			continue
		}
		order.SetExpired()
		notify.Bepusdt(order)
		log.Info(fmt.Sprintf("Stripe: order %s expired", order.OrderId))
	}
}

// tradeConfirmHandle 将已收到 Stripe 成功通知的确认中订单推进为最终成功。
func (s stripeTask) tradeConfirmHandle(context.Context) {
	orders := getConfirmingOrders(stripeTradeTypes())
	for _, order := range orders {
		order.SetSuccess()
		notify.Bepusdt(order)
		log.Info(fmt.Sprintf("Stripe: order %s finalized", order.OrderId))
	}
}

// listChannels 返回所有启用的 Stripe 支付通道，用于 Webhook 验签匹配。
func (s stripeTask) listChannels() ([]model.Channel, error) {
	channels := make([]model.Channel, 0)
	err := model.Db.Where("trade_type in (?) and status = ?", stripeTradeTypes(), model.ClStatusEnable).Find(&channels).Error
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("enabled stripe channel not found")
	}
	return channels, nil
}

// stripeTradeTypes 返回任务需要处理的全部 Stripe 交易类型。
func stripeTradeTypes() []model.TradeType {
	return []model.TradeType{
		model.StripeAlipay,
		model.StripeWechatPay,
		model.StripeCard,
		model.StripeAll,
	}
}

// stripeAmountEqual 使用十进制定点数比较本地订单金额与 Stripe 回调金额。
func stripeAmountEqual(left, right string) bool {
	l, err := decimal.NewFromString(strings.TrimSpace(left))
	if err != nil {
		return false
	}
	r, err := decimal.NewFromString(strings.TrimSpace(right))
	if err != nil {
		return false
	}
	return l.Equal(r)
}

// firstNonEmpty 返回参数中第一个去除首尾空白后非空的字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
