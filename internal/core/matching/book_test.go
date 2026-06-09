package matching

import (
	"fmt"
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"
)

func TestMarketBookMarketBuyFillsSingleAskAndRemovesLevel(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	maker := testBookOrder("ask-1", order.SideSell, order.TypeLimit, "100", "5", 1)
	if err := book.Load([]order.Order{maker}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "100", "5", 2), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("expected one trade, got %d", len(result.Trades))
	}
	if result.Trades[0].MakerOrderID != "ask-1" || result.Trades[0].Price != "100" || result.Trades[0].Quantity != "5" {
		t.Fatalf("unexpected trade: %#v", result.Trades[0])
	}
	if result.Taker.Status != order.StatusFilled || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("market buy should be filled: %#v", result.Taker)
	}
	if len(result.Makers) != 1 || result.Makers[0].Status != order.StatusFilled || result.Makers[0].RemainingQuantity != "0" {
		t.Fatalf("maker should be filled: %#v", result.Makers)
	}
	if _, ok := book.ActiveOrder("ask-1"); ok {
		t.Fatalf("filled maker stayed active")
	}
	if _, ok := book.BestAsk(); ok {
		t.Fatalf("ask level was not removed")
	}
	if book.ActiveOrderCount() != 0 {
		t.Fatalf("active book should be empty, got %d", book.ActiveOrderCount())
	}
}

func TestMarketBookMarketBuyPartiallyConsumesSecondAsk(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-100", order.SideSell, order.TypeLimit, "100", "5", 1),
		testBookOrder("ask-101", order.SideSell, order.TypeLimit, "101", "5", 2),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "101", "7", 3), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 2 {
		t.Fatalf("expected two trades, got %d", len(result.Trades))
	}
	if result.Trades[0].Price != "100" || result.Trades[0].Quantity != "5" {
		t.Fatalf("first trade should consume best ask: %#v", result.Trades[0])
	}
	if result.Trades[1].Price != "101" || result.Trades[1].Quantity != "2" {
		t.Fatalf("second trade should partially consume next ask: %#v", result.Trades[1])
	}
	if !result.Trades[0].CreatedAt.Equal(now) || !result.Trades[1].CreatedAt.Equal(now.Add(time.Microsecond)) {
		t.Fatalf("trade timestamps must preserve book match order: %#v", result.Trades)
	}
	if _, ok := book.ActiveOrder("ask-100"); ok {
		t.Fatalf("filled 100 ask stayed active")
	}
	remaining, ok := book.ActiveOrder("ask-101")
	if !ok {
		t.Fatalf("partial 101 ask missing")
	}
	if remaining.Status != order.StatusPartiallyFilled || remaining.RemainingQuantity != "3" {
		t.Fatalf("unexpected 101 ask remainder: %#v", remaining)
	}
	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk != "101" {
		t.Fatalf("best ask = %s/%v, want 101/true", bestAsk, ok)
	}
	snapshot := book.Snapshot(10)
	if len(snapshot.Asks) != 1 || snapshot.Asks[0].Price != "101" || snapshot.Asks[0].Quantity != "3" || snapshot.Asks[0].OrderCount != 1 {
		t.Fatalf("unexpected ask snapshot: %#v", snapshot.Asks)
	}
}

func TestMarketBookMarketSellFillsSingleBidAndRemovesLevel(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "99", "5", 1)}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("sell-1", order.SideSell, order.TypeMarket, "", "5", 2), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 1 || result.Trades[0].MakerOrderID != "bid-1" || result.Trades[0].Price != "99" || result.Trades[0].Quantity != "5" {
		t.Fatalf("unexpected trades: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusFilled || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("market sell should be filled: %#v", result.Taker)
	}
	if _, ok := book.ActiveOrder("bid-1"); ok {
		t.Fatalf("filled bid stayed active")
	}
	if _, ok := book.BestBid(); ok {
		t.Fatalf("bid level was not removed")
	}
}

