package epusdt

import (
	"testing"

	"github.com/v03413/bepusdt/app/model"
)

func TestPaymentAddressUsesDuolabaoQRCodeURL(t *testing.T) {
	order := model.Order{
		TradeType: model.DuolabaoQr,
		Address:   "100000000001",
		QrcodeURL: "https://order.duolabao.com/active/c?state=ok",
	}

	if got := paymentAddress(order); got != order.QrcodeURL {
		t.Fatalf("paymentAddress() = %q, want %q", got, order.QrcodeURL)
	}
}

func TestPaymentAddressUsesAddressForNonDuolabaoOrder(t *testing.T) {
	order := model.Order{
		TradeType: model.UsdtTrc20,
		Address:   "TReceiverAddress",
		QrcodeURL: "https://order.duolabao.com/active/c?state=ok",
	}

	if got := paymentAddress(order); got != order.Address {
		t.Fatalf("paymentAddress() = %q, want %q", got, order.Address)
	}
}
