package epusdt

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/callback"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/duolabao"
)

// duolabaoReturnGracePeriod 是多啦宝同步回跳桥接链接的宽限期。
// 超过订单过期时间 1 小时后，桥接链接返回 404，不再跳回上游 return_url。
const duolabaoReturnGracePeriod = time.Hour

// DuolabaoNotify 接收哆啦宝异步支付回调，并交给后台任务处理验签和订单状态推进。
func (Epusdt) DuolabaoNotify(ctx *gin.Context) {
	callback.HandleDuolabaoNotify(ctx)
}

// ensureDuolabaoPayment 创建哆啦宝动态收款二维码并写回订单。
func (Epusdt) ensureDuolabaoPayment(ctx *gin.Context, order model.Order) (model.Order, error) {
	if order.TradeType != model.DuolabaoQr || order.Status != model.OrderStatusWaiting {
		return order, nil
	}
	if strings.TrimSpace(order.QrcodeURL) != "" {
		return order, nil
	}

	channel, config, err := findDuolabaoOrderChannel(order)
	if err != nil {
		return order, err
	}

	notifyURL := strings.TrimSpace(config.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimRight(requestHost(ctx), "/") + "/api/v1/pay/duolabao/notify"
	}
	returnURL := buildDuolabaoCompleteURL(ctx, order, config)
	requestNum, err := buildDuolabaoRequestNum(order.TradeId)
	if err != nil {
		return order, err
	}
	if duolabaoOrderLikelyCreated(order, config) {
		log.Warn(fmt.Sprintf(
			"Duolabao: regenerating payment url for order %s channel %s customer %s amount %s requestNum %s",
			order.TradeId,
			channel.Name,
			maskDuolabaoID(config.CustomerNum),
			order.Money,
			requestNum,
		))
	}

	result, err := duolabao.CreatePayment(ctx.Request.Context(), config, duolabao.CreateInput{
		OrderNo:    requestNum,
		Amount:     order.Money,
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		ClientIP:   ctx.ClientIP(),
		TimeExpire: order.ExpiredAt.Format("2006-01-02 15:04:05"),
		ExtraInfo:  order.TradeId,
	})
	if err != nil {
		log.Warn(fmt.Sprintf(
			"Duolabao: payment create failed for order %s channel %s customer %s amount %s: %v",
			order.TradeId,
			channel.Name,
			maskDuolabaoID(config.CustomerNum),
			order.Money,
			err,
		))
		return order, err
	}

	order.QrcodeURL = result.URL
	order.Address = config.CustomerNum
	order.RefFrom = config.CustomerNum
	order.RefOrderNo = requestNum
	if err := model.Db.Save(&order).Error; err != nil {
		return order, err
	}

	log.Info(fmt.Sprintf("Duolabao: payment url created for order %s by channel %s requestNum %s", order.TradeId, channel.Name, requestNum))
	return order, nil
}

// DuolabaoReturn 接收哆啦宝浏览器同步回跳，再跳转到订单原始 ReturnUrl。
// 这样 BEpusdt 向哆啦宝暴露的是本地桥接地址，不直接泄露 dujiao-next 等上游系统的 return_url。
func (Epusdt) DuolabaoReturn(ctx *gin.Context) {
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
	if duolabaoReturnExpired(order, time.Now()) {
		ctx.String(http.StatusNotFound, "order return expired")
		return
	}

	returnURL := strings.TrimSpace(order.ReturnUrl)
	if returnURL == "" {
		ctx.String(http.StatusBadRequest, "return_url is empty")
		return
	}
	if _, err := url.ParseRequestURI(returnURL); err != nil {
		log.Error("DuolabaoReturn: return_url parse failed: ", err.Error())
		ctx.String(http.StatusBadRequest, "return_url is invalid")
		return
	}

	ctx.Redirect(http.StatusFound, returnURL)
}

// duolabaoReturnExpired 判断哆啦宝桥接回跳链接是否已经过期。
// 规则不区分订单状态，只要当前时间超过订单过期时间 1 小时即视为失效。
func duolabaoReturnExpired(order model.Order, now time.Time) bool {
	return now.After(order.ExpiredAt.Add(duolabaoReturnGracePeriod))
}