func TestMarketBookMarketSellPartiallyConsumesSecondBid(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("bid-99", order.SideBuy, order.TypeLimit, "99", "5", 1),
		testBookOrder("bid-98", order.SideBuy, order.TypeLimit, "98", "5", 2),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("sell-1", order.SideSell, order.TypeMarket, "", "7", 3), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 2 {
		t.Fatalf("expected two trades, got %d", len(result.Trades))
	}
	if result.Trades[0].Price != "99" || result.Trades[0].Quantity != "5" {
		t.Fatalf("first trade should consume best bid: %#v", result.Trades[0])
	}
	if result.Trades[1].Price != "98" || result.Trades[1].Quantity != "2" {
		t.Fatalf("second trade should partially consume next bid: %#v", result.Trades[1])
	}
	if _, ok := book.ActiveOrder("bid-99"); ok {
		t.Fatalf("filled 99 bid stayed active")
	}
	remaining, ok := book.ActiveOrder("bid-98")
	if !ok {
		t.Fatalf("partial 98 bid missing")
	}
	if remaining.Status != order.StatusPartiallyFilled || remaining.RemainingQuantity != "3" {
		t.Fatalf("unexpected 98 bid remainder: %#v", remaining)
	}
	bestBid, ok := book.BestBid()
	if !ok || bestBid != "98" {
		t.Fatalf("best bid = %s/%v, want 98/true", bestBid, ok)
	}
}

func TestMarketBookMarketOrderExpiresUnfilledRemainderWithoutResting(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{testBookOrder("ask-1", order.SideSell, order.TypeLimit, "100", "3", 1)}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "100", "10", 2), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 1 || result.Trades[0].Quantity != "3" {
		t.Fatalf("expected 3 units to trade, got %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusExpired || result.Taker.FilledQuantity != "3" || result.Taker.RemainingQuantity != "7" {
		t.Fatalf("market order remainder should expire: %#v", result.Taker)
	}
	if _, ok := book.ActiveOrder("buy-1"); ok {
		t.Fatalf("market order was added to book")
	}
	if book.ActiveOrderCount() != 0 {
		t.Fatalf("active book should be empty, got %d", book.ActiveOrderCount())
	}
}

func TestMarketBookLimitOrderRestsExactRemainderAfterCrossing(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "0.5", "95", 1)}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("sell-1", order.SideSell, order.TypeLimit, "0.5", "100", 2), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 1 || result.Trades[0].Quantity != "95" || result.Trades[0].Price != "0.5" {
		t.Fatalf("unexpected trade: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusPartiallyFilled || result.Taker.FilledQuantity != "95" || result.Taker.RemainingQuantity != "5" {
		t.Fatalf("taker should keep exact remainder: %#v", result.Taker)
	}
	if _, ok := book.ActiveOrder("bid-1"); ok {
		t.Fatalf("filled bid stayed active")
	}
	resting, ok := book.ActiveOrder("sell-1")
	if !ok {
		t.Fatalf("remaining sell order did not rest")
	}
	if resting.Side != order.SideSell || resting.Price != "0.5" || resting.RemainingQuantity != "5" {
		t.Fatalf("unexpected resting sell remainder: %#v", resting)
	}
	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk != "0.5" {
		t.Fatalf("best ask = %s/%v, want 0.5/true", bestAsk, ok)
	}
}

