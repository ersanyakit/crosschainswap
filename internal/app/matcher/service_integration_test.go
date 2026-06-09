package matcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	storage "exchange/internal/adapters/storage/postgres"
	apporders "exchange/internal/app/orders"
	"exchange/internal/core/balance"
	"exchange/internal/core/decimal"
	"exchange/internal/core/market"
	corematching "exchange/internal/core/matching"
	"exchange/internal/core/order"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
)

func TestCommandLogMatcherAppliesPendingOrdersIntegration(t *testing.T) {
	repo, db := matcherIntegrationRepository(t)
	base, marketSymbol := matcherIntegrationMarket("MCLG")
	seller := matcherIntegrationUser(t, "seller")
	buyer := matcherIntegrationUser(t, "buyer")
	cleanupMatcherIntegrationMarket(t, db, marketSymbol, seller, buyer)
	defer cleanupMatcherIntegrationMarket(t, db, marketSymbol, seller, buyer)
	t.Setenv("MATCHING_MODE", "command_log")
	t.Setenv("MATCHING_ENGINE", "book")
	t.Setenv("MATCHING_RUNTIME", "resident")
	t.Setenv("MATCHER_SNAPSHOT_EVERY", "1")

	svc := apporders.NewService(market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}), repo)
	fundMatcherUser(t, repo, seller, base, "5")
	fundMatcherUser(t, repo, buyer, "USD", "500")

	sell, err := svc.Place(context.Background(), apporders.PlaceRequest{
		CommandID:     "cmd_" + base + "_sell",
		ClientOrderID: "client_" + base + "_sell",
		UserID:        seller,
		Market:        marketSymbol,
		Side:          string(order.SideSell),
		Type:          string(order.TypeLimit),
		Price:         "100",
		Quantity:      "5",
	})
	if err != nil {
		t.Fatalf("place sell failed: %v", err)
	}
	buy, err := svc.Place(context.Background(), apporders.PlaceRequest{
		CommandID:     "cmd_" + base + "_buy",
		ClientOrderID: "client_" + base + "_buy",
		UserID:        buyer,
		Market:        marketSymbol,
		Side:          string(order.SideBuy),
		Type:          string(order.TypeMarket),
		Price:         "100",
		Quantity:      "5",
	})
	if err != nil {
		t.Fatalf("place buy failed: %v", err)
	}
	if sell.Order.Status != order.StatusPendingMatch || buy.Order.Status != order.StatusPendingMatch {
		t.Fatalf("orders should be accepted as pending_match: sell=%#v buy=%#v", sell.Order, buy.Order)
	}

	var matchJobCount int64
	if err := db.Model(&storage.ExchangeMatchJob{}).Where("market = ?", marketSymbol).Count(&matchJobCount).Error; err != nil {
		t.Fatalf("count match jobs failed: %v", err)
	}
	if matchJobCount != 0 {
		t.Fatalf("command-log mode created %d legacy match jobs", matchJobCount)
	}

	runtime := newMarketRuntime(repo, svc, market.Market{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true})
	processed, err := runCommandLogOnceWithRuntime(context.Background(), repo, svc, marketSymbol, "test-worker", 10, 1, time.Second, time.Minute, runtime)
	if err != nil {
		t.Fatalf("run command-log matcher failed: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed command logs = %d, want 2", processed)
	}
	if runtime.book == nil || runtime.book.ActiveOrderCount() != 0 {
		t.Fatalf("resident book should be empty after full match")
	}

	reloadedSell, err := repo.GetOrder(context.Background(), sell.Order.ID)
	if err != nil {
		t.Fatalf("reload sell failed: %v", err)
	}
	reloadedBuy, err := repo.GetOrder(context.Background(), buy.Order.ID)
	if err != nil {
		t.Fatalf("reload buy failed: %v", err)
	}
	if reloadedSell.Status != order.StatusFilled || decimal.Cmp(reloadedSell.RemainingQuantity, "0") != 0 {
		t.Fatalf("sell should be filled: %#v", reloadedSell)
	}
	if reloadedBuy.Status != order.StatusFilled || decimal.Cmp(reloadedBuy.RemainingQuantity, "0") != 0 {
		t.Fatalf("buy should be filled: %#v", reloadedBuy)
	}

	var appliedCount int64
	if err := db.Model(&storage.ExchangeOrderCommandLog{}).
		Where("market = ? AND status = ?", marketSymbol, storage.OrderCommandLogStatusApplied).
		Count(&appliedCount).Error; err != nil {
		t.Fatalf("count applied command logs failed: %v", err)
	}
	if appliedCount != 2 {
		t.Fatalf("applied command logs = %d, want 2", appliedCount)
	}
	snapshot, err := repo.LatestMatcherSnapshot(context.Background(), marketSymbol)
	if err != nil {
		t.Fatalf("latest matcher snapshot failed: %v", err)
	}
	if snapshot.LastAppliedSequence != 2 || len(snapshot.ActiveOrders) != 0 {
		t.Fatalf("unexpected runtime snapshot: %#v", snapshot)
	}

	book, err := svc.Book(context.Background(), marketSymbol, 10)
	if err != nil {
		t.Fatalf("book failed: %v", err)
	}
	if len(book.Bids) != 0 || len(book.Asks) != 0 {
		t.Fatalf("book should be empty after full match: %#v", book)
	}
}

