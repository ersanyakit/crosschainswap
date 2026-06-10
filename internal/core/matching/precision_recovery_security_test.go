package matching

import (
	"fmt"
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
	"exchange/internal/core/trade"
)

func TestDustPartialFillsConserveExactQuantity(t *testing.T) {
	// Arrange.
	now := time.Unix(30, 0).UTC()
	makers := make([]order.Order, 0, 10_000)
	for i := 0; i < 10_000; i++ {
		makers = append(makers, order.Order{
			ID:                order.ID(fmt.Sprintf("maker-%05d", i)),
			UserID:            fmt.Sprintf("seller-%05d", i),
			Market:            "BTC/USDT",
			BaseAsset:         "BTC",
			QuoteAsset:        "USDT",
			Side:              order.SideSell,
			Status:            order.StatusOpen,
			Price:             "1",
			FilledQuantity:    "0",
			RemainingQuantity: "0.00000001",
		})
	}
	taker := order.Order{
		ID:                "taker",
		UserID:            "buyer",
		Market:            "BTC/USDT",
		BaseAsset:         "BTC",
		QuoteAsset:        "USDT",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "1",
		FilledQuantity:    "0",
		RemainingQuantity: "0.0001",
	}

	// Act.
	var seq int
	result, err := MatchLimit(taker, makers, func() trade.ID {
		seq++
		return trade.ID(fmt.Sprintf("trade-%05d", seq))
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	// Assert.
	if len(result.Trades) != 10_000 {
		t.Fatalf("trade count = %d, want 10000", len(result.Trades))
	}
	if result.Taker.Status != order.StatusFilled || result.Taker.FilledQuantity != "0.0001" || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("unexpected taker dust accounting: %#v", result.Taker)
	}
	totalQty := "0"
	totalQuote := "0"
	for _, item := range result.Trades {
		totalQty = decimal.Add(totalQty, item.Quantity)
		totalQuote = decimal.Add(totalQuote, item.QuoteQuantity)
	}
	if totalQty != "0.0001" || totalQuote != "0.0001" {
		t.Fatalf("dust totals qty=%s quote=%s, want 0.0001/0.0001", totalQty, totalQuote)
	}
	for _, maker := range result.Makers {
		if maker.Status != order.StatusFilled || maker.RemainingQuantity != "0" {
			t.Fatalf("dust maker not fully depleted: %#v", maker)
		}
	}
}

func TestSnapshotJournalReplayIsDeterministic(t *testing.T) {
	// Arrange.
	now := time.Unix(40, 0).UTC()
	live := newUSDCBook(t)
	if err := live.Load([]order.Order{
		testBookOrder("bid-99", order.SideBuy, order.TypeLimit, "99", "4", 1),
		testBookOrder("ask-101", order.SideSell, order.TypeLimit, "101", "5", 2),
		testBookOrder("ask-102", order.SideSell, order.TypeLimit, "102", "5", 3),
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := live.CaptureState(3, now)
	commands := []order.Order{
		testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "102", "6", 4),
		testBookOrder("sell-1", order.SideSell, order.TypeLimit, "99", "6", 5),
		testBookOrder("buy-2", order.SideBuy, order.TypeLimit, "100", "2", 6),
	}
	results := make([]Result, 0, len(commands))

	// Act.
	for i, command := range commands {
		result, err := live.Apply(command, nextBookTradeID(), now.Add(time.Duration(i+1)*time.Second))
		if err != nil {
			t.Fatalf("live apply %d failed: %v", i, err)
		}
		results = append(results, result)
	}
	replayed, err := RestoreMarketBook(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for i, result := range results {
		if err := replayed.ApplyResult(result); err != nil {
			t.Fatalf("replay result %d failed: %v", i, err)
		}
	}

	// Assert.
	liveSig := bookSnapshotSignature(live.Snapshot(0))
	replaySig := bookSnapshotSignature(replayed.Snapshot(0))
	if liveSig != replaySig {
		t.Fatalf("replayed book diverged\nlive:   %s\nreplay: %s", liveSig, replaySig)
	}
}

func bookSnapshotSignature(snapshot orderbook.Snapshot) string {
	out := "bids:"
	for _, level := range snapshot.Bids {
		out += fmt.Sprintf("%s@%s#%d|", level.Quantity, level.Price, level.OrderCount)
	}
	out += "asks:"
	for _, level := range snapshot.Asks {
		out += fmt.Sprintf("%s@%s#%d|", level.Quantity, level.Price, level.OrderCount)
	}
	return out
}
