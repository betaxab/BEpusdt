package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/v03413/bepusdt/app/conf"
)

func TestOrderCanReselectPayment(t *testing.T) {
	tests := []struct {
		name  string
		order Order
		want  bool
	}{
		{
			name: "create order waiting",
			order: Order{
				ApiType: OrderApiTypeEpusdtOrder,
				Status:  OrderStatusWaiting,
			},
			want: true,
		},
		{
			name: "create order selected",
			order: Order{
				ApiType:   OrderApiTypeEpusdtOrder,
				Status:    OrderStatusWaiting,
				TradeType: UsdtTrc20,
			},
			want: true,
		},
		{
			name: "create transaction waiting",
			order: Order{
				ApiType: OrderApiTypeEpusdt,
				Status:  OrderStatusWaiting,
			},
			want: false,
		},
		{
			name: "admin order waiting",
			order: Order{
				ApiType: OrderApiTypeAdmin,
				Status:  OrderStatusWaiting,
			},
			want: true,
		},
		{
			name: "admin order selected",
			order: Order{
				ApiType:   OrderApiTypeAdmin,
				Status:    OrderStatusWaiting,
				TradeType: UsdtTrc20,
			},
			want: true,
		},
		{
			name: "create order confirming",
			order: Order{
				ApiType: OrderApiTypeEpusdtOrder,
				Status:  OrderStatusConfirming,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.order.CanReselectPayment(); got != tt.want {
				t.Fatalf("CanReselectPayment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdminOrderGetMethodsIncludesGramTon(t *testing.T) {
	oldDB := Db
	if Db != nil {
		Close()
	}
	t.Cleanup(func() {
		if Db != nil {
			Close()
		}
		Db = oldDB
	})

	if err := Init(filepath.Join(t.TempDir(), "test.db"), ""); err != nil {
		t.Fatal(err)
	}

	now := Datetime(time.Now())
	if err := Db.Create(&Wallet{
		Name:        "ton gram",
		Status:      WaStatusEnable,
		Address:     "UQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAJKZ",
		MatchAddr:   "UQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAJKZ",
		TradeType:   string(TonGram),
		OtherNotify: WaOtherDisable,
		AutoTimeAt: AutoTimeAt{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Db.Create(&Rate{
		Rate:    "15",
		Fiat:    string(CNY),
		Crypto:  string(GRAM),
		RawRate: 15,
		AutoTimeAt: AutoTimeAt{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	order := Order{
		ApiType: OrderApiTypeAdmin,
		Status:  OrderStatusWaiting,
		Fiat:    CNY,
		Money:   "100",
	}

	assertHasGramTon := func(t *testing.T, methods []MethodItem) {
		t.Helper()
		for _, method := range methods {
			if method.Currency == string(GRAM) && method.Network == string(conf.Ton) {
				return
			}
		}
		t.Fatalf("expected GRAM TON method, got %#v", methods)
	}

	assertHasGramTon(t, order.GetMethods(""))

	order.CurrencyLimit = string(GRAM)
	assertHasGramTon(t, order.GetMethods(""))
}
