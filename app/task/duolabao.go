package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/callback"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/duolabao"
	"github.com/v03413/bepusdt/app/task/notify"
	"gorm.io/gorm"
)

type duolabaoTask struct{}

type duolabaoTransfer struct {
	transfer
	RefFromInfo string
}

var duolabaoRunner duolabaoTask

// duolabaoInit 注册哆啦宝后台任务。
func duolabaoInit() {
	duolabaoRunner = duolabaoTask{}
	callback.RegisterDuolabaoNotify(duolabaoRunner.notify)
	Register(Task{
		Duration: 30 * time.Second,
		Callback: duolabaoRunner.expireWaitingOrders,
	})
	Register(Task{
		Duration: 5 * time.Second,
		Callback: duolabaoRunner.tradeConfirmHandle,
	})
}

func (d duolabaoTask) notify(ctx *gin.Context) {
	parseBody, signBody, err := readDuolabaoCallbackBody(ctx)
	if err != nil {
		log.Warn("DuolabaoNotify: read callback body failed:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}

	callback, err := duolabao.ParseCallback(parseBody)
	if err != nil {
		log.Warn("DuolabaoNotify: parse callback failed:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}

	_, config, err := d.findNotifyChannel(callback)
	if err != nil {
		log.Warn("DuolabaoNotify: channel config not found:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}

	if err := duolabao.VerifyNotifySignatureByHeader(config, signBody, ctx.Request.Header, ctx.Request.Method); err != nil {
		log.Warn("DuolabaoNotify: signature invalid:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}
	if err := duolabao.VerifyCallback(config, callback); err != nil {
		log.Warn("DuolabaoNotify: callback invalid:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}

	order, err := d.findOrder(callback.RequestNum)
	if err != nil {
		log.Warn("DuolabaoNotify: order not found:", err.Error())
		ctx.String(http.StatusOK, "fail")
		return
	}
	if order.TradeType != model.DuolabaoQr {
		log.Warn(fmt.Sprintf("DuolabaoNotify: order %s trade_type mismatch: %s", order.TradeId, order.TradeType))
		ctx.String(http.StatusOK, "fail")
		return
	}

	parsed, ok := d.parseTransfer(callback)
	if !ok {
		log.Warn(fmt.Sprintf("DuolabaoNotify: transfer parse failed for requestNum %s", callback.RequestNum))
		ctx.String(http.StatusOK, "fail")
		return
	}
	if !duolabaoAmountEqual(order.Money, parsed.Amount) {
		log.Warn(fmt.Sprintf("DuolabaoNotify: order %s amount mismatch: local=%s upstream=%s", order.TradeId, order.Money, callback.OrderAmount))
		ctx.String(http.StatusOK, "fail")
		return
	}

	if order.Status == model.OrderStatusSuccess || order.Status == model.OrderStatusConfirming {
		ctx.String(http.StatusOK, "success")
		return
	}
	if order.Status != model.OrderStatusWaiting {
		log.Warn(fmt.Sprintf("DuolabaoNotify: order %s status invalid: %d", order.TradeId, order.Status))
		ctx.String(http.StatusOK, "fail")
		return
	}

	if d.refHashUsed(order.ID, parsed.TxHash) {
		log.Warn(fmt.Sprintf("DuolabaoNotify: ref hash already used: %s", parsed.TxHash))
		ctx.String(http.StatusOK, "fail")
		return
	}

	order.FromAddress = parsed.FromAddress
	order.MarkChannelConfirming(parsed.RecvAddress, parsed.RefFromInfo, parsed.TxHash, parsed.Timestamp)

	log.Info(fmt.Sprintf("DuolabaoNotify: order %s marked confirming with upstream order %s", order.TradeId, parsed.TxHash))
	ctx.String(http.StatusOK, "success")
}

func (d duolabaoTask) parseTransfer(callback *duolabao.CallbackData) (duolabaoTransfer, bool) {
	if callback == nil {
		return duolabaoTransfer{}, false
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(callback.OrderAmount))
	if err != nil {
		return duolabaoTransfer{}, false
	}

	// from_address: bankOutTradeNum，一般是微信单号
	// ref_orderno: bankRequestNum，银行流水号 1003开头
	// ref_from_info: subOpenId，用户 openID
	// ref_hash: orderNum，jd 的订单号 1002开头
	return duolabaoTransfer{
		transfer: transfer{
			Network:     "Duolabao",
			TxHash:      strings.TrimSpace(callback.OrderNum),
			Amount:      amount,
			FromAddress: strings.TrimSpace(callback.BankOutTradeNo),
			RecvAddress: strings.TrimSpace(callback.BankRequestNum),
			Timestamp:   parseDuolabaoCompleteTime(callback.CompleteTime),
			TradeType:   model.DuolabaoQr,
			BlockNum:    0,
		},
		RefFromInfo: strings.TrimSpace(callback.SubOpenID),
	}, true
}

// expireWaitingOrders 将超时未支付的哆啦宝订单标记为过期。
func (d duolabaoTask) expireWaitingOrders(context.Context) {
	var orders []model.Order
	if err := model.Db.Where("trade_type = ? and status = ?", model.DuolabaoQr, model.OrderStatusWaiting).Find(&orders).Error; err != nil {
		log.Task.Error("Duolabao: waiting order query failed", err)
		return
	}

	now := time.Now()
	for _, order := range orders {
		if now.Before(order.ExpiredAt) {
			continue
		}
		order.SetExpired()
		notify.Bepusdt(order)
		log.Info(fmt.Sprintf("Duolabao: order %s expired", order.OrderId))
	}
}

// tradeConfirmHandle 将已收到哆啦宝成功回调的订单推进为最终成功。
func (d duolabaoTask) tradeConfirmHandle(context.Context) {
	orders := getConfirmingOrders([]model.TradeType{model.DuolabaoQr})
	for _, order := range orders {
		markFinalConfirmed(order)
		log.Info(fmt.Sprintf("Duolabao: order %s finalized", order.OrderId))
	}
}

func readDuolabaoCallbackBody(ctx *gin.Context) ([]byte, []byte, error) {
	if ctx.Request.Method != http.MethodGet {
		body, err := ctx.GetRawData()
		return body, body, err
	}

	payload := make(map[string]string)
	for key, values := range ctx.Request.URL.Query() {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return body, nil, nil
}

func (d duolabaoTask) findNotifyChannel(callback *duolabao.CallbackData) (model.Channel, *duolabao.Config, error) {
	if callback == nil {
		return model.Channel{}, nil, errors.New("callback is nil")
	}

	channels, err := d.listChannels()
	if err != nil {
		return model.Channel{}, nil, err
	}

	customerNum := strings.TrimSpace(callback.CustomerNum)
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
		if customerNum != "" && strings.EqualFold(strings.TrimSpace(config.CustomerNum), customerNum) {
			return channel, config, nil
		}
		fallbackChannel = channel
		fallbackConfig = config
	}

	if customerNum == "" && validCount == 1 && fallbackConfig != nil {
		return fallbackChannel, fallbackConfig, nil
	}
	return model.Channel{}, nil, fmt.Errorf("no enabled duolabao channel matches customerNum %s", customerNum)
}

func (d duolabaoTask) listChannels() ([]model.Channel, error) {
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

func (d duolabaoTask) findOrder(requestNum string) (model.Order, error) {
	requestNum = strings.TrimSpace(requestNum)
	if requestNum == "" {
		return model.Order{}, errors.New("requestNum is empty")
	}

	var order model.Order
	err := model.Db.Where("trade_id = ?", requestNum).Limit(1).First(&order).Error
	if err == nil {
		return order, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Order{}, err
	}

	err = model.Db.Where("order_id = ?", requestNum).Order("id desc").Limit(1).First(&order).Error
	if err != nil {
		return model.Order{}, err
	}
	return order, nil
}

func duolabaoAmountEqual(localAmount string, upstreamAmount decimal.Decimal) bool {
	local, err := decimal.NewFromString(strings.TrimSpace(localAmount))
	if err != nil {
		return false
	}
	return local.Round(2).Equal(upstreamAmount.Round(2))
}

func (d duolabaoTask) refHashUsed(orderID int64, refHash string) bool {
	refHash = strings.TrimSpace(refHash)
	if refHash == "" {
		return false
	}
	var count int64
	model.Db.Model(&model.Order{}).
		Where("id <> ? and trade_type = ? and ref_hash = ?", orderID, model.DuolabaoQr, refHash).
		Count(&count)
	return count > 0
}

func parseDuolabaoCompleteTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now()
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"20060102150405",
		time.RFC3339,
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Now()
}
