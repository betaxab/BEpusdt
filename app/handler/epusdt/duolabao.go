package epusdt

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/callback"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/duolabao"
)

// DuolabaoNotify receives the Duolabao payment callback and delegates processing.
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
	returnURL := strings.TrimSpace(config.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(order.ReturnUrl)
	}

	result, err := duolabao.CreatePayment(ctx.Request.Context(), config, duolabao.CreateInput{
		OrderNo:    order.TradeId,
		Amount:     order.Money,
		NotifyURL:  notifyURL,
		ReturnURL:  returnURL,
		ClientIP:   ctx.ClientIP(),
		TimeExpire: order.ExpiredAt.Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return order, err
	}

	order.QrcodeURL = result.URL
	order.Address = config.CustomerNum
	order.RefFrom = config.CustomerNum
	if err := model.Db.Save(&order).Error; err != nil {
		return order, err
	}

	log.Info(fmt.Sprintf("Duolabao: payment url created for order %s by channel %s", order.TradeId, channel.Name))
	return order, nil
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
