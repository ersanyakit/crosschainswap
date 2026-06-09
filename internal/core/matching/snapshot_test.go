package matching

import (
	"errors"
	"testing"
	"time"

	"exchange/internal/core/order"
)

func TestBookStateSnapshotCapturesFrozenCopyAndRestores(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "99", "5", 1),
		testBookOrder("ask-1", order.SideSell, order.TypeLimit, "101", "7", 2),
	}); err != nil {
		t.Fatal(err)
	}

	snapshot := book.CaptureState(42, now)
	if snapshot.LastAppliedSequence != 42 || snapshot.Checksum == "" || len(snapshot.ActiveOrders) != 2 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	_, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "101", "7", 3), nextBookTradeID(), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := book.ActiveOrder("ask-1"); ok {
		t.Fatalf("ask should be consumed in live book")
	}
	if len(snapshot.ActiveOrders) != 2 {
		t.Fatalf("snapshot changed after live mutation: %#v", snapshot.ActiveOrders)
	}

	restored, err := RestoreMarketBook(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ActiveOrderCount() != 2 {
		t.Fatalf("restored active order count = %d, want 2", restored.ActiveOrderCount())
	}
	if ask, ok := restored.ActiveOrder("ask-1"); !ok || ask.RemainingQuantity != "7" {
		t.Fatalf("restored ask missing or mutated: %#v %v", ask, ok)
	}
	bestAsk, ok := restored.BestAsk()
	if !ok || bestAsk != "101" {
		t.Fatalf("restored best ask = %s/%v, want 101/true", bestAsk, ok)
	}
}

func TestBookStateSnapshotChecksumDetectsCorruption(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{testBookOrder("ask-1", order.SideSell, order.TypeLimit, "101", "7", 1)}); err != nil {
		t.Fatal(err)
	}

	snapshot := book.CaptureState(1, now)
	snapshot.ActiveOrders[0].RemainingQuantity = "8"
	if err := snapshot.Validate(); !errors.Is(err, ErrSnapshotCorrupt) {
		t.Fatalf("expected corrupt snapshot error, got %v", err)
	}
}

func TestEncodeDecodeBookStateSnapshotValidatesChecksum(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "99", "5", 1)}); err != nil {
		t.Fatal(err)
	}

	payload, err := EncodeBookStateSnapshot(book.CaptureState(7, now))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBookStateSnapshot(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.LastAppliedSequence != 7 || len(decoded.ActiveOrders) != 1 {
		t.Fatalf("unexpected decoded snapshot: %#v", decoded)
	}
}

func TestCheckSnapshotRetentionRejectsUnsafeRecoveryWindows(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	safe := SnapshotRetentionGuard{
		LatestSnapshotSequence: 100,
		OldestRetainedSequence: 101,
		SnapshotCreatedAt:      now.Add(-time.Minute),
		Now:                    now,
		EventRetention:         15 * time.Minute,
		MaxSnapshotAge:         5 * time.Minute,
	}
	if err := CheckSnapshotRetention(safe); err != nil {
		t.Fatalf("safe retention guard failed: %v", err)
	}

	gap := safe
	gap.OldestRetainedSequence = 102
	if err := CheckSnapshotRetention(gap); !errors.Is(err, ErrSnapshotRetentionUnsafe) {
		t.Fatalf("expected retention gap error, got %v", err)
	}

	stale := safe
	stale.SnapshotCreatedAt = now.Add(-6 * time.Minute)
	if err := CheckSnapshotRetention(stale); !errors.Is(err, ErrSnapshotRetentionUnsafe) {
		t.Fatalf("expected stale snapshot error, got %v", err)
	}

	shortRetention := safe
	shortRetention.EventRetention = 10 * time.Minute
	if err := CheckSnapshotRetention(shortRetention); !errors.Is(err, ErrSnapshotRetentionUnsafe) {
		t.Fatalf("expected short retention error, got %v", err)
	}
}
