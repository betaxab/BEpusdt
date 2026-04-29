package task

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/alipaymck"
)

type alipayMck struct{}

var ali alipayMck

func alipayMckInit() {
	ali = alipayMck{}
	Register(Task{
		Duration: 30 * time.Second,
		Callback: ali.syncBill,
	})
	Register(Task{
		Duration: 5 * time.Second,
		Callback: ali.tradeConfirmHandle,
	})
}

func (a *alipayMck) syncBill(ctx context.Context) {
	var orders []model.Order
	model.Db.Where("trade_type = ? AND status = ?", model.AlipayMck, model.OrderStatusWaiting).Find(&orders)
	if len(orders) == 0 {
		return
	}

	processedChannels := make(map[int64]bool)
	for _, order := range orders {
		if time.Now().After(order.ExpiredAt) {
			log.Info(fmt.Sprintf("AlipayMck: Order %s expired", order.OrderId))
			order.SetExpired()
			continue
		}

		var channel model.Channel
		result := model.Db.Where("match_qr = ?", order.Address).First(&channel)
		if result.Error != nil {
			log.Error(fmt.Sprintf("AlipayMck task: Channel not found for order %s address %s", order.OrderId, order.Address))
			continue
		}
		if processedChannels[channel.ID] {
			continue
		}

		cfg, err := alipaymck.ParseConfigText(channel.Config)
		if err != nil {
			log.Error(fmt.Sprintf("AlipayMck task: Invalid config for channel %s: %v", channel.Name, err))
			continue
		}
		client, err := alipaymck.NewClient(cfg)
		if err != nil {
			log.Error(fmt.Sprintf("AlipayMck task: Build client failed for channel %s: %v", channel.Name, err))
			continue
		}

		now := time.Now()
		startTime := now.Add(-5 * time.Minute)
		res, err := client.QuerySellBill(ctx, startTime, now, 1, 2000)
		if err != nil {
			log.Error(fmt.Sprintf("AlipayMck request error for order %s: %v", order.OrderId, err))
			continue
		}
		processedChannels[channel.ID] = true

		callbacks, err := alipaymck.ParseCallbacks(res)
		if err != nil {
			log.Error(fmt.Sprintf("AlipayMck parse callback failed for channel %s: %v", channel.Name, err))
			continue
		}

		transfers := make([]transfer, 0, len(callbacks))
		for _, callback := range callbacks {
			if err := alipaymck.VerifyCallback(cfg, callback); err != nil {
				continue
			}
			parsed, ok := a.parseTransfer(callback)
			if ok {
				transfers = append(transfers, parsed)
			}
		}

		if len(transfers) > 0 {
			transferQueue.In <- transfers
		}
	}
}

func (a *alipayMck) parseTransfer(item *alipaymck.CallbackData) (transfer, bool) {
	if item == nil {
		return transfer{}, false
	}
	totalAmount, err := decimal.NewFromString(item.TotalAmount)
	if err != nil {
		return transfer{}, false
	}

	tradeTime := item.TradeTime
	if tradeTime.IsZero() {
		tradeTime = time.Now()
	}
	tradeTimeText := tradeTime.Format("2006-01-02 15:04:05")

	log.Task.Info(fmt.Sprintf("AlipayMck Scan: order_no=%s account=%s amount=%s status=%s time=%s", item.AlipayOrderNo, item.OtherAccount, item.TotalAmount, item.TradeStatus, tradeTimeText))

	// TxHash: 支付宝交易订单号； FromAddress: 对方交易信息； RecvAddress: 商家订单号
	return transfer{
		Network:     "AlipayMck",
		TxHash:      item.AlipayOrderNo,
		Amount:      totalAmount,
		FromAddress: item.OtherAccount,
		RecvAddress: item.MerchantOrderNo,
		Timestamp:   tradeTime,
		TradeType:   model.AlipayMck,
		BlockNum:    0,
	}, true
}

// tradeConfirmHandle handles the final confirmation of AlipayMck orders
func (a *alipayMck) tradeConfirmHandle(ctx context.Context) {
	orders := getConfirmingOrders([]model.TradeType{model.AlipayMck})
	for _, o := range orders {
		markFinalConfirmed(o)
		log.Info(fmt.Sprintf("AlipayMck: Order %s finalized", o.OrderId))
	}
}
