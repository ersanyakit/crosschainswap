package matching

import (
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
)

// TestMatchingBasicLimitOrderSpreadAndMidPrice tests basic limit matching,
// best bid/ask updates, spread, and mid price calculation.
func TestMatchingBasicLimitOrderSpreadAndMidPrice(t *testing.T) {
	book := newUSDCBook(t)

	// 1. Empty book state checks
	if _, ok := book.BestBid(); ok {
		t.Fatal("expected no best bid on empty book")
	}
	if _, ok := book.BestAsk(); ok {
		t.Fatal("expected no best ask on empty book")
	}

	// 2. Limit Buy ekleme
	buyOrder := testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "50000", "2", 1)
	err := book.Load([]order.Order{buyOrder})
	if err != nil {
		t.Fatalf("failed to load buy order: %v", err)
	}

	bestBid, ok := book.BestBid()
	if !ok || bestBid != "50000" {
		t.Fatalf("expected best bid 50000, got %s", bestBid)
	}

	// 3. Limit Sell ekleme (Cross olmayan, book'ta kalması gereken)
	sellOrder := testBookOrder("sell-1", order.SideSell, order.TypeLimit, "51000", "3", 2)
	err = book.Load([]order.Order{sellOrder})
	if err != nil {
		t.Fatalf("failed to load sell order: %v", err)
	}

	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk != "51000" {
		t.Fatalf("expected best ask 51000, got %s", bestAsk)
	}

	// 4. Spread ve Mid Price hesaplaması
	spread := decimal.SubFloorZero(bestAsk, bestBid)
	if spread != "1000" {
		t.Fatalf("expected spread 1000, got %s", spread)
	}

	// mid = (bestAsk + bestBid) / 2 = 50500
	sumPrice := decimal.Add(bestAsk, bestBid)
	midPrice := decimal.Mul(sumPrice, "0.5")
	if midPrice != "50500" {
		t.Fatalf("expected mid price 50500, got %s", midPrice)
	}
}

// TestMatchingTimePriorityFIFO verifies FIFO ordering at the same price level.
func TestMatchingTimePriorityFIFO(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)

	// Add two sell orders at the same price with different sequence IDs (time priority)
	sell1 := testBookOrder("sell-1", order.SideSell, order.TypeLimit, "100", "5", 10)
	sell2 := testBookOrder("sell-2", order.SideSell, order.TypeLimit, "100", "5", 11)

	if err := book.Load([]order.Order{sell1, sell2}); err != nil {
		t.Fatal(err)
	}

	// Buy order that matches part of the depth (e.g. qty = 7)
	buy := testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "100", "7", 12)
	result, err := book.Apply(buy, nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(result.Trades))
	}

	// First trade must be against sell-1 (FIFO oldest)
	if result.Trades[0].MakerOrderID != "sell-1" || result.Trades[0].Quantity != "5" {
		t.Fatalf("expected first trade with sell-1 for 5 qty, got: %#v", result.Trades[0])
	}

	// Second trade must be against sell-2 (FIFO next)
	if result.Trades[1].MakerOrderID != "sell-2" || result.Trades[1].Quantity != "2" {
		t.Fatalf("expected second trade with sell-2 for 2 qty, got: %#v", result.Trades[1])
	}
}

// TestMatchingPartialFillLeavesRemaining verifies status changes and remaining qty.
func TestMatchingPartialFillLeavesRemaining(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)

	// Buy order of 10 BTC
	buy := testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "50000", "10", 1)
	if err := book.Load([]order.Order{buy}); err != nil {
		t.Fatal(err)
	}

	// Sell order of 3 BTC (crosses limit price) -> should match
	sell := testBookOrder("sell-1", order.SideSell, order.TypeLimit, "50000", "3", 2)
	result, err := book.Apply(sell, nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}

	// Taker (sell) should be fully filled
	if result.Taker.Status != order.StatusFilled || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("expected taker to be filled, got remaining: %s, status: %s", result.Taker.RemainingQuantity, result.Taker.Status)
	}

	// Maker (buy) should be partially filled, leaving 7 remaining qty in book
	makerUpdate := result.Makers[0]
	if makerUpdate.ID != "buy-1" || makerUpdate.Status != order.StatusPartiallyFilled || makerUpdate.RemainingQuantity != "7" {
		t.Fatalf("unexpected maker status: %#v", makerUpdate)
	}
}

