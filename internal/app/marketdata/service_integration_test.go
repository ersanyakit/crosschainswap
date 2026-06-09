package marketdata

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	storage "exchange/internal/adapters/storage/postgres"
	"exchange/internal/core/decimal"
	"exchange/internal/core/market"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"

	"gorm.io/gorm"
)

func TestCandleWorkerBuildsCandlesFromTradesIntegration(t *testing.T) {
	repo, db := marketDataIntegrationRepository(t)
	marketSymbol := "MDATA/USD"
	cleanupMarketDataIntegration(t, db, marketSymbol)
	defer cleanupMarketDataIntegration(t, db, marketSymbol)
	if err := storage.SyncExchangeMarkets(db, []market.Market{
		{Symbol: marketSymbol, BaseAsset: "MDATA", QuoteAsset: "USD", Enabled: true},
	}); err != nil {
		t.Fatalf("sync exchange market failed: %v", err)
	}

	baseTime := time.Date(2026, 6, 9, 9, 0, 5, 0, time.UTC)
	trades := []trade.Trade{
		{
			ID:            "trd_mdata_1",
			Market:        marketSymbol,
			BaseAsset:     "MDATA",
			QuoteAsset:    "USD",
			MakerOrderID:  order.ID("maker_1"),
			TakerOrderID:  order.ID("taker_1"),
			MakerUserID:   "seller",
			TakerUserID:   "buyer",
			TakerSide:     order.SideBuy,
			Price:         "10",
			Quantity:      "2",
			QuoteQuantity: "20",
			CreatedAt:     baseTime,
		},
		{
			ID:            "trd_mdata_2",
			Market:        marketSymbol,
			BaseAsset:     "MDATA",
			QuoteAsset:    "USD",
			MakerOrderID:  order.ID("maker_2"),
			TakerOrderID:  order.ID("taker_2"),
			MakerUserID:   "seller",
			TakerUserID:   "buyer",
			TakerSide:     order.SideBuy,
			Price:         "12",
			Quantity:      "3",
			QuoteQuantity: "36",
			CreatedAt:     baseTime.Add(10 * time.Second),
		},
	}
	if err := repo.CreateTrades(context.Background(), trades); err != nil {
		t.Fatalf("create trades failed: %v", err)
	}

	candles, err := repo.ListCandles(context.Background(), marketSymbol, "1m", 10)
	if err != nil {
		t.Fatalf("list candles before worker failed: %v", err)
	}
	if len(candles) != 0 {
		t.Fatalf("CreateTrades should not update candles synchronously: %#v", candles)
	}

	processed, err := RunOnce(context.Background(), repo, market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: "MDATA", QuoteAsset: "USD", Enabled: true},
	}), 100)
	if err != nil {
		t.Fatalf("run candle worker failed: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed trades = %d, want 2", processed)
	}

	candles, err = repo.ListCandles(context.Background(), marketSymbol, "1m", 10)
	if err != nil {
		t.Fatalf("list candles after worker failed: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected one 1m candle, got %#v", candles)
	}
	candle := candles[0]
	if decimal.Cmp(candle.Open, "10") != 0 ||
		decimal.Cmp(candle.High, "12") != 0 ||
		decimal.Cmp(candle.Low, "10") != 0 ||
		decimal.Cmp(candle.Close, "12") != 0 ||
		decimal.Cmp(candle.VolumeBase, "5") != 0 ||
		decimal.Cmp(candle.VolumeQuote, "56") != 0 ||
		candle.TradeCount != 2 {
		t.Fatalf("unexpected candle: %#v", candle)
	}

	offset, err := repo.GetProjectionOffset(context.Background(), candleProjectionName, marketSymbol)
	if err != nil {
		t.Fatalf("load candle projection offset failed: %v", err)
	}
	if offset.LastSequence != 2 || offset.LastEventID != "trd_mdata_2" || !offset.LastEventAt.Equal(trades[1].CreatedAt) {
		t.Fatalf("unexpected candle projection offset: %#v", offset)
	}
	var marketRow storage.ExchangeMarket
	if err := db.First(&marketRow, "symbol = ?", marketSymbol).Error; err != nil {
		t.Fatalf("load exchange market row failed: %v", err)
	}
	if decimal.Cmp(marketRow.LastPrice, "12") != 0 ||
		decimal.Cmp(marketRow.Change24h, "20") != 0 ||
		decimal.Cmp(marketRow.High24h, "12") != 0 ||
		decimal.Cmp(marketRow.Low24h, "10") != 0 ||
		decimal.Cmp(marketRow.Volume24h, "5") != 0 {
		t.Fatalf("unexpected exchange market ticker stats: %#v", marketRow)
	}

	processed, err = RunOnce(context.Background(), repo, market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: "MDATA", QuoteAsset: "USD", Enabled: true},
	}), 100)
	if err != nil {
		t.Fatalf("second candle worker run failed: %v", err)
	}
	if processed != 0 {
		t.Fatalf("second run should not reprocess trades, got %d", processed)
	}
}

func marketDataIntegrationRepository(t *testing.T) (*storage.ExchangeRepository, *gorm.DB) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		_ = storage.LoadEnv(".")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("DATABASE_URL is required for marketdata integration tests")
	}
	db, err := storage.ConnectWithOptions(storage.ConnectOptions{AutoMigrate: true})
	if err != nil {
		t.Skipf("postgres integration database unavailable: %v", err)
	}
	return storage.NewExchangeRepository(db), db
}

func cleanupMarketDataIntegration(t *testing.T, db *gorm.DB, marketSymbol string) {
	t.Helper()
	for _, model := range []any{
		&storage.ExchangeCandle{},
		&storage.ExchangeTrade{},
		&storage.ExchangeProjectionOffset{},
	} {
		if err := db.Where("market = ?", marketSymbol).Delete(model).Error; err != nil {
			t.Fatalf("cleanup %T failed: %v", model, err)
		}
	}
	if err := db.Where("symbol = ?", marketSymbol).Delete(&storage.ExchangeMarket{}).Error; err != nil {
		t.Fatalf("cleanup exchange market failed: %v", err)
	}
}
