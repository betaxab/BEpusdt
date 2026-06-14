package task

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/payment/duolabao"
	"gorm.io/gorm"
)

func newDuolabaoTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "duolabao-task-test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Order{}); err != nil {
		t.Fatalf("auto migrate order: %v", err)
	}
	return db
}

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

func TestDuolabaoFindOrderByRegeneratedRequestNum(t *testing.T) {
	db := newDuolabaoTaskTestDB(t)
	originalDB := model.Db
	model.Db = db
	t.Cleanup(func() {
		model.Db = originalDB
	})

	now := time.Now()
	confirmedAt := now
	order := model.Order{
		OrderId:     "merchant-order-1",
		TradeId:     "c1z5YXuYIecxez6bDB",
		TradeType:   model.DuolabaoQr,
		Fiat:        model.CNY,
		Crypto:      model.CNYE,
		Rate:        "1",
		Amount:      "0",
		Money:       "12.34",
		Address:     "100000000001",
		RefOrderNo:  "2026061316164412345",
		Status:      model.OrderStatusWaiting,
		ApiType:     model.OrderApiTypeEpusdt,
		ExpiredAt:   now.Add(time.Hour),
		ConfirmedAt: &confirmedAt,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	requestNum := order.RefOrderNo
	got, err := duolabaoTask{}.findOrder(&duolabao.CallbackData{RequestNum: requestNum})
	if err != nil {
		t.Fatalf("findOrder() error = %v", err)
	}
	if got.TradeId != order.TradeId {
		t.Fatalf("findOrder() trade_id = %q, want %q", got.TradeId, order.TradeId)
	}
}

func TestDuolabaoFindOrderByExtraInfoWhenRequestNumWasSuperseded(t *testing.T) {
	db := newDuolabaoTaskTestDB(t)
	originalDB := model.Db
	model.Db = db
	t.Cleanup(func() {
		model.Db = originalDB
	})

	now := time.Now()
	confirmedAt := now
	order := model.Order{
		OrderId:     "merchant-order-2",
		TradeId:     "local-trade-2",
		TradeType:   model.DuolabaoQr,
		Fiat:        model.CNY,
		Crypto:      model.CNYE,
		Rate:        "1",
		Amount:      "0",
		Money:       "23.45",
		Address:     "100000000001",
		RefOrderNo:  "2026061415000099999",
		Status:      model.OrderStatusWaiting,
		ApiType:     model.OrderApiTypeEpusdt,
		ExpiredAt:   now.Add(time.Hour),
		ConfirmedAt: &confirmedAt,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	got, err := duolabaoTask{}.findOrder(&duolabao.CallbackData{
		RequestNum: "2026061414590011111",
		ExtraInfo:  order.TradeId,
	})
	if err != nil {
		t.Fatalf("findOrder() error = %v", err)
	}
	if got.TradeId != order.TradeId {
		t.Fatalf("findOrder() trade_id = %q, want %q", got.TradeId, order.TradeId)
	}
}
