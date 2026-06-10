package matching

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
)

func TestOrderBookSortsBidsAndAsksByPrice(t *testing.T) {
	// Arrange.
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("bid-09", order.SideBuy, order.TypeLimit, "9", "1", 3),
		testBookOrder("bid-11", order.SideBuy, order.TypeLimit, "11", "1", 1),
		testBookOrder("bid-10", order.SideBuy, order.TypeLimit, "10", "1", 2),
		testBookOrder("ask-13", order.SideSell, order.TypeLimit, "13", "1", 6),
		testBookOrder("ask-12", order.SideSell, order.TypeLimit, "12", "1", 4),
		testBookOrder("ask-14", order.SideSell, order.TypeLimit, "14", "1", 5),
	}); err != nil {
		t.Fatal(err)
	}

	// Act.
	snapshot := book.Snapshot(10)

	// Assert.
	assertSnapshotPrices(t, snapshot.Bids, []string{"11", "10", "9"}, "bids")
	assertSnapshotPrices(t, snapshot.Asks, []string{"12", "13", "14"}, "asks")
	assertMarketBookInvariants(t, book)
}

func TestOrderBookNeverCrossed(t *testing.T) {
	// Arrange.
	now := time.Unix(20, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-100", order.SideSell, order.TypeLimit, "100", "1", 1),
		testBookOrder("ask-101", order.SideSell, order.TypeLimit, "101", "1", 2),
	}); err != nil {
		t.Fatal(err)
	}

	// Act.
	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "100", "1.5", 3), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	// Assert.
	if len(result.Trades) != 1 || result.Trades[0].Price != "100" || result.Trades[0].Quantity != "1" {
		t.Fatalf("unexpected trades: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusPartiallyFilled || result.Taker.RemainingQuantity != "0.5" {
		t.Fatalf("unexpected resting taker: %#v", result.Taker)
	}
	bestBid, bidOK := book.BestBid()
	bestAsk, askOK := book.BestAsk()
	if !bidOK || !askOK || decimal.Cmp(bestBid, bestAsk) >= 0 {
		t.Fatalf("book crossed or missing sides: best bid %s/%v best ask %s/%v", bestBid, bidOK, bestAsk, askOK)
	}
	assertMarketBookInvariants(t, book)
}

func TestCancelPreventsFutureMatch(t *testing.T) {
	// Arrange.
	now := time.Unix(21, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-1", order.SideSell, order.TypeLimit, "100", "1", 1),
	}); err != nil {
		t.Fatal(err)
	}

	// Act.
	canceled, ok := book.Cancel("ask-1", now)
	if !ok {
		t.Fatalf("cancel should remove active ask")
	}
	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "100", "1", 2), nextBookTradeID(), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// Assert.
	if canceled.Status != order.StatusCanceled {
		t.Fatalf("cancel status = %s, want canceled", canceled.Status)
	}
	if len(result.Trades) != 0 {
		t.Fatalf("canceled order matched again: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusExpired {
		t.Fatalf("unfilled market order status = %s, want expired", result.Taker.Status)
	}
	assertMarketBookInvariants(t, book)
}