// buildDuolabaoCompleteURL 构造传给哆啦宝的 completeUrl。
// 有订单 ReturnUrl 时优先使用 BEpusdt 本地桥接链接承接回跳；没有订单 ReturnUrl 时回退到通道配置。
func buildDuolabaoCompleteURL(ctx *gin.Context, order model.Order, config *duolabao.Config) string {
	if strings.TrimSpace(order.ReturnUrl) != "" && strings.TrimSpace(order.TradeId) != "" {
		return strings.TrimRight(requestHost(ctx), "/") + "/api/v1/pay/duolabao/return/" + url.PathEscape(strings.TrimSpace(order.TradeId))
	}
	if config == nil {
		return ""
	}
	return strings.TrimSpace(config.ReturnURL)
}

// buildDuolabaoRequestNum 生成提交给哆啦宝的商户请求号。
// 哆啦宝 requestNum 使用日期时间加随机数的纯数字格式，避免复用本地 trade_id 触发重复下单或格式兼容问题。
func buildDuolabaoRequestNum(tradeID string) (string, error) {
	tradeID = strings.TrimSpace(tradeID)
	if tradeID == "" {
		return "", errors.New("duolabao trade_id is empty")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(90000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%05d", time.Now().Format("20060102150405"), n.Int64()+10000), nil
}

// duolabaoOrderLikelyCreated 判断订单是否已经向哆啦宝创建过支付链接。
// 由于动态二维码链接不持久化，重复打开收银台时会重新向哆啦宝下单，这里用于输出诊断日志。
func duolabaoOrderLikelyCreated(order model.Order, config *duolabao.Config) bool {
	if config == nil {
		return false
	}
	customerNum := strings.TrimSpace(config.CustomerNum)
	if customerNum == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(order.Address), customerNum) ||
		strings.EqualFold(strings.TrimSpace(order.RefFrom), customerNum)
}

// maskDuolabaoID 脱敏展示哆啦宝商户号等标识，避免日志泄露完整编号。
func maskDuolabaoID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	if len(value) <= 8 {
		return "****" + value[len(value)-4:]
	}
	return value[:4] + "****" + value[len(value)-4:]
}

// findDuolabaoOrderChannel 查找本地订单对应的哆啦宝通道配置。
func findDuolabaoOrderChannel(order model.Order) (model.Channel, *duolabao.Config, error) {
	channels, err := listDuolabaoChannels()
	if err != nil {
		return model.Channel{}, nil, err
	}

	var fallbackChannel model.Channel
	var fallbackConfig *duolabao.Config
	validCount := 0
	for _, channel := range channels {
		config, err := duolabao.ParseConfigText(channel.Config)
		if err != nil {
			log.Warn(fmt.Sprintf("Duolabao: skip invalid channel config %s: %v", channel.Name, err))
			continue
		}
		if err := duolabao.ValidateConfig(config); err != nil {
			log.Warn(fmt.Sprintf("Duolabao: skip incomplete channel config %s: %v", channel.Name, err))
			continue
		}
		validCount++
		if strings.TrimSpace(order.Address) != "" && strings.TrimSpace(order.Address) == strings.TrimSpace(channel.MatchQr) {
			return channel, config, nil
		}
		if strings.TrimSpace(order.Address) != "" &&
			strings.EqualFold(strings.TrimSpace(order.Address), strings.TrimSpace(config.CustomerNum)) {
			return channel, config, nil
		}
		if strings.TrimSpace(order.RefFrom) != "" &&
			strings.EqualFold(strings.TrimSpace(order.RefFrom), strings.TrimSpace(config.CustomerNum)) {
			return channel, config, nil
		}
		fallbackChannel = channel
		fallbackConfig = config
	}

	if validCount == 1 && fallbackConfig != nil {
		return fallbackChannel, fallbackConfig, nil
	}
	return model.Channel{}, nil, fmt.Errorf("no enabled duolabao channel matches order %s", order.TradeId)
}

// listDuolabaoChannels 返回所有启用的哆啦宝通道。
func listDuolabaoChannels() ([]model.Channel, error) {
	channels := make([]model.Channel, 0)
	err := model.Db.Where("trade_type = ? and status = ?", model.DuolabaoQr, model.ClStatusEnable).Find(&channels).Error
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, errors.New("enabled duolabao channel not found")
	}
	return channels, nil
}

// requestHost 返回当前请求的完整站点地址。
func requestHost(ctx *gin.Context) string {
	scheme := "http"
	if ctx.Request.TLS != nil {
		scheme = "https"
	} else if proto := ctx.GetHeader("X-Forwarded-Proto"); proto == "https" {
		scheme = "https"
	}
	return scheme + "://" + ctx.Request.Host
}
