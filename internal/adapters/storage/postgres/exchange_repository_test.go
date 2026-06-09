package postgres

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"
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
		{Market: "PEPPER/USDC", Side: order.SideBuy, Type: order.TypeLimit, Status: order.StatusOpen, Price: "1", RemainingQuantity: "2", SequenceID: 12},
		{Market: "PEPPER/USDC", Side: order.SideBuy, Type: order.TypeLimit, Status: order.StatusOpen, Price: "1", RemainingQuantity: "3", SequenceID: 7},
	})

	if len(levels) != 1 {
		t.Fatalf("expected one level, got %d", len(levels))
	}
	if levels[0].Quantity != "5" || levels[0].OrderCount != 2 || levels[0].FirstSequenceID != 7 {
		t.Fatalf("unexpected aggregate level: %#v", levels[0])
	}
}

func TestAggregateLevelsSkipsNonBookableOrders(t *testing.T) {
	levels := aggregateLevels("PEPPER/USDC", order.SideBuy, []order.Order{
		{Market: "PEPPER/USDC", Side: order.SideBuy, Type: order.TypeLimit, Status: order.StatusFilled, Price: "1", RemainingQuantity: "0", SequenceID: 1},
		{Market: "PEPPER/USDC", Side: order.SideBuy, Type: order.TypeLimit, Status: order.StatusOpen, Price: "1", RemainingQuantity: "0.000000000000000000", SequenceID: 2},
		{Market: "PEPPER/USDC", Side: order.SideBuy, Type: order.TypeMarket, Status: order.StatusOpen, Price: "1", RemainingQuantity: "2", SequenceID: 3},
		{Market: "PEPPER/USDC", Side: order.SideBuy, Type: order.TypeLimit, Status: order.StatusOpen, Price: "1", RemainingQuantity: "4", SequenceID: 4},
	})

	if len(levels) != 1 {
		t.Fatalf("expected one level, got %d: %#v", len(levels), levels)
	}
	if levels[0].Quantity != "4" || levels[0].OrderCount != 1 || levels[0].FirstSequenceID != 4 {
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

func TestSettlementPlanForBuyTakerDebitsLockedLimitAndReleasesImprovement(t *testing.T) {
	maker := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: "9", RemainingQuantity: "2"}
	taker := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: "10", RemainingQuantity: "2"}
	item := trade.Trade{
		ID:            "trd-1",
		BaseAsset:     "PEPPER",
		QuoteAsset:    "USDC",
		MakerOrderID:  maker.ID,
		TakerOrderID:  taker.ID,
		MakerUserID:   maker.UserID,
		TakerUserID:   taker.UserID,
		TakerSide:     order.SideBuy,
		Price:         "9",
		Quantity:      "2",
		QuoteQuantity: "18",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}

	plan, err := settlementForTrade(item, maker, taker)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Buyer.ID != taker.ID || plan.Seller.ID != maker.ID {
		t.Fatalf("unexpected buyer/seller: %#v", plan)
	}
	if plan.BuyerLockedQuote != "20" {
		t.Fatalf("buyer locked debit = %s, want 20", plan.BuyerLockedQuote)
	}
	if plan.BuyerQuoteRelease != "2" {
		t.Fatalf("buyer quote release = %s, want 2", plan.BuyerQuoteRelease)
	}
}

func TestSettlementPlanForSellTakerUsesMakerBuyerLimit(t *testing.T) {
	maker := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: "10", RemainingQuantity: "2"}
	taker := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: "9", RemainingQuantity: "2"}
	item := trade.Trade{
		ID:            "trd-1",
		BaseAsset:     "PEPPER",
		QuoteAsset:    "USDC",
		MakerOrderID:  maker.ID,
		TakerOrderID:  taker.ID,
		MakerUserID:   maker.UserID,
		TakerUserID:   taker.UserID,
		TakerSide:     order.SideSell,
		Price:         "10",
		Quantity:      "2",
		QuoteQuantity: "20",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}

	plan, err := settlementForTrade(item, maker, taker)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Buyer.ID != maker.ID || plan.Seller.ID != taker.ID {
		t.Fatalf("unexpected buyer/seller: %#v", plan)
	}
	if plan.BuyerLockedQuote != "20" || plan.BuyerQuoteRelease != "0" {
		t.Fatalf("unexpected buyer accounting: %#v", plan)
	}
}

