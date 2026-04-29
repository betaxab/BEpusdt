package task

import (
	"testing"
	"time"

	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/duolabao"
)

func TestDuolabaoParseTransferMapsCallbackFields(t *testing.T) {
	parsed, ok := duolabaoTask{}.parseTransfer(&duolabao.CallbackData{
		OrderAmount:    "12.34",
		OrderNum:       "100200000001",
		BankOutTradeNo: "420000000001",
		BankRequestNum: "100300000001",
		SubOpenID:      "openid_001",
		CompleteTime:   "2026-04-28 12:34:56",
	})
	if !ok {
		t.Fatal("parseTransfer() ok = false")
	}

	if parsed.TradeType != model.DuolabaoQr {
		t.Fatalf("TradeType = %q, want %q", parsed.TradeType, model.DuolabaoQr)
	}
	if parsed.TxHash != "100200000001" {
		t.Fatalf("TxHash = %q, want orderNum", parsed.TxHash)
	}
	if parsed.FromAddress != "420000000001" {
		t.Fatalf("FromAddress = %q, want bankOutTradeNum", parsed.FromAddress)
	}
	if parsed.RecvAddress != "100300000001" {
		t.Fatalf("RecvAddress = %q, want bankRequestNum", parsed.RecvAddress)
	}
	if parsed.RefFromInfo != "openid_001" {
		t.Fatalf("RefFromInfo = %q, want subOpenId", parsed.RefFromInfo)
	}
	if got := parsed.Timestamp.Format(time.DateTime); got != "2026-04-28 12:34:56" {
		t.Fatalf("Timestamp = %q, want completeTime", got)
	}
}