func TestMatcherSnapshotRepositoryRoundTripIntegration(t *testing.T) {
	repo, db := matcherIntegrationRepository(t)
	base, marketSymbol := matcherIntegrationMarket("MSNP")
	cleanupMatcherIntegrationMarket(t, db, marketSymbol)
	defer cleanupMatcherIntegrationMarket(t, db, marketSymbol)

	book := corematching.NewMarketBook(marketSymbol, base, "USD")
	if err := book.Load([]order.Order{
		{
			ID:                order.ID("ord_" + base + "_1"),
			ClientOrderID:     order.ClientOrderID("client_" + base + "_1"),
			UserID:            "user_" + base,
			Market:            marketSymbol,
			BaseAsset:         base,
			QuoteAsset:        "USD",
			Side:              order.SideBuy,
			Type:              order.TypeLimit,
			Status:            order.StatusOpen,
			TimeInForce:       order.TimeInForceGTC,
			Price:             "1",
			Quantity:          "2",
			FilledQuantity:    "0",
			RemainingQuantity: "2",
			SequenceID:        7,
			CreatedAt:         time.Unix(1, 0).UTC(),
			UpdatedAt:         time.Unix(1, 0).UTC(),
		},
	}); err != nil {
		t.Fatalf("load book failed: %v", err)
	}

	snapshot := book.CaptureState(12, time.Unix(10, 0).UTC())
	if err := repo.SaveMatcherSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("save matcher snapshot failed: %v", err)
	}
	latest, err := repo.LatestMatcherSnapshot(context.Background(), marketSymbol)
	if err != nil {
		t.Fatalf("load latest matcher snapshot failed: %v", err)
	}
	if latest.LastAppliedSequence != 12 || latest.Checksum != snapshot.Checksum {
		t.Fatalf("unexpected latest snapshot: %#v", latest)
	}
	restored, err := corematching.RestoreMarketBook(*latest)
	if err != nil {
		t.Fatalf("restore latest snapshot failed: %v", err)
	}
	if restored.ActiveOrderCount() != 1 {
		t.Fatalf("restored active orders = %d, want 1", restored.ActiveOrderCount())
	}
	if bestBid, ok := restored.BestBid(); !ok || bestBid != "1" {
		t.Fatalf("restored best bid = %s/%v, want 1/true", bestBid, ok)
	}
}

func TestResidentRuntimeReplayStartsAfterLatestSnapshotIntegration(t *testing.T) {
	repo, db := matcherIntegrationRepository(t)
	base, marketSymbol := matcherIntegrationMarket("MRPL")
	cleanupMatcherIntegrationMarket(t, db, marketSymbol)
	defer cleanupMatcherIntegrationMarket(t, db, marketSymbol)

	if _, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_" + base + "_1",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-1",
		Payload:     `{"id":"1"}`,
	}); err != nil {
		t.Fatalf("append first command log failed: %v", err)
	}
	second, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_" + base + "_2",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-2",
		Payload:     `{"id":"2"}`,
	})
	if err != nil {
		t.Fatalf("append second command log failed: %v", err)
	}

	book := corematching.NewMarketBook(marketSymbol, base, "USD")
	snapshot := book.CaptureState(1, time.Now().UTC())
	if err := repo.SaveMatcherSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("save matcher snapshot failed: %v", err)
	}

	runtime := newMarketRuntime(repo, apporders.NewService(market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}), repo), market.Market{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true})
	replay, err := runtime.ReplayFromSnapshot(context.Background(), 10)
	if err != nil {
		t.Fatalf("replay from snapshot failed: %v", err)
	}
	if runtime.lastSequence != 1 {
		t.Fatalf("runtime last sequence = %d, want 1", runtime.lastSequence)
	}
	if len(replay) != 1 || replay[0].CommandID != second.CommandID {
		t.Fatalf("unexpected replay commands: %#v", replay)
	}
}

