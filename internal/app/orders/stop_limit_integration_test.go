package orders

import (
	"context"
	"testing"

	"exchange/internal/core/order"
)

func TestStopLimitTriggerAndFillIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TSTOP")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)

	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

	// 1. Fund users
	fundUser(t, repo, buyer, "USD", "1000") // Buy locks USD
	fundUser(t, repo, seller, base, "10")   // Sell locks base asset

	// 2. Place resting limit sell order at price 105
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "105", "2")
	assertOrderState(t, repo, ask.Order.ID, order.StatusOpen, "2")

	// 3. Place pending stop limit buy order for buyer (StopPrice = 100, Price = 105, Qty = 2)
	result, err := svc.Place(context.Background(), PlaceRequest{
		ClientOrderID: "cli-stop-buy",
		UserID:        buyer,
		Market:        marketSymbol,
		Side:          string(order.SideBuy),
		Type:          string(order.TypeStopLimit),
		Price:         "105",
		StopPrice:     "100",
		Quantity:      "2",
	})
	if err != nil {
		t.Fatalf("failed to place stop-limit buy: %v", err)
	}

	stopOrderID := result.Order.ID
	assertOrderState(t, repo, stopOrderID, order.StatusPendingStop, "2")

	// Check that locked USD balance for buyer is 105 * 2 = 210 USD
	assertLockedBalance(t, db, buyer, "USD", "210")

	// Check that this stop order is NOT in the book bids yet
	assertBookSide(t, svc, marketSymbol, order.SideBuy, nil)

	// 4. Trigger stops with LastPrice = 99 (should NOT trigger because buy stop triggers at >= 100)
	triggerResults, err := svc.TriggerStops(context.Background(), TriggerRequest{
		Market:    marketSymbol,
		LastPrice: "99",
	})
	if err != nil {
		t.Fatalf("failed to trigger stops: %v", err)
	}
	if len(triggerResults) != 0 {
		t.Fatalf("expected 0 triggered stop orders, got %d", len(triggerResults))
	}
	assertOrderState(t, repo, stopOrderID, order.StatusPendingStop, "2")

	// 5. Trigger stops with LastPrice = 100 (should trigger and immediately match limit sell at 105!)
	triggerResults, err = svc.TriggerStops(context.Background(), TriggerRequest{
		Market:    marketSymbol,
		LastPrice: "100",
	})
	if err != nil {
		t.Fatalf("failed to trigger stops: %v", err)
	}
	if len(triggerResults) != 1 {
		t.Fatalf("expected 1 triggered stop order result, got %d", len(triggerResults))
	}

	// 6. Assert stop order is now FILLED and the matched ask is also FILLED
	assertOrderState(t, repo, stopOrderID, order.StatusFilled, "0")
	assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")

	// 7. Verify balances:
	// - Buyer USD locked should be 0
	// - Buyer USD available should have decreased by 210 USD
	// - Buyer base balance available should have increased by 2
	assertLockedBalance(t, db, buyer, "USD", "0")
	assertNoNegativeBalances(t, db, buyer, seller)
}