func TestMarketBookLimitBuyRestsRemainderAfterSweepingAsks(t *testing.T) {
	now := time.Unix(11, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-100", order.SideSell, order.TypeLimit, "100", "5", 1),
		testBookOrder("ask-101", order.SideSell, order.TypeLimit, "101", "10", 2),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "101", "16", 3), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 2 || result.Trades[0].Quantity != "5" || result.Trades[1].Quantity != "10" {
		t.Fatalf("unexpected trades: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusPartiallyFilled || result.Taker.FilledQuantity != "15" || result.Taker.RemainingQuantity != "1" {
		t.Fatalf("taker should rest exact buy remainder: %#v", result.Taker)
	}
	if _, ok := book.ActiveOrder("ask-100"); ok {
		t.Fatalf("filled ask-100 stayed active")
	}
	if _, ok := book.ActiveOrder("ask-101"); ok {
		t.Fatalf("filled ask-101 stayed active")
	}
	resting, ok := book.ActiveOrder("buy-1")
	if !ok {
		t.Fatalf("remaining buy order did not rest")
	}
	if resting.Side != order.SideBuy || resting.Price != "101" || resting.RemainingQuantity != "1" {
		t.Fatalf("unexpected resting buy remainder: %#v", resting)
	}
	bestBid, ok := book.BestBid()
	if !ok || bestBid != "101" {
		t.Fatalf("best bid = %s/%v, want 101/true", bestBid, ok)
	}
	if bestAsk, ok := book.BestAsk(); ok {
		t.Fatalf("best ask = %s/%v, want empty", bestAsk, ok)
	}
}

func TestMarketBookLimitBuyAmountRestsRemainderAfterSweepingAsks(t *testing.T) {
	now := time.Unix(12, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-1", order.SideSell, order.TypeLimit, "1", "1", 1),
		testBookOrder("ask-2", order.SideSell, order.TypeLimit, "2", "1", 2),
		testBookOrder("ask-3", order.SideSell, order.TypeLimit, "3", "1", 3),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeLimit, "3", "4", 4), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 3 {
		t.Fatalf("trade count = %d, want 3: %#v", len(result.Trades), result.Trades)
	}
	for idx, expectedPrice := range []string{"1", "2", "3"} {
		if result.Trades[idx].Price != expectedPrice || result.Trades[idx].Quantity != "1" {
			t.Fatalf("trade %d = %s@%s, want 1@%s", idx, result.Trades[idx].Quantity, result.Trades[idx].Price, expectedPrice)
		}
	}
	if result.Taker.Status != order.StatusPartiallyFilled || result.Taker.FilledQuantity != "3" || result.Taker.RemainingQuantity != "1" {
		t.Fatalf("taker should fill 3 and rest 1: %#v", result.Taker)
	}
	for _, askID := range []order.ID{"ask-1", "ask-2", "ask-3"} {
		if _, ok := book.ActiveOrder(askID); ok {
			t.Fatalf("filled ask %s stayed active", askID)
		}
	}
	resting, ok := book.ActiveOrder("buy-1")
	if !ok {
		t.Fatalf("remaining buy order did not rest")
	}
	if resting.Side != order.SideBuy || resting.Price != "3" || resting.RemainingQuantity != "1" {
		t.Fatalf("unexpected resting buy remainder: %#v", resting)
	}
	bestBid, ok := book.BestBid()
	if !ok || bestBid != "3" {
		t.Fatalf("best bid = %s/%v, want 3/true", bestBid, ok)
	}
	if bestAsk, ok := book.BestAsk(); ok {
		t.Fatalf("best ask = %s/%v, want empty", bestAsk, ok)
	}
}

func TestMarketBookApplyResultReplaysBookMutation(t *testing.T) {
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "99", "5", 1),
		testBookOrder("ask-1", order.SideSell, order.TypeLimit, "101", "5", 2),
	}); err != nil {
		t.Fatal(err)
	}

	replayedBid := testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "99", "5", 1)
	replayedBid.FilledQuantity = "2"
	replayedBid.RemainingQuantity = "3"
	replayedBid.Status = order.StatusPartiallyFilled
	replayedAsk := testBookOrder("ask-1", order.SideSell, order.TypeLimit, "101", "5", 2)
	replayedAsk.FilledQuantity = "5"
	replayedAsk.RemainingQuantity = "0"
	replayedAsk.Status = order.StatusFilled
	replayedTaker := testBookOrder("sell-1", order.SideSell, order.TypeLimit, "99", "4", 3)
	replayedTaker.FilledQuantity = "2"
	replayedTaker.RemainingQuantity = "2"
	replayedTaker.Status = order.StatusPartiallyFilled

	if err := book.ApplyResult(Result{
		Taker:  replayedTaker,
		Makers: []order.Order{replayedBid, replayedAsk},
		Trades: []trade.Trade{{ID: "trd-1", MakerOrderID: "bid-1", TakerOrderID: "sell-1", Price: "99", Quantity: "2"}},
	}); err != nil {
		t.Fatal(err)
	}

	bid, ok := book.ActiveOrder("bid-1")
	if !ok || bid.RemainingQuantity != "3" || bid.Status != order.StatusPartiallyFilled {
		t.Fatalf("partial maker was not replayed: %#v", bid)
	}
	if _, ok := book.ActiveOrder("ask-1"); ok {
		t.Fatalf("filled maker stayed active after replay")
	}
	taker, ok := book.ActiveOrder("sell-1")
	if !ok || taker.RemainingQuantity != "2" || taker.Side != order.SideSell {
		t.Fatalf("taker remainder was not replayed: %#v", taker)
	}
}

