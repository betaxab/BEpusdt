package epusdt

import (
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
