package matching

import (
	"testing"
	"time"

	"exchange/internal/core/order"
	"exchange/internal/core/trade"
)

func TestMatchLimitRespectsPriceTimeAndSelfTradePrevention(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "10",
		RemainingQuantity: "7",
	}
	makers := []order.Order{
		{ID: "self", UserID: "u1", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "9", RemainingQuantity: "2"},
		{ID: "m1", UserID: "u2", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "9", RemainingQuantity: "5"},
		{ID: "m2", UserID: "u3", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "10", RemainingQuantity: "5"},
	}

	var seq int
	result, err := MatchLimit(taker, makers, func() trade.ID {
		seq++
		return trade.ID("trd")
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(result.Trades))
	}
	if result.Trades[0].MakerOrderID != "m1" || result.Trades[0].Price != "9" || result.Trades[0].Quantity != "5" {
		t.Fatalf("unexpected first trade: %#v", result.Trades[0])
	}
	if result.Trades[1].MakerOrderID != "m2" || result.Trades[1].Price != "10" || result.Trades[1].Quantity != "2" {
		t.Fatalf("unexpected second trade: %#v", result.Trades[1])
	}
	if result.Taker.Status != order.StatusFilled || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("unexpected taker result: %#v", result.Taker)
	}
	if len(result.Makers) != 2 || result.Makers[0].Status != order.StatusFilled || result.Makers[1].RemainingQuantity != "3" {
		t.Fatalf("unexpected maker updates: %#v", result.Makers)
	}
}

func TestMatchLimitDoesNotCrossOutsideLimitPrice(t *testing.T) {
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideSell,
		Status:            order.StatusOpen,
		Price:             "10",
		RemainingQuantity: "1",
	}
	makers := []order.Order{
		{ID: "m1", UserID: "u2", Market: "PEPPER/USDC", Side: order.SideBuy, Status: order.StatusOpen, Price: "9.99", RemainingQuantity: "1"},
	}

	result, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 0 || result.Taker.Status != order.StatusOpen || result.Taker.RemainingQuantity != "1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestMatchLimitBuyDoesNotTradeAboveLimit(t *testing.T) {
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "10",
		RemainingQuantity: "1",
	}
	makers := []order.Order{
		{ID: "m1", UserID: "u2", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "10.000000000000000001", RemainingQuantity: "1"},
	}

	result, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 0 {
		t.Fatalf("expected no trades above buy limit, got %#v", result.Trades)
	}
}
