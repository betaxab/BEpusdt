package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCreatePaymentCreatesCheckoutSessionWithPaymentMethod(t *testing.T) {
	var body string
	restore := mockHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/checkout/sessions" {
			t.Fatalf("path = %q, want /v1/checkout/sessions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
			t.Fatalf("Authorization = %q, want Basic auth", got)
		}
		raw, _ := ioReadAllString(r)
		body = raw
		return jsonResponse(`{"id":"cs_test_123","url":"https://checkout.stripe.com/pay/cs_test_123","status":"open","payment_intent":"pi_test_123"}`), nil
	})
	defer restore()

	result, err := CreatePayment(context.Background(), &Config{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_123",
		APIBaseURL:    "https://stripe.test",
	}, CreateInput{
		OrderNo:            "trade-123",
		MerchantOrderID:    "order-123",
		Amount:             "100.50",
		Currency:           "CNY",
		Description:        "test order",
		SuccessURL:         "https://example.com/success",
		CancelURL:          "https://example.com/cancel",
		PaymentMethodTypes: []string{MethodAlipay},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "cs_test_123" {
		t.Fatalf("SessionID = %q, want cs_test_123", result.SessionID)
	}
	if result.PaymentIntentID != "pi_test_123" {
		t.Fatalf("PaymentIntentID = %q, want pi_test_123", result.PaymentIntentID)
	}
	if result.URL == "" {
		t.Fatal("URL is empty")
	}
	assertFormContains(t, body, "mode=payment")
	assertFormContains(t, body, "line_items%5B0%5D%5Bprice_data%5D%5Bunit_amount%5D=10050")
	assertFormContains(t, body, "payment_method_types%5B%5D=alipay")
	assertFormContains(t, body, "metadata%5Btrade_id%5D=trade-123")
}

func TestCreatePaymentAllDoesNotLimitPaymentMethods(t *testing.T) {
	var body string
	restore := mockHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		body, _ = ioReadAllString(r)
		return jsonResponse(`{"id":"cs_test_all","url":"https://checkout.stripe.com/pay/cs_test_all","status":"open"}`), nil
	})
	defer restore()

	if _, err := CreatePayment(context.Background(), &Config{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_123",
		APIBaseURL:    "https://stripe.test",
	}, CreateInput{
		OrderNo:     "trade-all",
		Amount:      "100",
		Currency:    "CNY",
		SuccessURL:  "https://example.com/success",
		CancelURL:   "https://example.com/cancel",
		Description: "all methods",
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "payment_method_types") {
		t.Fatalf("body contains payment_method_types for stripe.all: %s", body)
	}
}

func TestVerifyAndParseWebhookCheckoutSession(t *testing.T) {
	now := time.Unix(1710000000, 0)
	body := []byte(`{
		"id":"evt_test_123",
		"type":"checkout.session.completed",
		"data":{
			"object":{
				"id":"cs_test_123",
				"object":"checkout.session",
				"payment_intent":"pi_test_123",
				"currency":"cny",
				"amount_total":10050,
				"created":1710000000,
				"metadata":{"trade_id":"trade-123","order_id":"order-123"}
			}
		}
	}`)
	headers := http.Header{}
	headers.Set("Stripe-Signature", signWebhook("whsec_123", now.Unix(), body))

	result, err := VerifyAndParseWebhook(&Config{
		SecretKey:     "sk_test_123",
		WebhookSecret: "whsec_123",
		APIBaseURL:    defaultAPIBaseURL,
	}, headers, body, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("Status = %q, want %q", result.Status, StatusSuccess)
	}
	if result.OrderNo != "trade-123" {
		t.Fatalf("OrderNo = %q, want trade-123", result.OrderNo)
	}
	if result.ProviderRef != "cs_test_123" {
		t.Fatalf("ProviderRef = %q, want cs_test_123", result.ProviderRef)
	}
	if result.PaymentIntentID != "pi_test_123" {
		t.Fatalf("PaymentIntentID = %q, want pi_test_123", result.PaymentIntentID)
	}
	if result.Amount != "100.5" {
		t.Fatalf("Amount = %q, want 100.5", result.Amount)
	}
}

func assertFormContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("form body missing %q: %s", want, body)
	}
}

func signWebhook(secret string, timestamp int64, body []byte) string {
	payload := []byte(strconv.FormatInt(timestamp, 10) + "." + string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return "t=" + strconv.FormatInt(timestamp, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func ioReadAllString(r *http.Request) (string, error) {
	data, err := io.ReadAll(r.Body)
	return string(data), err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func mockHTTPClient(t *testing.T, fn roundTripFunc) func() {
	t.Helper()
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: fn}
	return func() {
		http.DefaultClient = oldClient
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
