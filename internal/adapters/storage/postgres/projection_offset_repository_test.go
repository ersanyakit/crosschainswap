package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestProjectionOffsetAdvanceIntegration(t *testing.T) {
	repo, db := projectionOffsetIntegrationRepository(t)
	market := "POFF/USD"
	projection := "orderbook_levels"
	if err := db.Where("projection = ? AND market = ?", projection, market).Delete(&ExchangeProjectionOffset{}).Error; err != nil {
		t.Fatalf("cleanup projection offset failed: %v", err)
	}
	defer db.Where("projection = ? AND market = ?", projection, market).Delete(&ExchangeProjectionOffset{})

	first, err := repo.AdvanceProjectionOffset(context.Background(), ProjectionOffset{
		Projection:    "OrderBook_Levels",
		Market:        market,
		Stream:        market,
		LastSequence:  10,
		LastEventID:   "event-10",
		LastEventHash: "hash-10",
	})
	if err != nil {
		t.Fatalf("advance first offset failed: %v", err)
	}
	if first.Projection != projection || first.Market != market || first.LastSequence != 10 {
		t.Fatalf("unexpected first offset: %#v", first)
	}

	older, err := repo.AdvanceProjectionOffset(context.Background(), ProjectionOffset{
		Projection:   projection,
		Market:       market,
		LastSequence: 9,
		LastEventID:  "event-9",
	})
	if err != nil {
		t.Fatalf("older offset should be ignored without error: %v", err)
	}
	if older.LastSequence != 10 || older.LastEventID != "event-10" {
		t.Fatalf("older offset rewound state: %#v", older)
	}

	if _, err := repo.AdvanceProjectionOffset(context.Background(), ProjectionOffset{
		Projection:    projection,
		Market:        market,
		LastSequence:  10,
		LastEventID:   "event-10-different",
		LastEventHash: "hash-10",
	}); err == nil {
		t.Fatalf("same sequence with different event id should conflict")
	}

	next, err := repo.AdvanceProjectionOffset(context.Background(), ProjectionOffset{
		Projection:    projection,
		Market:        market,
		LastSequence:  11,
		LastEventID:   "event-11",
		LastEventHash: "hash-11",
	})
	if err != nil {
		t.Fatalf("advance next offset failed: %v", err)
	}
	if next.LastSequence != 11 || next.LastEventID != "event-11" {
		t.Fatalf("unexpected next offset: %#v", next)
	}

	loaded, err := repo.GetProjectionOffset(context.Background(), projection, market)
	if err != nil {
		t.Fatalf("load projection offset failed: %v", err)
	}
	if loaded.LastSequence != 11 || loaded.LastEventHash != "hash-11" {
		t.Fatalf("unexpected loaded offset: %#v", loaded)
	}
}

func projectionOffsetIntegrationRepository(t *testing.T) (*ExchangeRepository, *gorm.DB) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		_ = LoadEnv(".")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("DATABASE_URL is required for projection offset integration tests")
	}
	db, err := ConnectWithOptions(ConnectOptions{AutoMigrate: true})
	if err != nil {
		t.Skipf("postgres integration database unavailable: %v", err)
	}
	return NewExchangeRepository(db), db
}
