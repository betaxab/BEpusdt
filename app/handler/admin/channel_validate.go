package admin

import (
	"fmt"

	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/alipaymck"
)

func validateChannel(channel *model.Channel) error {
	if channel == nil {
		return fmt.Errorf("通道为空")
	}

	switch model.TradeType(channel.TradeType) {
	case model.AlipayMck:
		// 二维码格式：https://qr.alipay.com/ts810738hvgx4ypapn585ba
		if !alipaymck.IsValidAlipayQR(channel.Qrcode) {
			return fmt.Errorf("支付宝二维码格式错误")
		}
		config, err := alipaymck.ParseConfigText(channel.Config)
		if err != nil {
			return fmt.Errorf("配置格式错误: %v", err)
		}
		if err := alipaymck.ValidateConfig(config); err != nil {
			return fmt.Errorf("配置校验错误: %v", err)
		}
	}

	return nil
}