func TestResidentRuntimeRejectsUnsafeSnapshotRetentionIntegration(t *testing.T) {
	repo, db := matcherIntegrationRepository(t)
	base, marketSymbol := matcherIntegrationMarket("MRET")
	cleanupMatcherIntegrationMarket(t, db, marketSymbol)
	defer cleanupMatcherIntegrationMarket(t, db, marketSymbol)
	t.Setenv("MATCHER_MAX_SNAPSHOT_AGE", "1ns")
	t.Setenv("MATCHER_EVENT_RETENTION", "1h")

	if _, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_" + base + "_1",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-1",
		Payload:     `{"id":"1"}`,
	}); err != nil {
		t.Fatalf("append command log failed: %v", err)
	}
	book := corematching.NewMarketBook(marketSymbol, base, "USD")
	snapshot := book.CaptureState(1, time.Now().UTC().Add(-time.Hour))
	if err := repo.SaveMatcherSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("save matcher snapshot failed: %v", err)
	}

	runtime := newMarketRuntime(repo, apporders.NewService(market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}), repo), market.Market{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true})
	err := runtime.ensureLoaded(context.Background())
	if !errors.Is(err, corematching.ErrSnapshotRetentionUnsafe) {
		t.Fatalf("expected unsafe snapshot retention error, got %v", err)
	}
}

func TestResidentRuntimeRebuildsActiveProjectionWhenSnapshotLagsAppliedCommandsIntegration(t *testing.T) {
	repo, db := matcherIntegrationRepository(t)
	base, marketSymbol := matcherIntegrationMarket("MAPL")
	cleanupMatcherIntegrationMarket(t, db, marketSymbol)
	defer cleanupMatcherIntegrationMarket(t, db, marketSymbol)

	if _, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_" + base + "_1",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-1",
		Payload:     `{"id":"1"}`,
	}); err != nil {
		t.Fatalf("append first command log failed: %v", err)
	}
	if err := repo.ApplyOrderCommandLog(context.Background(), "cmd_"+base+"_1", "ord_"+base+"_1"); err != nil {
		t.Fatalf("apply first command log failed: %v", err)
	}
	second, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_" + base + "_2",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-2",
		Payload:     `{"id":"2"}`,
	})
	if err != nil {
		t.Fatalf("append second command log failed: %v", err)
	}
	if err := repo.ApplyOrderCommandLog(context.Background(), second.CommandID, "ord_"+base+"_2"); err != nil {
		t.Fatalf("apply second command log failed: %v", err)
	}

	book := corematching.NewMarketBook(marketSymbol, base, "USD")
	snapshot := book.CaptureState(1, time.Now().UTC())
	if err := repo.SaveMatcherSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("save matcher snapshot failed: %v", err)
	}

	active := order.Order{
		ID:                order.ID("ord_" + base + "_2"),
		ClientOrderID:     order.ClientOrderID("client_" + base + "_2"),
		UserID:            "user_" + strings.ToLower(base),
		Market:            marketSymbol,
		BaseAsset:         base,
		QuoteAsset:        "USD",
		Side:              order.SideSell,
		Type:              order.TypeLimit,
		Status:            order.StatusOpen,
		TimeInForce:       order.TimeInForceGTC,
		Price:             "100",
		Quantity:          "3",
		FilledQuantity:    "0",
		RemainingQuantity: "3",
		SequenceID:        2,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := repo.CreateOrder(context.Background(), active); err != nil {
		t.Fatalf("create active order failed: %v", err)
	}

	runtime := newMarketRuntime(repo, apporders.NewService(market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}), repo), market.Market{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true})
	if err := runtime.ensureLoaded(context.Background()); err != nil {
		t.Fatalf("runtime ensure loaded failed: %v", err)
	}
	if runtime.lastSequence != second.SequenceID {
		t.Fatalf("runtime last sequence = %d, want %d", runtime.lastSequence, second.SequenceID)
	}
	if runtime.book == nil || runtime.book.ActiveOrderCount() != 1 {
		t.Fatalf("runtime should rebuild active book from projection, got %#v", runtime.book)
	}
	if bestAsk, ok := runtime.book.BestAsk(); !ok || decimal.Cmp(bestAsk, "100") != 0 {
		t.Fatalf("runtime best ask = %s/%v, want 100/true", bestAsk, ok)
	}
}