func TestSelfTradeIsPreventedWithExpireTaker(t *testing.T) {
	// Arrange.
	now := time.Unix(22, 0).UTC()
	book := newUSDCBook(t)
	ask := testBookOrder("ask-1", order.SideSell, order.TypeLimit, "100", "1", 1)
	ask.UserID = "user-a"
	if err := book.Load([]order.Order{ask}); err != nil {
		t.Fatal(err)
	}
	buy := testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "100", "1", 2)
	buy.UserID = "user-a"

	// Act.
	result, err := book.Apply(buy, nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	// Assert.
	if len(result.Trades) != 0 {
		t.Fatalf("self-trade must not produce trades: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusExpired || result.Taker.RemainingQuantity != "1" {
		t.Fatalf("self-crossing taker should expire without fill: %#v", result.Taker)
	}
	if _, ok := book.ActiveOrder("ask-1"); !ok {
		t.Fatalf("self-trade prevention should leave maker resting")
	}
	if _, ok := book.ActiveOrder("buy-1"); ok {
		t.Fatalf("expired self-crossing taker must not rest")
	}
	assertMarketBookInvariants(t, book)
}

func TestMarketBookRandomFlowMaintainsInvariants(t *testing.T) {
	// Arrange.
	rng := rand.New(rand.NewSource(20260610))
	book := newUSDCBook(t)
	activeIDs := make([]order.ID, 0, 256)
	now := time.Unix(23, 0).UTC()

	// Act.
	for i := 0; i < 250; i++ {
		switch rng.Intn(5) {
		case 0:
			if len(activeIDs) == 0 {
				continue
			}
			id := activeIDs[rng.Intn(len(activeIDs))]
			book.Cancel(id, now.Add(time.Duration(i)*time.Millisecond))
		default:
			side := order.SideBuy
			if rng.Intn(2) == 0 {
				side = order.SideSell
			}
			typ := order.TypeLimit
			if rng.Intn(5) == 0 {
				typ = order.TypeMarket
			}
			price := fixed6(1_000_000 + int64(rng.Intn(100))*10_000)
			if typ == order.TypeMarket && side == order.SideSell {
				price = "0"
			}
			qty := fixed6(1 + int64(rng.Intn(500_000)))
			item := testBookOrder(fmt.Sprintf("ord-%03d", i), side, typ, price, qty, uint64(i+1))
			item.UserID = fmt.Sprintf("user-%d", rng.Intn(25))
			result, err := book.Apply(item, nextBookTradeID(), now.Add(time.Duration(i)*time.Millisecond))
			if err != nil {
				t.Fatalf("apply random order %d failed: %v", i, err)
			}
			if _, ok := book.ActiveOrder(result.Taker.ID); ok {
				activeIDs = append(activeIDs, result.Taker.ID)
			}
		}
		assertMarketBookInvariants(t, book)
	}

	// Assert.
	assertMarketBookInvariants(t, book)
}

func FuzzMarketBookNeverCrossed(f *testing.F) {
	f.Add(uint64(1), uint8(20))
	f.Add(uint64(99), uint8(40))

	f.Fuzz(func(t *testing.T, seed uint64, steps uint8) {
		// Arrange.
		rng := rand.New(rand.NewSource(int64(seed)))
		book := newUSDCBook(t)
		now := time.Unix(24, 0).UTC()
		stepCount := int(steps%64) + 1

		// Act.
		for i := 0; i < stepCount; i++ {
			side := order.SideBuy
			if rng.Intn(2) == 0 {
				side = order.SideSell
			}
			typ := order.TypeLimit
			if rng.Intn(4) == 0 {
				typ = order.TypeMarket
			}
			price := fixed6(1_000_000 + int64(rng.Intn(100))*10_000)
			if typ == order.TypeMarket && side == order.SideSell {
				price = "0"
			}
			qty := fixed6(1 + int64(rng.Intn(750_000)))
			item := testBookOrder(fmt.Sprintf("fuzz-%d", i), side, typ, price, qty, uint64(i+1))
			if _, err := book.Apply(item, nextBookTradeID(), now.Add(time.Duration(i)*time.Millisecond)); err != nil {
				t.Fatalf("fuzz apply %d failed: %v", i, err)
			}
			assertMarketBookInvariants(t, book)
		}

		// Assert.
		assertMarketBookInvariants(t, book)
	})
}

func assertSnapshotPrices(t *testing.T, got []orderbook.PriceLevel, want []string, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s prices = %#v, want %v", label, got, want)
	}
	for i := range want {
		if decimal.Cmp(got[i].Price, want[i]) != 0 {
			t.Fatalf("%s price %d = %s, want %s", label, i, got[i].Price, want[i])
		}
	}
}

func assertMarketBookInvariants(t *testing.T, book *MarketBook) {
	t.Helper()
	snapshot := book.Snapshot(0)
	assertBookSideLevels(t, snapshot.Bids, order.SideBuy)
	assertBookSideLevels(t, snapshot.Asks, order.SideSell)
	if bestBid, bidOK := book.BestBid(); bidOK {
		if bestAsk, askOK := book.BestAsk(); askOK && decimal.Cmp(bestBid, bestAsk) >= 0 {
			t.Fatalf("crossed book: best bid %s best ask %s snapshot=%#v", bestBid, bestAsk, snapshot)
		}
	}
}

func assertBookSideLevels(t *testing.T, levels []orderbook.PriceLevel, side order.Side) {
	t.Helper()
	for i, level := range levels {
		if level.Side != side {
			t.Fatalf("level side = %s, want %s: %#v", level.Side, side, level)
		}
		if decimal.Cmp(level.Price, "0") <= 0 || decimal.Cmp(level.Quantity, "0") <= 0 {
			t.Fatalf("non-positive level: %#v", level)
		}
		if level.OrderCount <= 0 {
			t.Fatalf("level order count must be positive: %#v", level)
		}
		if i == 0 {
			continue
		}
		cmp := decimal.Cmp(levels[i-1].Price, level.Price)
		if side == order.SideBuy && cmp <= 0 {
			t.Fatalf("bids not strictly descending: %#v", levels)
		}
		if side == order.SideSell && cmp >= 0 {
			t.Fatalf("asks not strictly ascending: %#v", levels)
		}
	}
}
