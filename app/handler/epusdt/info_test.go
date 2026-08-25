package epusdt

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	applog "github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
)

func TestInfoReturnsReselectForUnselectedAdminOrder(t *testing.T) {
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

	if err := model.Init(filepath.Join(t.TempDir(), "test.db"), ""); err != nil {
		t.Fatal(err)
	}

	order, err := model.BuildPendingOrder(model.OrderParams{
		Money:   decimal.NewFromInt(100),
		ApiType: model.OrderApiTypeAdmin,
		OrderId: "admin-info-reselect",
		Name:    "admin info reselect",
		Fiat:    model.CNY,
	})
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/pay/info", bytes.NewBufferString(`{"trade_id":"`+order.TradeId+`"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	Epusdt{}.Info(ctx)

	var res struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Reselect bool `json:"reselect"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status_code = %d, want 200; body=%s", res.StatusCode, recorder.Body.String())
	}
	if !res.Data.Reselect {
		t.Fatalf("reselect = %v, want true; body=%s", res.Data.Reselect, recorder.Body.String())
	}
}

func TestCreateOrderReturnsReselect(t *testing.T) {
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

	if err := model.Init(filepath.Join(t.TempDir(), "test.db"), ""); err != nil {
		t.Fatal(err)
	}
	if err := applog.Init(filepath.Join(t.TempDir(), "logs")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(applog.Close)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/order/create-order", bytes.NewBufferString(`{
		"order_id": "create-order-reselect",
		"notify_url": "https://example.com/notify",
		"redirect_url": "https://example.com/return",
		"signature": "unused",
		"amount": 100,
		"name": "create order reselect",
		"fiat": "CNY",
		"reselect": true
	}`))
	ctx.Request.Host = "bepusdt.example"
	ctx.Request.Header.Set("Content-Type", "application/json")

	Epusdt{}.CreateOrder(ctx)

	var res struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Reselect bool `json:"reselect"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status_code = %d, want 200; body=%s", res.StatusCode, recorder.Body.String())
	}
	if !res.Data.Reselect {
		t.Fatalf("reselect = %v, want true; body=%s", res.Data.Reselect, recorder.Body.String())
	}
}