func TestResidentRuntimeReplaysMatchEventLogAfterSnapshotIntegration(t *testing.T) {
	repo, db := matcherIntegrationRepository(t)
	base, marketSymbol := matcherIntegrationMarket("MELG")
	cleanupMatcherIntegrationMarket(t, db, marketSymbol)
	defer cleanupMatcherIntegrationMarket(t, db, marketSymbol)

	book := corematching.NewMarketBook(marketSymbol, base, "USD")
	snapshot := book.CaptureState(1, time.Now().UTC())
	if err := repo.SaveMatcherSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("save matcher snapshot failed: %v", err)
	}

	resting := order.Order{
		ID:                order.ID("ord_" + base + "_ask"),
		ClientOrderID:     order.ClientOrderID("client_" + base + "_ask"),
		UserID:            "user_" + strings.ToLower(base),
		Market:            marketSymbol,
		BaseAsset:         base,
		QuoteAsset:        "USD",
		Side:              order.SideSell,
		Type:              order.TypeLimit,
		Status:            order.StatusOpen,
		TimeInForce:       order.TimeInForceGTC,
		Price:             "101",
		Quantity:          "4",
		FilledQuantity:    "0",
		RemainingQuantity: "4",
		SequenceID:        2,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	payload := replayMatchEventPayload{Taker: resting}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal replay payload failed: %v", err)
	}
	sum := sha256.Sum256(raw)
	if _, err := repo.AppendMatchEventLog(context.Background(), storage.MatchEventLog{
		Market:      marketSymbol,
		SequenceID:  2,
		Type:        storage.MatchEventTypeResult,
		PayloadHash: hex.EncodeToString(sum[:]),
		Payload:     string(raw),
	}); err != nil {
		t.Fatalf("append match event log failed: %v", err)
	}

	runtime := newMarketRuntime(repo, apporders.NewService(market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}), repo), market.Market{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true})
	if err := runtime.ensureLoaded(context.Background()); err != nil {
		t.Fatalf("runtime ensure loaded failed: %v", err)
	}
	if runtime.lastSequence != 2 {
		t.Fatalf("runtime last sequence = %d, want 2", runtime.lastSequence)
	}
	replayed, ok := runtime.book.ActiveOrder(resting.ID)
	if !ok || decimal.Cmp(replayed.RemainingQuantity, "4") != 0 {
		t.Fatalf("resting order was not replayed from event log: %#v", replayed)
	}
	if bestAsk, ok := runtime.book.BestAsk(); !ok || decimal.Cmp(bestAsk, "101") != 0 {
		t.Fatalf("runtime best ask = %s/%v, want 101/true", bestAsk, ok)
	}
}

func matcherIntegrationRepository(t *testing.T) (*storage.ExchangeRepository, *gorm.DB) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		_ = storage.LoadEnv(".")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("DATABASE_URL is required for matcher integration tests")
	}
	db, err := storage.ConnectWithOptions(storage.ConnectOptions{AutoMigrate: true})
	if err != nil {
		t.Skipf("postgres integration database unavailable: %v", err)
	}
	return storage.NewExchangeRepository(db), db
}

func matcherIntegrationMarket(base string) (string, string) {
	base = strings.ToUpper(strings.TrimSpace(base))
	return base, base + "/USD"
}

func matcherIntegrationUser(t *testing.T, suffix string) string {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(strings.ToLower(t.Name()))
	return fmt.Sprintf("it_%s_%s", name, suffix)
}

func cleanupMatcherIntegrationMarket(t *testing.T, db *gorm.DB, marketSymbol string, users ...string) {
	t.Helper()
	for _, model := range []any{
		&storage.ExchangeOrderEvent{},
		&storage.ExchangeTrade{},
		&storage.ExchangeCandle{},
		&storage.ExchangePriceLevel{},
		&storage.ExchangeMatchJob{},
		&storage.ExchangeActiveOrder{},
		&storage.ExchangeOrderCommandLog{},
		&storage.ExchangeMatcherSnapshot{},
		&storage.ExchangeMatchEventLog{},
		&storage.ExchangeOrderCommand{},
		&storage.ExchangeReservation{},
		&storage.ExchangeOrder{},
		&storage.ExchangeOrderCommandSequence{},
		&storage.ExchangeOrderSequence{},
	} {
		if err := db.Where("market = ?", marketSymbol).Delete(model).Error; err != nil {
			t.Fatalf("cleanup %T failed: %v", model, err)
		}
	}
	if len(users) == 0 {
		return
	}
	if err := db.Where("user_id IN ?", users).Delete(&storage.ExchangeBalanceEvent{}).Error; err != nil {
		t.Fatalf("cleanup balance events failed: %v", err)
	}
	if err := db.Where("user_id IN ?", users).Delete(&storage.ExchangeBalance{}).Error; err != nil {
		t.Fatalf("cleanup balances failed: %v", err)
	}
}

func fundMatcherUser(t *testing.T, repo *storage.ExchangeRepository, userID string, asset string, amount string) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.MarkDepositPending(ctx, userID, asset, amount, balance.EventID(idgen.New("bev"))); err != nil {
		t.Fatalf("fund pending %s %s failed: %v", userID, asset, err)
	}
	if _, err := repo.SettleDeposit(ctx, userID, asset, amount, balance.EventID(idgen.New("bev"))); err != nil {
		t.Fatalf("fund settle %s %s failed: %v", userID, asset, err)
	}
}
