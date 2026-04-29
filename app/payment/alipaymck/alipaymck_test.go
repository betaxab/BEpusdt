package alipaymck

import "testing"

func TestParseCallbackMapFallsBackToGmtCreate(t *testing.T) {
	callback, err := ParseCallbackMap(map[string]interface{}{
		"trade_status":      "成功",
		"total_amount":      "12.34",
		"alipay_order_no":   "202604280001",
		"merchant_order_no": "ORDER001",
		"other_account":     "buyer@example.com",
		"gmt_create":        "2026-04-28 12:34:56",
	})
	if err != nil {
		t.Fatalf("ParseCallbackMap() error = %v", err)
	}

	if callback.TradeTime.IsZero() {
		t.Fatal("TradeTime is zero")
	}
	if got := callback.TradeTime.Format(tradeTimeLayout); got != "2026-04-28 12:34:56" {
		t.Fatalf("TradeTime = %q, want %q", got, "2026-04-28 12:34:56")
	}
}

func TestParseCallbackMapFallsBackToTransDt(t *testing.T) {
	callback, err := ParseCallbackMap(map[string]interface{}{
		"trade_status":      "成功",
		"total_amount":      "12.34",
		"alipay_order_no":   "202604280001",
		"merchant_order_no": "ORDER001",
		"other_account":     "buyer@example.com",
		"trans_dt":          "2026-04-28 12:34:56",
	})
	if err != nil {
		t.Fatalf("ParseCallbackMap() error = %v", err)
	}

	if callback.TradeTime.IsZero() {
		t.Fatal("TradeTime is zero")
	}
	if got := callback.TradeTime.Format(tradeTimeLayout); got != "2026-04-28 12:34:56" {
		t.Fatalf("TradeTime = %q, want %q", got, "2026-04-28 12:34:56")
	}
}