func TestSettlementPlanRejectsTradeAboveBuyerLimitBeforeBalanceMutation(t *testing.T) {
	maker := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: "11", RemainingQuantity: "2"}
	taker := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: "10", RemainingQuantity: "2"}
	item := trade.Trade{
		ID:            "trd-1",
		BaseAsset:     "PEPPER",
		QuoteAsset:    "USDC",
		MakerOrderID:  maker.ID,
		TakerOrderID:  taker.ID,
		MakerUserID:   maker.UserID,
		TakerUserID:   taker.UserID,
		TakerSide:     order.SideBuy,
		Price:         "11",
		Quantity:      "2",
		QuoteQuantity: "22",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}

	if _, err := settlementForTrade(item, maker, taker); !errors.Is(err, ErrInvalidSettlement) {
		t.Fatalf("expected ErrInvalidSettlement, got %v", err)
	}
}

func TestSettlementPlanRejectsNonPositiveTradeAmountsBeforeBalanceMutation(t *testing.T) {
	maker := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: "1", RemainingQuantity: "1"}
	taker := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: "1", RemainingQuantity: "1"}
	base := trade.Trade{
		ID:            "trd-1",
		BaseAsset:     "PEPPER",
		QuoteAsset:    "USDC",
		MakerOrderID:  maker.ID,
		TakerOrderID:  taker.ID,
		MakerUserID:   maker.UserID,
		TakerUserID:   taker.UserID,
		TakerSide:     order.SideBuy,
		Price:         "1",
		Quantity:      "1",
		QuoteQuantity: "1",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}

	zeroQuantity := base
	zeroQuantity.Quantity = "0"
	zeroQuote := base
	zeroQuote.QuoteQuantity = "0"
	for name, item := range map[string]trade.Trade{
		"zero quantity": zeroQuantity,
		"zero quote":    zeroQuote,
	} {
		if _, err := settlementForTrade(item, maker, taker); !errors.Is(err, ErrInvalidSettlement) {
			t.Fatalf("%s: expected ErrInvalidSettlement, got %v", name, err)
		}
	}
}

func TestSettlementPlanRejectsDustQuoteBelowSupportedPrecision(t *testing.T) {
	maker := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: "0.000000000000000001", RemainingQuantity: "0.000000000000000001"}
	taker := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: "0.000000000000000001", RemainingQuantity: "0.000000000000000001"}
	item := trade.Trade{
		ID:            "trd-1",
		BaseAsset:     "PEPPER",
		QuoteAsset:    "USDC",
		MakerOrderID:  maker.ID,
		TakerOrderID:  taker.ID,
		MakerUserID:   maker.UserID,
		TakerUserID:   taker.UserID,
		TakerSide:     order.SideBuy,
		Price:         "0.000000000000000001",
		Quantity:      "0.000000000000000001",
		QuoteQuantity: "0",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}

	if _, err := settlementForTrade(item, maker, taker); !errors.Is(err, ErrInvalidSettlement) {
		t.Fatalf("expected ErrInvalidSettlement, got %v", err)
	}
}

func TestSettlementPlanSupportsSelfTradeWithoutNegativeRelease(t *testing.T) {
	maker := order.Order{ID: "ask-1", UserID: "user-a", Side: order.SideSell, Price: "9.5", RemainingQuantity: "1.25"}
	taker := order.Order{ID: "bid-1", UserID: "user-a", Side: order.SideBuy, Price: "10", RemainingQuantity: "1.25"}
	item := trade.Trade{
		ID:            "trd-1",
		BaseAsset:     "PEPPER",
		QuoteAsset:    "USDC",
		MakerOrderID:  maker.ID,
		TakerOrderID:  taker.ID,
		MakerUserID:   maker.UserID,
		TakerUserID:   taker.UserID,
		TakerSide:     order.SideBuy,
		Price:         "9.5",
		Quantity:      "1.25",
		QuoteQuantity: "11.875",
		CreatedAt:     time.Unix(1, 0).UTC(),
	}

	plan, err := settlementForTrade(item, maker, taker)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Buyer.UserID != "user-a" || plan.Seller.UserID != "user-a" {
		t.Fatalf("expected self trade user on both sides: %#v", plan)
	}
	if plan.BuyerLockedQuote != "12.5" || plan.BuyerQuoteRelease != "0.625" {
		t.Fatalf("unexpected self-trade accounting: %#v", plan)
	}
}

