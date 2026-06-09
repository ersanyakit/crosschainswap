package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"

	"gorm.io/gorm"
)

func TestReconcileOrderBookProjectionDetectsDriftIntegration(t *testing.T) {
	repo, db := reconciliationIntegrationRepository(t)
	market := "RCON/USD"
	cleanupReconciliationMarket(t, db, market)
	defer cleanupReconciliationMarket(t, db, market)

	item := order.Order{
		ID:                "ord_rcon_1",
		ClientOrderID:     "client_rcon_1",
		UserID:            "user_rcon",
		Market:            market,
		BaseAsset:         "RCON",
		QuoteAsset:        "USD",
		Side:              order.SideBuy,
		Type:              order.TypeLimit,
		Status:            order.StatusOpen,
		TimeInForce:       order.TimeInForceGTC,
		Price:             "1",
		Quantity:          "2",
		FilledQuantity:    "0",
		RemainingQuantity: "2",
		SequenceID:        1,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := repo.SyncActiveOrder(context.Background(), item); err != nil {
		t.Fatalf("sync active order failed: %v", err)
	}
	if err := repo.RefreshPriceLevels(context.Background(), []PriceLevelKey{{Market: market, Side: order.SideBuy, Price: "1"}}); err != nil {
		t.Fatalf("refresh price levels failed: %v", err)
	}
	drift, err := repo.ReconcileOrderBookProjection(context.Background(), market)
	if err != nil {
		t.Fatalf("reconcile clean projection failed: %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("expected clean projection, got drift: %#v", drift)
	}

	if err := db.Save(&ExchangePriceLevel{
		Market:          market,
		Side:            string(order.SideBuy),
		Price:           "1",
		Quantity:        "3",
		OrderCount:      1,
		FirstSequenceID: 1,
		LastUpdatedAt:   time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("corrupt price level failed: %v", err)
	}
	drift, err = repo.ReconcileOrderBookProjection(context.Background(), market)
	if err != nil {
		t.Fatalf("reconcile drift failed: %v", err)
	}
	if len(drift) != 1 {
		t.Fatalf("expected one drift row, got %#v", drift)
	}
	if decimal.Cmp(drift[0].ActiveQuantity, "2") != 0 || decimal.Cmp(drift[0].LevelQuantity, "3") != 0 {
		t.Fatalf("unexpected drift row: %#v", drift[0])
	}
}

func reconciliationIntegrationRepository(t *testing.T) (*ExchangeRepository, *gorm.DB) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		_ = LoadEnv(".")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("DATABASE_URL is required for reconciliation integration tests")
	}
	db, err := ConnectWithOptions(ConnectOptions{AutoMigrate: true})
	if err != nil {
		t.Skipf("postgres integration database unavailable: %v", err)
	}
	return NewExchangeRepository(db), db
}

func cleanupReconciliationMarket(t *testing.T, db *gorm.DB, market string) {
	t.Helper()
	for _, model := range []any{&ExchangePriceLevel{}, &ExchangeActiveOrder{}} {
		if err := db.Where("market = ?", market).Delete(model).Error; err != nil {
			t.Fatalf("cleanup %T failed: %v", model, err)
		}
	}
}
