package epusdt

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/model"
)

func TestEnsureStripePaymentCreatesCheckoutSession(t *testing.T) {
	oldDB := model.Db
	if model.Db != nil {
		model.Close()
	}
	t.Cleanup(func() {
		if model.Db != nil {
			model.Close()
		}
		model.Db = oldDB
	})

	if err := model.Init(filepath.Join(t.TempDir(), "test.db"), "", ""); err != nil {
		t.Fatal(err)
	}

	restore := mockStripeHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/checkout/sessions" {
			t.Fatalf("path = %q, want /v1/checkout/sessions", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("payment_method_types[]"); got != "card" {
			t.Fatalf("payment_method_types[] = %q, want card; form=%s", got, r.Form.Encode())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_order","url":"https://checkout.stripe.com/pay/cs_test_order","status":"open","payment_intent":"pi_test_order"}`)),
		}, nil
	})
	defer restore()

	now := model.Datetime(time.Now())
	channel := model.Channel{
		Name:      "stripe card",
		Status:    model.ClStatusEnable,
		Qrcode:    "stripe.card",
		TradeType: string(model.StripeCard),
		Config: `{
			"secret_key": "sk_test_123",
			"webhook_secret": "whsec_123",
			"api_base_url": "https://stripe.test"
		}`,
		AutoTimeAt: model.AutoTimeAt{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}
	if err := model.Db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}

	order, err := model.BuildOrder(model.OrderParams{
		Money:     decimal.NewFromInt(100),
		ApiType:   model.OrderApiTypeEpusdt,
		OrderId:   "stripe-card-order",
		TradeType: model.StripeCard,
		Name:      "stripe card order",
		Fiat:      model.CNY,
	}, model.Trade{
		Wallet: model.Wallet{
			Address:   channel.Qrcode,
			MatchAddr: channel.MatchQr,
			TradeType: string(model.StripeCard),
		},
		Crypto: model.CNYE,
		Rate:   decimal.NewFromInt(1),
		Amount: "100",
	})
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/pay/checkout/"+order.TradeId, nil)
	ctx.Request.Host = "bepusdt.example"

	updated, err := Epusdt{}.ensureStripePayment(ctx, order)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(updated.QrcodeURL, "https://checkout.stripe.com/pay/") {
		t.Fatalf("QrcodeURL = %q, want Stripe Checkout URL", updated.QrcodeURL)
	}
	if updated.RefOrderNo != "cs_test_order" {
		t.Fatalf("RefOrderNo = %q, want cs_test_order", updated.RefOrderNo)
	}
	if updated.RefHash != "pi_test_order" {
		t.Fatalf("RefHash = %q, want pi_test_order", updated.RefHash)
	}
	if got := paymentAddress(updated); got != updated.QrcodeURL {
		t.Fatalf("paymentAddress() = %q, want %q", got, updated.QrcodeURL)
	}
}

func TestInfoCreatesStripeCheckoutSessionWhenMissing(t *testing.T) {
	oldDB := model.Db
	if model.Db != nil {
		model.Close()
	}
	t.Cleanup(func() {
		if model.Db != nil {
			model.Close()
		}
		model.Db = oldDB
	})

	if err := model.Init(filepath.Join(t.TempDir(), "test.db"), "", ""); err != nil {
		t.Fatal(err)
	}

	restore := mockStripeHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/checkout/sessions" {
			t.Fatalf("path = %q, want /v1/checkout/sessions", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_info","url":"https://checkout.stripe.com/pay/cs_test_info","status":"open"}`)),
		}, nil
	})
	defer restore()

	now := model.Datetime(time.Now())
	channel := model.Channel{
		Name:      "stripe all",
		Status:    model.ClStatusEnable,
		Qrcode:    "stripe.all",
		TradeType: string(model.StripeAll),
		Config: `{
			"secret_key": "sk_test_123",
			"webhook_secret": "whsec_123",
			"api_base_url": "https://stripe.test"
		}`,
		AutoTimeAt: model.AutoTimeAt{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}
	if err := model.Db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}

	order, err := model.BuildOrder(model.OrderParams{
		Money:     decimal.NewFromInt(100),
		ApiType:   model.OrderApiTypeEpusdt,
		OrderId:   "stripe-info-order",
		TradeType: model.StripeAll,
		Name:      "stripe info order",
		Fiat:      model.CNY,
	}, model.Trade{
		Wallet: model.Wallet{
			Address:   channel.Qrcode,
			MatchAddr: channel.MatchQr,
			TradeType: string(model.StripeAll),
		},
		Crypto: model.CNYE,
		Rate:   decimal.NewFromInt(1),
		Amount: "100",
	})
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/pay/info", bytes.NewBufferString(`{"trade_id":"`+order.TradeId+`"}`))
	ctx.Request.Host = "bepusdt.example"
	ctx.Request.Header.Set("Content-Type", "application/json")

	Epusdt{}.Info(ctx)

	var res struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status_code = %d, want 200; body=%s", res.StatusCode, recorder.Body.String())
	}
	if res.Data.Token != "https://checkout.stripe.com/pay/cs_test_info" {
		t.Fatalf("token = %q, want Stripe Checkout URL; body=%s", res.Data.Token, recorder.Body.String())
	}
}

type stripeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f stripeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func mockStripeHTTPClient(t *testing.T, fn stripeRoundTripFunc) func() {
	t.Helper()
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: fn}
	return func() {
		http.DefaultClient = oldClient
	}
}
