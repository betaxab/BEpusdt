package epusdt

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/model"
)

// orderPaymentEnsurer 在返回订单前准备通道专属支付信息。
type orderPaymentEnsurer func(Epusdt, *gin.Context, model.Order) (model.Order, error)

// orderPaymentEnsurers 将通道专属支付准备逻辑从订单主流程中分离出来。
var orderPaymentEnsurers = map[model.TradeType]orderPaymentEnsurer{
	model.DuolabaoQr: Epusdt.ensureDuolabaoPayment,
}

// ensureOrderPayment 按交易类型分发可选的通道专属支付准备逻辑。
func (e Epusdt) ensureOrderPayment(ctx *gin.Context, order model.Order) (model.Order, error) {
	ensurer, ok := orderPaymentEnsurers[order.TradeType]
	if !ok {
		return order, nil
	}
	return ensurer(e, ctx, order)
}

// paymentAddress 返回对外展示的付款地址；动态二维码通道可使用虚拟地址。
func paymentAddress(order model.Order) string {
	if order.TradeType == model.DuolabaoQr && strings.TrimSpace(order.QrcodeURL) != "" {
		return order.QrcodeURL
	}
	return order.Address
}
