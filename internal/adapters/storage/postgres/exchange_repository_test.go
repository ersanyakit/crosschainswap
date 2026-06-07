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

func TestAggregateLevelsTracksFirstSequenceID(t *testing.T) {
	levels := aggregateLevels("PEPPER/USDC", order.SideBuy, []order.Order{
		{Market: "PEPPER/USDC", Side: order.SideBuy, Price: "1", RemainingQuantity: "2", SequenceID: 12},
		{Market: "PEPPER/USDC", Side: order.SideBuy, Price: "1", RemainingQuantity: "3", SequenceID: 7},
	})

	if len(levels) != 1 {
		t.Fatalf("expected one level, got %d", len(levels))
	}
	if levels[0].Quantity != "5" || levels[0].OrderCount != 2 || levels[0].FirstSequenceID != 7 {
		t.Fatalf("unexpected aggregate level: %#v", levels[0])
	}
}

func TestOrderModelMappingPreservesSequenceID(t *testing.T) {
	item := order.Order{
		ID:                "ord_1",
		ClientOrderID:     "client_1",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideBuy,
		Type:              order.TypeLimit,
		Status:            order.StatusOpen,
		TimeInForce:       order.TimeInForceGTC,
		Price:             "1",
		Quantity:          "2",
		RemainingQuantity: "2",
		SequenceID:        42,
	}

	model := orderToModel(item)
	roundTrip := modelToOrder(model)
	if model.SequenceID != 42 || roundTrip.SequenceID != 42 {
		t.Fatalf("sequence id was not preserved: model=%#v order=%#v", model, roundTrip)
	}
}
