package postgres

import (
	"testing"

	"exchange/internal/core/order"
)

func TestUniquePriceLevelKeysDeduplicatesByMarketSidePrice(t *testing.T) {
	keys := uniquePriceLevelKeys([]PriceLevelKey{
		{Market: "PEPPER/USDC", Side: order.SideBuy, Price: "1"},
		{Market: "PEPPER/USDC", Side: order.SideBuy, Price: "1"},
		{Market: "PEPPER/USDC", Side: order.SideSell, Price: "1"},
	})

	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %#v", len(keys), keys)
	}
}
