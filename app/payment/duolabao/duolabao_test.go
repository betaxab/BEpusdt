package duolabao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePaymentErrorIncludesCodeAndMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != createPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, createPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"code":"DUPLICATE_ORDER","msg":"系统处理错误"}`))
	}))
	defer server.Close()

	_, err := CreatePayment(context.Background(), &Config{
		GatewayURL:  server.URL,
		AccessKey:   "access",
		SecretKey:   "secret",
		CustomerNum: "100000000001",
		NotifyURL:   "https://example.com/notify",
	}, CreateInput{
		OrderNo: "trade001",
		Amount:  "12.34",
	})
	if err == nil {
		t.Fatal("CreatePayment() error is nil")
	}

	want := "duolabao response invalid: [DUPLICATE_ORDER] 系统处理错误"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("CreatePayment() error = %q, want to contain %q", err.Error(), want)
	}
}

func TestCreatePaymentSendsExtraInfo(t *testing.T) {
	var payload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"url":"https://order.duolabao.com/active/c?state=ok"}`))
	}))
	defer server.Close()

	_, err := CreatePayment(context.Background(), &Config{
		GatewayURL:  server.URL,
		AccessKey:   "access",
		SecretKey:   "secret",
		CustomerNum: "100000000001",
		NotifyURL:   "https://example.com/notify",
	}, CreateInput{
		OrderNo:   "2026061414590011111",
		Amount:    "12.34",
		ExtraInfo: "local-trade-1",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if got := strings.TrimSpace(payload["extraInfo"].(string)); got != "local-trade-1" {
		t.Fatalf("extraInfo = %q, want local-trade-1", got)
	}
}

func TestFormatCreateErrorReadsNestedError(t *testing.T) {
	got := formatCreateError(map[string]interface{}{
		"error": map[string]interface{}{
			"errorCode": "E1001",
			"errorMsg":  "订单号重复",
		},
	}, &CreateResult{})

	want := "[E1001] 订单号重复"
	if got != want {
		t.Fatalf("formatCreateError() = %q, want %q", got, want)
	}
}

func TestMarshalRequestPayloadDoesNotEscapeURLQuery(t *testing.T) {
	body, err := marshalRequestPayload(map[string]interface{}{
		"completeUrl": "https://example.com/return?a=1&b=2",
	})
	if err != nil {
		t.Fatalf("marshalRequestPayload() error = %v", err)
	}

	if strings.Contains(string(body), `\u0026`) {
		t.Fatalf("marshalRequestPayload() escaped ampersand: %s", body)
	}
	if !json.Valid(body) {
		t.Fatalf("marshalRequestPayload() produced invalid JSON: %s", body)
	}
}