func TestSettlementPlanMaintainsQuoteInvariantsAcrossGeneratedTrades(t *testing.T) {
	rng := rand.New(rand.NewSource(73))

	for i := 0; i < 500; i++ {
		buyerLimitUnits := int64(1_000_000 + rng.Int63n(90_000_000))
		executionPriceUnits := buyerLimitUnits - rng.Int63n(100_000)
		if executionPriceUnits <= 0 {
			executionPriceUnits = 1
		}
		quantityUnits := int64(1 + rng.Int63n(10_000_000))

		buyer := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: decimalTestFixed6(buyerLimitUnits), RemainingQuantity: decimalTestFixed6(quantityUnits)}
		seller := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: decimalTestFixed6(executionPriceUnits), RemainingQuantity: decimalTestFixed6(quantityUnits)}
		takerSide := order.SideBuy
		maker := seller
		taker := buyer
		if i%2 == 1 {
			takerSide = order.SideSell
			maker = buyer
			taker = seller
		}

		item := trade.Trade{
			ID:            "trd-1",
			BaseAsset:     "PEPPER",
			QuoteAsset:    "USDC",
			MakerOrderID:  maker.ID,
			TakerOrderID:  taker.ID,
			MakerUserID:   maker.UserID,
			TakerUserID:   taker.UserID,
			TakerSide:     takerSide,
			Price:         decimalTestFixed6(executionPriceUnits),
			Quantity:      decimalTestFixed6(quantityUnits),
			QuoteQuantity: decimal.Mul(decimalTestFixed6(quantityUnits), decimalTestFixed6(executionPriceUnits)),
			CreatedAt:     time.Unix(1, 0).UTC(),
		}

		plan, err := settlementForTrade(item, maker, taker)
		if err != nil {
			t.Fatalf("case %d failed: %v", i, err)
		}
		if plan.Buyer.ID != buyer.ID || plan.Seller.ID != seller.ID {
			t.Fatalf("case %d resolved wrong buyer/seller: %#v", i, plan)
		}
		if decimal.Cmp(plan.BuyerLockedQuote, item.QuoteQuantity) < 0 {
			t.Fatalf("case %d locked quote below trade quote: plan=%#v trade=%#v", i, plan, item)
		}
		if decimal.Cmp(plan.BuyerQuoteRelease, "0") < 0 {
			t.Fatalf("case %d negative quote release: %#v", i, plan)
		}
		wantRelease := decimal.SubFloorZero(plan.BuyerLockedQuote, item.QuoteQuantity)
		if decimal.Cmp(plan.BuyerQuoteRelease, wantRelease) != 0 {
			t.Fatalf("case %d release = %s, want %s", i, plan.BuyerQuoteRelease, wantRelease)
		}
	}
}

func TestSettlementPlanRejectsGeneratedTradesAboveBuyerLimit(t *testing.T) {
	rng := rand.New(rand.NewSource(91))

	for i := 0; i < 200; i++ {
		buyerLimitUnits := int64(1_000_000 + rng.Int63n(90_000_000))
		executionPriceUnits := buyerLimitUnits + 1 + rng.Int63n(100_000)
		quantityUnits := int64(1 + rng.Int63n(10_000_000))

		maker := order.Order{ID: "ask-1", UserID: "seller", Side: order.SideSell, Price: decimalTestFixed6(executionPriceUnits), RemainingQuantity: decimalTestFixed6(quantityUnits)}
		taker := order.Order{ID: "bid-1", UserID: "buyer", Side: order.SideBuy, Price: decimalTestFixed6(buyerLimitUnits), RemainingQuantity: decimalTestFixed6(quantityUnits)}
		item := trade.Trade{
			ID:            "trd-1",
			BaseAsset:     "PEPPER",
			QuoteAsset:    "USDC",
			MakerOrderID:  maker.ID,
			TakerOrderID:  taker.ID,
			MakerUserID:   maker.UserID,
			TakerUserID:   taker.UserID,
			TakerSide:     order.SideBuy,
			Price:         decimalTestFixed6(executionPriceUnits),
			Quantity:      decimalTestFixed6(quantityUnits),
			QuoteQuantity: decimal.Mul(decimalTestFixed6(quantityUnits), decimalTestFixed6(executionPriceUnits)),
			CreatedAt:     time.Unix(1, 0).UTC(),
		}

		if _, err := settlementForTrade(item, maker, taker); !errors.Is(err, ErrInvalidSettlement) {
			t.Fatalf("case %d expected ErrInvalidSettlement, got %v", i, err)
		}
	}
}

func decimalTestFixed6(units int64) string {
	whole := units / 1_000_000
	frac := units % 1_000_000
	return decimal.Trim(fmt.Sprintf("%d.%06d", whole, frac))
}