func TestMarketBookCancelRemovesOrderAndPriceLevel(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{testBookOrder("bid-1", order.SideBuy, order.TypeLimit, "99", "5", 1)}); err != nil {
		t.Fatal(err)
	}

	canceled, ok := book.Cancel("bid-1", now)
	if !ok {
		t.Fatalf("expected cancel to succeed")
	}
	if canceled.Status != order.StatusCanceled {
		t.Fatalf("unexpected canceled order: %#v", canceled)
	}
	if _, ok := book.ActiveOrder("bid-1"); ok {
		t.Fatalf("canceled order stayed active")
	}
	if _, ok := book.BestBid(); ok {
		t.Fatalf("empty bid level stayed indexed")
	}
	if _, ok := book.Cancel("bid-1", now); ok {
		t.Fatalf("second cancel should not mutate active book")
	}
}

func TestMarketBookRespectsFIFOAtSamePrice(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-2", order.SideSell, order.TypeLimit, "100", "1", 20),
		testBookOrder("ask-1", order.SideSell, order.TypeLimit, "100", "1", 10),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "100", "2", 30), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 2 {
		t.Fatalf("expected two trades, got %d", len(result.Trades))
	}
	if result.Trades[0].MakerOrderID != "ask-1" || result.Trades[1].MakerOrderID != "ask-2" {
		t.Fatalf("same-price queue was not FIFO by sequence: %#v", result.Trades)
	}
}

func TestMarketBookNeverCreatesDecimalDustRemainder(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	book := newUSDCBook(t)
	if err := book.Load([]order.Order{
		testBookOrder("ask-1", order.SideSell, order.TypeLimit, "1", "0.333333333333333333", 1),
		testBookOrder("ask-2", order.SideSell, order.TypeLimit, "1", "0.333333333333333333", 2),
		testBookOrder("ask-3", order.SideSell, order.TypeLimit, "1", "0.333333333333333334", 3),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := book.Apply(testBookOrder("buy-1", order.SideBuy, order.TypeMarket, "1", "1", 4), nextBookTradeID(), now)
	if err != nil {
		t.Fatal(err)
	}

	if result.Taker.Status != order.StatusFilled || result.Taker.RemainingQuantity != "0" || result.Taker.FilledQuantity != "1" {
		t.Fatalf("unexpected exact decimal taker: %#v", result.Taker)
	}
	sum := "0"
	for _, item := range result.Trades {
		sum = decimal.Add(sum, item.Quantity)
	}
	if sum != "1" {
		t.Fatalf("trade quantity sum = %s, want 1", sum)
	}
	if book.ActiveOrderCount() != 0 {
		t.Fatalf("all asks should be removed, got %d active orders", book.ActiveOrderCount())
	}
}

func newUSDCBook(t *testing.T) *MarketBook {
	t.Helper()
	return NewMarketBook("USDC/USD", "USDC", "USD")
}

func testBookOrder(id string, side order.Side, typ order.Type, price string, qty string, seq uint64) order.Order {
	return order.Order{
		ID:                order.ID(id),
		ClientOrderID:     order.ClientOrderID("cl-" + id),
		UserID:            "user-" + id,
		Market:            "USDC/USD",
		BaseAsset:         "USDC",
		QuoteAsset:        "USD",
		Side:              side,
		Type:              typ,
		Status:            order.StatusOpen,
		TimeInForce:       order.TimeInForceGTC,
		Price:             price,
		Quantity:          qty,
		FilledQuantity:    "0",
		RemainingQuantity: qty,
		SequenceID:        seq,
		CreatedAt:         time.Unix(int64(seq), 0).UTC(),
		UpdatedAt:         time.Unix(int64(seq), 0).UTC(),
	}
}

func nextBookTradeID() TradeIDFactory {
	var seq int
	return func() trade.ID {
		seq++
		return trade.ID(fmt.Sprintf("trd-%d", seq))
	}
}
