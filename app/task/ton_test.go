package task

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/smallnest/chanx"
	"github.com/v03413/bepusdt/app/conf"
	"github.com/v03413/bepusdt/app/model"
	tgo "github.com/xssnick/tonutils-go/ton"
)

func TestTonShardRangesContinueAfterPreviousMasterBlock(t *testing.T) {
	previous := []*tgo.BlockIDExt{
		{Workchain: 0, Shard: 1, SeqNo: 100},
	}
	current := []*tgo.BlockIDExt{
		{Workchain: 0, Shard: 1, SeqNo: 103},
	}

	got := tonShardRanges(previous, current)
	want := []tonShardRange{
		{Workchain: 0, Shard: 1, Start: 101, End: 103},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tonShardRanges() = %#v, want %#v", got, want)
	}
}

func TestRetryTonBlockRetriesTheSameHeight(t *testing.T) {
	var calls []uint32
	err := retryTonBlock(context.Background(), 123, 0, func(seqno uint32) error {
		calls = append(calls, seqno)
		if len(calls) == 1 {
			return errors.New("temporary RPC error")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("retryTonBlock() error = %v", err)
	}

	want := []uint32{123, 123}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("retryTonBlock() calls = %v, want %v", calls, want)
	}
}

func TestPendingTonLookbackIsMarkedOnlyAfterCompletion(t *testing.T) {
	originalDB := model.Db
	if err := model.Init(filepath.Join(t.TempDir(), "ton-lookback.db"), ""); err != nil {
		t.Fatalf("init test db: %v", err)
	}
	t.Cleanup(func() {
		model.Close()
		model.Db = originalDB
	})

	now := model.Datetime(time.Now().Add(-time.Minute))
	zero := time.Unix(0, 0)
	order := model.Order{
		OrderId:     "ton-lookback-order",
		TradeId:     "ton-lookback-trade",
		TradeType:   model.UsdtTon,
		Fiat:        model.CNY,
		Crypto:      model.USDT,
		Rate:        "7",
		Amount:      "1",
		Money:       "7",
		Address:     "UQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAJKZ",
		Status:      model.OrderStatusWaiting,
		ApiType:     model.OrderApiTypeEpusdt,
		ExpiredAt:   time.Now().Add(time.Hour),
		ConfirmedAt: &zero,
		AutoTimeAt: model.AutoTimeAt{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}
	if err := model.Db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	lookbackDone.Delete(order.ID)
	t.Cleanup(func() { lookbackDone.Delete(order.ID) })

	_, _, orderIDs, ok := pendingLookbackUnix(conf.Ton)
	if !ok {
		t.Fatal("pendingLookbackUnix() ok = false")
	}
	if _, marked := lookbackDone.Load(order.ID); marked {
		t.Fatal("order was marked before the lookback completed")
	}

	markLookbackDone(orderIDs)
	if _, marked := lookbackDone.Load(order.ID); !marked {
		t.Fatal("order was not marked after the lookback completed")
	}
}

func TestTonSyncStopsForExpiredOrders(t *testing.T) {
	originalDB := model.Db
	if err := model.Init(filepath.Join(t.TempDir(), "ton-expired-order.db"), ""); err != nil {
		t.Fatalf("init test db: %v", err)
	}
	t.Cleanup(func() {
		model.Close()
		model.Db = originalDB
	})

	now := model.Datetime(time.Now().Add(-time.Hour))
	zero := time.Unix(0, 0)
	order := model.Order{
		OrderId:     "ton-expired-order",
		TradeId:     "ton-expired-trade",
		TradeType:   model.UsdtTon,
		Fiat:        model.CNY,
		Crypto:      model.USDT,
		Rate:        "7",
		Amount:      "1",
		Money:       "7",
		Address:     "UQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAJKZ",
		Status:      model.OrderStatusExpired,
		ApiType:     model.OrderApiTypeEpusdt,
		ExpiredAt:   time.Now().Add(-time.Minute),
		ConfirmedAt: &zero,
		AutoTimeAt: model.AutoTimeAt{
			CreatedAt: &now,
			UpdatedAt: &now,
		},
	}
	if err := model.Db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	scanner := ton{
		blockScanQueue: chanx.NewUnboundedChan[uint32](context.Background(), 1),
	}
	if !scanner.syncBreak() {
		t.Fatal("syncBreak() = false for an expired order")
	}
}