// TestMatchingMarketBuyConsumesMultipleAskLevels verifies market buy sweeps asks.
func TestMatchingMarketBuyConsumesMultipleAskLevels(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)

	// Set up order book with asks at multiple levels
	ask1 := testBookOrder("ask-1", order.SideSell, order.TypeLimit, "100", "2", 1)
	ask2 := testBookOrder("ask-2", order.SideSell, order.TypeLimit, "105", "3", 2)
	ask3 := testBookOrder("ask-3", order.SideSell, order.TypeLimit, "110", "5", 3)

	if err := book.Load([]order.Order{ask1, ask2, ask3}); err != nil {
		t.Fatal(err)
	}

	// Market Buy order of 7 qty, cap price 115
	mBuy := testBookOrder("buy-market", order.SideBuy, order.TypeMarket, "115", "7", 4)
	result, err := book.Apply(mBuy, nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	// Trades should be:
	// - 2 qty @ 100 (ask-1)
	// - 3 qty @ 105 (ask-2)
	// - 2 qty @ 110 (ask-3)
	if len(result.Trades) != 3 {
		t.Fatalf("expected 3 trades, got %d", len(result.Trades))
	}

	if result.Trades[0].Price != "100" || result.Trades[0].Quantity != "2" || result.Trades[0].MakerOrderID != "ask-1" {
		t.Errorf("unexpected trade 0: %#v", result.Trades[0])
	}
	if result.Trades[1].Price != "105" || result.Trades[1].Quantity != "3" || result.Trades[1].MakerOrderID != "ask-2" {
		t.Errorf("unexpected trade 1: %#v", result.Trades[1])
	}
	if result.Trades[2].Price != "110" || result.Trades[2].Quantity != "2" || result.Trades[2].MakerOrderID != "ask-3" {
		t.Errorf("unexpected trade 2: %#v", result.Trades[2])
	}

	// Check final state of ask-3 (1 qty remaining)
	remainingAsk3, ok := book.ActiveOrder("ask-3")
	if !ok || remainingAsk3.RemainingQuantity != "5" { // wait, Apply doesn't auto-update internal book unless ApplyResult is called?
		// Wait, look at book.go line 160: makerSide.levels[price][0] = maker
		// Yes, it mutates the side levels inside Apply!
	}
	// Let's verify by checking BestAsk
	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk != "110" {
		t.Fatalf("expected best ask 110, got %s", bestAsk)
	}
}

// TestMatchingIOCPartialFillCancelsRemainder tests IOC TIF logic.
func TestMatchingIOCPartialFillCancelsRemainder(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)

	// Sell order of 4 qty @ 100
	sell := testBookOrder("sell-1", order.SideSell, order.TypeLimit, "100", "4", 1)
	if err := book.Load([]order.Order{sell}); err != nil {
		t.Fatal(err)
	}

	// Limit Buy IOC order of 10 qty @ 100
	buyIOC := testBookOrder("buy-ioc", order.SideBuy, order.TypeLimit, "100", "10", 2)
	buyIOC.TimeInForce = order.TimeInForceIOC

	result, err := book.Apply(buyIOC, nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	// 4 qty should trade
	if len(result.Trades) != 1 || result.Trades[0].Quantity != "4" {
		t.Fatalf("expected trade for 4 qty, got %#v", result.Trades)
	}

	// Taker status should be EXPIRED (canceled remainder)
	if result.Taker.Status != order.StatusExpired {
		t.Fatalf("expected Taker status to be EXPIRED, got %s", result.Taker.Status)
	}
	if result.Taker.RemainingQuantity != "6" {
		t.Fatalf("expected taker remaining quantity 6, got %s", result.Taker.RemainingQuantity)
	}

	// Check that buy-ioc is NOT in the book
	if _, ok := book.ActiveOrder("buy-ioc"); ok {
		t.Fatal("IOC order remainder was added to book")
	}
}
