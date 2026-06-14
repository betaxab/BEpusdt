package epusdt

import (
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/duolabao"
)

func TestDuolabaoOrderLikelyCreated(t *testing.T) {
	config := &duolabao.Config{CustomerNum: "100000000001"}

	if !duolabaoOrderLikelyCreated(model.Order{Address: "100000000001"}, config) {
		t.Fatal("duolabaoOrderLikelyCreated() = false for matching address")
	}
	if !duolabaoOrderLikelyCreated(model.Order{RefFrom: "100000000001"}, config) {
		t.Fatal("duolabaoOrderLikelyCreated() = false for matching ref_from")
	}
	if duolabaoOrderLikelyCreated(model.Order{Address: "200000000001"}, config) {
		t.Fatal("duolabaoOrderLikelyCreated() = true for non-matching order")
	}
	if duolabaoOrderLikelyCreated(model.Order{Address: "100000000001"}, nil) {
		t.Fatal("duolabaoOrderLikelyCreated() = true for nil config")
	}
}

func TestMaskDuolabaoID(t *testing.T) {
	got := maskDuolabaoID("100000000001")
	want := "1000****0001"
	if got != want {
		t.Fatalf("maskDuolabaoID() = %q, want %q", got, want)
	}
}

func TestBuildDuolabaoRequestNumUsesNumericFormat(t *testing.T) {
	tradeID := "c1z5YXuYIecxez6bDB"
	requestNum, err := buildDuolabaoRequestNum(tradeID)
	if err != nil {
		t.Fatalf("buildDuolabaoRequestNum() error = %v", err)
	}
	if requestNum == tradeID {
		t.Fatal("requestNum was not regenerated")
	}
	if !regexp.MustCompile(`^\d{19}$`).MatchString(requestNum) {
		t.Fatalf("requestNum = %q, want 19 numeric digits", requestNum)
	}
}

func TestBuildDuolabaoCompleteURLUsesLocalBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "http://bepusdt.example/pay/checkout-counter/TRADE-1", nil)
	ctx.Request.Host = "bepusdt.example"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	got := buildDuolabaoCompleteURL(ctx, model.Order{
		TradeId:   "TRADE 1",
		ReturnUrl: "https://dujiao.example/pay/return?order=DJ1",
	}, &duolabao.Config{ReturnURL: "https://fallback.example/return"})

	want := "https://bepusdt.example/api/v1/pay/duolabao/return/TRADE%201"
	if got != want {
		t.Fatalf("buildDuolabaoCompleteURL() = %q, want %q", got, want)
	}
}

func TestBuildDuolabaoCompleteURLFallsBackToChannelReturnURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "http://bepusdt.example/pay/checkout-counter/TRADE-1", nil)

	got := buildDuolabaoCompleteURL(ctx, model.Order{
		TradeId: "TRADE-1",
	}, &duolabao.Config{ReturnURL: "https://fallback.example/return"})

	want := "https://fallback.example/return"
	if got != want {
		t.Fatalf("buildDuolabaoCompleteURL() = %q, want %q", got, want)
	}
}

func TestDuolabaoReturnExpired(t *testing.T) {
	now := time.Date(2026, 6, 13, 18, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		expiredAt time.Time
		want      bool
	}{
		{
			name:      "within one hour after order expiration",
			expiredAt: now.Add(-duolabaoReturnGracePeriod),
			want:      false,
		},
		{
			name:      "over one hour after order expiration",
			expiredAt: now.Add(-duolabaoReturnGracePeriod - time.Second),
			want:      true,
		},
		{
			name:      "zero expiration is expired",
			expiredAt: time.Time{},
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := duolabaoReturnExpired(model.Order{ExpiredAt: tc.expiredAt}, now)
			if got != tc.want {
				t.Fatalf("duolabaoReturnExpired() = %v, want %v", got, tc.want)
			}
		})
	}
}
