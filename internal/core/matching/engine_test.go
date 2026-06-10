package matching

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"
)

func TestMatchLimitRespectsPriceTimePriority(t *testing.T) {
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
	if !result.Trades[0].CreatedAt.Equal(now) || !result.Trades[1].CreatedAt.Equal(now.Add(time.Microsecond)) {
		t.Fatalf("trade timestamps must preserve match order: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusFilled || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("unexpected taker result: %#v", result.Taker)
	}
	if len(result.Makers) != 2 || result.Makers[0].Status != order.StatusFilled || result.Makers[1].RemainingQuantity != "3" {
		t.Fatalf("unexpected maker updates: %#v", result.Makers)
	}
}

func TestMatchLimitPreventsSelfTradeByExpiringTaker(t *testing.T) {
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
		RemainingQuantity: "2",
	}
	makers := []order.Order{
		{ID: "maker", UserID: "u1", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "9", RemainingQuantity: "2"},
	}

	result, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 0 {
		t.Fatalf("self-trade must not produce trades: %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusExpired || result.Taker.RemainingQuantity != "2" {
		t.Fatalf("self-crossing taker should expire without fill: %#v", result.Taker)
	}
	if len(result.Makers) != 0 {
		t.Fatalf("self-trade must not mutate maker orders: %#v", result.Makers)
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

func TestMatchLimitPartiallyFillsTakerAndLeavesRemainderOpen(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "USDC/USD",
		BaseAsset:         "USDC",
		QuoteAsset:        "USD",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "0.5",
		Quantity:          "100",
		FilledQuantity:    "0",
		RemainingQuantity: "100",
	}
	makers := []order.Order{
		{
			ID:                "maker",
			UserID:            "u2",
			Market:            "USDC/USD",
			BaseAsset:         "USDC",
			QuoteAsset:        "USD",
			Side:              order.SideSell,
			Status:            order.StatusOpen,
			Price:             "0.5",
			Quantity:          "95",
			FilledQuantity:    "0",
			RemainingQuantity: "95",
		},
	}

	result, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("expected one trade, got %d", len(result.Trades))
	}
	tr := result.Trades[0]
	if tr.Price != "0.5" || tr.Quantity != "95" || tr.QuoteQuantity != "47.5" {
		t.Fatalf("unexpected trade: %#v", tr)
	}
	if result.Taker.Status != order.StatusPartiallyFilled || result.Taker.FilledQuantity != "95" || result.Taker.RemainingQuantity != "5" {
		t.Fatalf("taker should keep exact open remainder: %#v", result.Taker)
	}
	if len(result.Makers) != 1 || result.Makers[0].Status != order.StatusFilled || result.Makers[0].FilledQuantity != "95" || result.Makers[0].RemainingQuantity != "0" {
		t.Fatalf("maker should be fully depleted: %#v", result.Makers)
	}
	assertNonNegativeDecimal(t, result.Taker.RemainingQuantity, "taker remaining")
	assertNonNegativeDecimal(t, result.Makers[0].RemainingQuantity, "maker remaining")
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

func TestMatchLimitRejectsZeroQuoteDustTrade(t *testing.T) {
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "0.000000000000000001",
		RemainingQuantity: "0.000000000000000001",
	}
	makers := []order.Order{
		{ID: "m1", UserID: "u2", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "0.000000000000000001", RemainingQuantity: "0.000000000000000001"},
	}

	if _, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, time.Unix(1, 0).UTC()); err == nil {
		t.Fatalf("expected zero quote dust trade to be rejected")
	}
}

func TestMatchLimitConservesQuantityWithoutDecimalEpsilon(t *testing.T) {
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "1",
		RemainingQuantity: "1",
	}
	makers := []order.Order{
		{ID: "m1", UserID: "u2", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "1", RemainingQuantity: "0.333333333333333333"},
		{ID: "m2", UserID: "u3", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "1", RemainingQuantity: "0.333333333333333333"},
		{ID: "m3", UserID: "u4", Market: "PEPPER/USDC", Side: order.SideSell, Status: order.StatusOpen, Price: "1", RemainingQuantity: "0.333333333333333334"},
	}

	var seq int
	result, err := MatchLimit(taker, makers, func() trade.ID {
		seq++
		return trade.ID("trd")
	}, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}

	if result.Taker.FilledQuantity != "1" || result.Taker.RemainingQuantity != "0" || result.Taker.Status != order.StatusFilled {
		t.Fatalf("unexpected taker exact fill: %#v", result.Taker)
	}
	sum := "0"
	for _, tr := range result.Trades {
		assertNonNegativeDecimal(t, tr.Quantity, "trade quantity")
		sum = decimal.Add(sum, tr.Quantity)
	}
	if sum != "1" {
		t.Fatalf("trade quantity sum = %s, want exact 1", sum)
	}
	for _, maker := range result.Makers {
		assertNonNegativeDecimal(t, maker.FilledQuantity, "maker filled")
		assertNonNegativeDecimal(t, maker.RemainingQuantity, "maker remaining")
		if maker.Status != order.StatusFilled || maker.RemainingQuantity != "0" {
			t.Fatalf("maker was not exactly depleted: %#v", maker)
		}
	}
}

func TestMatchLimitNeverOverfillsTakerOrMaker(t *testing.T) {
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideSell,
		Status:            order.StatusOpen,
		Price:             "9",
		RemainingQuantity: "5",
	}
	makers := []order.Order{
		{ID: "m1", UserID: "u2", Market: "PEPPER/USDC", Side: order.SideBuy, Status: order.StatusOpen, Price: "10", RemainingQuantity: "2"},
		{ID: "m2", UserID: "u3", Market: "PEPPER/USDC", Side: order.SideBuy, Status: order.StatusOpen, Price: "9", RemainingQuantity: "10"},
	}

	result, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if result.Taker.FilledQuantity != "5" || result.Taker.RemainingQuantity != "0" {
		t.Fatalf("taker over/under filled: %#v", result.Taker)
	}
	if len(result.Makers) != 2 {
		t.Fatalf("expected two maker updates, got %d", len(result.Makers))
	}
	if result.Makers[0].FilledQuantity != "2" || result.Makers[0].RemainingQuantity != "0" {
		t.Fatalf("first maker over/under filled: %#v", result.Makers[0])
	}
	if result.Makers[1].FilledQuantity != "3" || result.Makers[1].RemainingQuantity != "7" {
		t.Fatalf("second maker over/under filled: %#v", result.Makers[1])
	}
	for _, maker := range result.Makers {
		assertNonNegativeDecimal(t, maker.FilledQuantity, "maker filled")
		assertNonNegativeDecimal(t, maker.RemainingQuantity, "maker remaining")
	}
}

func TestMatchLimitHugeDecimalsDoNotOverflowOrGoNegative(t *testing.T) {
	taker := order.Order{
		ID:                "taker",
		UserID:            "u1",
		Market:            "PEPPER/USDC",
		BaseAsset:         "PEPPER",
		QuoteAsset:        "USDC",
		Side:              order.SideBuy,
		Status:            order.StatusOpen,
		Price:             "999999999999999999.999999999999999999",
		RemainingQuantity: "1000000000000000000000000000000.123456789123456789",
	}
	makers := []order.Order{
		{
			ID:                "m1",
			UserID:            "u2",
			Market:            "PEPPER/USDC",
			Side:              order.SideSell,
			Status:            order.StatusOpen,
			Price:             "999999999999999999.999999999999999999",
			RemainingQuantity: "1000000000000000000000000000000.123456789123456789",
		},
	}

	result, err := MatchLimit(taker, makers, func() trade.ID { return "trd" }, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("expected one huge trade, got %d", len(result.Trades))
	}
	if result.Taker.RemainingQuantity != "0" || result.Makers[0].RemainingQuantity != "0" {
		t.Fatalf("huge decimal trade left unexpected remainder: taker=%#v maker=%#v", result.Taker, result.Makers[0])
	}
	assertNonNegativeDecimal(t, result.Trades[0].QuoteQuantity, "huge quote quantity")
}

func TestMatchLimitMaintainsAccountingInvariantsAcrossGeneratedBooks(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	now := time.Unix(1, 0).UTC()

	for i := 0; i < 200; i++ {
		takerSide := order.SideBuy
		makerSide := order.SideSell
		if i%2 == 1 {
			takerSide = order.SideSell
			makerSide = order.SideBuy
		}
		takerPriceUnits := int64(1_000_000 + rng.Int63n(9_000_000))
		takerQtyUnits := int64(1 + rng.Int63n(2_000_000))
		taker := order.Order{
			ID:                order.ID(fmt.Sprintf("taker-%d", i)),
			UserID:            "u-taker",
			Market:            "PEPPER/USDC",
			BaseAsset:         "PEPPER",
			QuoteAsset:        "USDC",
			Side:              takerSide,
			Status:            order.StatusOpen,
			Price:             fixed6(takerPriceUnits),
			FilledQuantity:    "0",
			RemainingQuantity: fixed6(takerQtyUnits),
		}

		makers := make([]order.Order, 0, 16)
		originalMakers := make(map[order.ID]order.Order)
		for j := 0; j < 16; j++ {
			priceUnits := takerPriceUnits
			if takerSide == order.SideBuy {
				if j%4 == 0 {
					priceUnits += 1 + rng.Int63n(50_000)
				} else {
					priceUnits -= rng.Int63n(50_000)
					if priceUnits <= 0 {
						priceUnits = 1
					}
				}
			} else {
				if j%4 == 0 {
					priceUnits -= rng.Int63n(50_000)
					if priceUnits <= 0 {
						priceUnits = 1
					}
				} else {
					priceUnits += rng.Int63n(50_000)
				}
			}

			item := order.Order{
				ID:                order.ID(fmt.Sprintf("maker-%d-%d", i, j)),
				UserID:            fmt.Sprintf("u-maker-%d", j),
				Market:            "PEPPER/USDC",
				BaseAsset:         "PEPPER",
				QuoteAsset:        "USDC",
				Side:              makerSide,
				Status:            order.StatusOpen,
				Price:             fixed6(priceUnits),
				FilledQuantity:    "0",
				RemainingQuantity: fixed6(1 + rng.Int63n(700_000)),
			}
			if j%7 == 0 {
				item.Side = takerSide
			}
			if j%9 == 0 {
				item.Status = order.StatusFilled
			}
			if j%11 == 0 {
				item.Market = "OTHER/USDC"
			}
			makers = append(makers, item)
			originalMakers[item.ID] = item
		}

		var seq int
		result, err := MatchLimit(taker, makers, func() trade.ID {
			seq++
			return trade.ID(fmt.Sprintf("trd-%d-%d", i, seq))
		}, now)
		if err != nil {
			t.Fatalf("case %d failed: %v", i, err)
		}

		takerTradeQty := "0"
		makerTradeQty := make(map[order.ID]string)
		for _, item := range result.Trades {
			originalMaker, ok := originalMakers[item.MakerOrderID]
			if !ok {
				t.Fatalf("case %d produced trade for unknown maker: %#v", i, item)
			}
			if !eligibleMaker(taker, originalMaker) {
				t.Fatalf("case %d produced trade for ineligible maker: maker=%#v trade=%#v", i, originalMaker, item)
			}
			assertNonNegativeDecimal(t, item.Quantity, "trade quantity")
			assertNonNegativeDecimal(t, item.QuoteQuantity, "trade quote quantity")
			if decimal.Cmp(item.Quantity, "0") <= 0 || decimal.Cmp(item.QuoteQuantity, "0") <= 0 {
				t.Fatalf("case %d produced non-positive trade: %#v", i, item)
			}
			if item.QuoteQuantity != decimal.Mul(item.Quantity, item.Price) {
				t.Fatalf("case %d quote quantity mismatch: %#v", i, item)
			}
			if takerSide == order.SideBuy && decimal.Cmp(item.Price, taker.Price) > 0 {
				t.Fatalf("case %d buy crossed above limit: taker=%#v trade=%#v", i, taker, item)
			}
			if takerSide == order.SideSell && decimal.Cmp(item.Price, taker.Price) < 0 {
				t.Fatalf("case %d sell crossed below limit: taker=%#v trade=%#v", i, taker, item)
			}
			takerTradeQty = decimal.Add(takerTradeQty, item.Quantity)
			makerTradeQty[item.MakerOrderID] = decimal.Add(makerTradeQty[item.MakerOrderID], item.Quantity)
		}
		if len(result.Makers) != len(makerTradeQty) {
			t.Fatalf("case %d maker updates = %d, want %d", i, len(result.Makers), len(makerTradeQty))
		}

		assertDecimalEqual(t, result.Taker.FilledQuantity, takerTradeQty, fmt.Sprintf("case %d taker filled", i))
		assertDecimalEqual(t, result.Taker.RemainingQuantity, decimal.SubFloorZero(taker.RemainingQuantity, takerTradeQty), fmt.Sprintf("case %d taker remaining", i))
		assertNonNegativeDecimal(t, result.Taker.FilledQuantity, "taker filled")
		assertNonNegativeDecimal(t, result.Taker.RemainingQuantity, "taker remaining")
		if decimal.Cmp(takerTradeQty, taker.RemainingQuantity) > 0 {
			t.Fatalf("case %d overfilled taker: filled=%s original=%s", i, takerTradeQty, taker.RemainingQuantity)
		}
		assertStatusMatchesRemaining(t, result.Taker.Status, result.Taker.RemainingQuantity, fmt.Sprintf("case %d taker status", i))

		for _, updated := range result.Makers {
			original, ok := originalMakers[updated.ID]
			if !ok {
				t.Fatalf("case %d returned unknown maker update: %#v", i, updated)
			}
			if !eligibleMaker(taker, original) {
				t.Fatalf("case %d updated ineligible maker: original=%#v updated=%#v", i, original, updated)
			}
			filled := makerTradeQty[updated.ID]
			if decimal.Cmp(filled, original.RemainingQuantity) > 0 {
				t.Fatalf("case %d overfilled maker %s: filled=%s original=%s", i, updated.ID, filled, original.RemainingQuantity)
			}
			assertDecimalEqual(t, updated.FilledQuantity, filled, fmt.Sprintf("case %d maker filled", i))
			assertDecimalEqual(t, updated.RemainingQuantity, decimal.SubFloorZero(original.RemainingQuantity, filled), fmt.Sprintf("case %d maker remaining", i))
			assertNonNegativeDecimal(t, updated.FilledQuantity, "maker filled")
			assertNonNegativeDecimal(t, updated.RemainingQuantity, "maker remaining")
			assertStatusMatchesRemaining(t, updated.Status, updated.RemainingQuantity, fmt.Sprintf("case %d maker status", i))
		}
	}
}

func FuzzMatchLimitAccountingInvariants(f *testing.F) {
	f.Add(uint64(1_000_000), uint64(250_000), uint64(999_000), uint64(100_000), true)
	f.Add(uint64(5_000_000), uint64(1_000_000), uint64(4_999_000), uint64(900_000), false)

	f.Fuzz(func(t *testing.T, takerPriceUnits uint64, takerQtyUnits uint64, makerPriceUnits uint64, makerQtyUnits uint64, buySide bool) {
		// Arrange.
		now := time.Unix(1, 0).UTC()
		takerSide := order.SideBuy
		makerSide := order.SideSell
		if !buySide {
			takerSide = order.SideSell
			makerSide = order.SideBuy
		}

		takerPriceUnits = takerPriceUnits%9_000_000 + 1_000_000
		takerQtyUnits = takerQtyUnits%2_000_000 + 1
		makerPriceUnits = makerPriceUnits%9_000_000 + 1_000_000
		makerQtyUnits = makerQtyUnits%2_000_000 + 1

		taker := order.Order{
			ID:                "taker",
			UserID:            "u-taker",
			Market:            "PEPPER/USDC",
			BaseAsset:         "PEPPER",
			QuoteAsset:        "USDC",
			Side:              takerSide,
			Status:            order.StatusOpen,
			Price:             fixed6(int64(takerPriceUnits)),
			FilledQuantity:    "0",
			RemainingQuantity: fixed6(int64(takerQtyUnits)),
		}

		makers := make([]order.Order, 0, 8)
		originalMakers := make(map[order.ID]order.Order, 8)
		for i := 0; i < 8; i++ {
			priceUnits := makerPriceUnits + uint64(i*137)
			if takerSide == order.SideBuy {
				if i%2 == 0 {
					priceUnits = takerPriceUnits - uint64(1+i*11)
					if priceUnits == 0 {
						priceUnits = 1
					}
				} else {
					priceUnits = takerPriceUnits + uint64(1+i*11)
				}
			} else {
				if i%2 == 0 {
					priceUnits = takerPriceUnits + uint64(1+i*11)
				} else {
					priceUnits = takerPriceUnits - uint64(1+i*11)
					if priceUnits == 0 {
						priceUnits = 1
					}
				}
			}

			item := order.Order{
				ID:                order.ID(fmt.Sprintf("maker-%d", i)),
				UserID:            fmt.Sprintf("u-maker-%d", i),
				Market:            "PEPPER/USDC",
				BaseAsset:         "PEPPER",
				QuoteAsset:        "USDC",
				Side:              makerSide,
				Status:            order.StatusOpen,
				Price:             fixed6(int64(priceUnits)),
				FilledQuantity:    "0",
				RemainingQuantity: fixed6(int64((makerQtyUnits+uint64(i*71))%2_000_000 + 1)),
			}
			if i%5 == 0 {
				item.Side = takerSide
			}
			if i%7 == 0 {
				item.Status = order.StatusFilled
			}
			if i%11 == 0 {
				item.Market = "OTHER/USDC"
			}
			makers = append(makers, item)
			originalMakers[item.ID] = item
		}

		// Act.
		var seq int
		result, err := MatchLimit(taker, makers, func() trade.ID {
			seq++
			return trade.ID(fmt.Sprintf("trd-%d", seq))
		}, now)
		if err != nil {
			t.Fatalf("match failed: %v", err)
		}

		// Assert.
		takerTradeQty := "0"
		makerTradeQty := make(map[order.ID]string)
		for _, item := range result.Trades {
			originalMaker, ok := originalMakers[item.MakerOrderID]
			if !ok {
				t.Fatalf("trade produced for unknown maker: %#v", item)
			}
			if !eligibleMaker(taker, originalMaker) {
				t.Fatalf("trade produced for ineligible maker: maker=%#v trade=%#v", originalMaker, item)
			}
			assertNonNegativeDecimal(t, item.Quantity, "trade quantity")
			assertNonNegativeDecimal(t, item.QuoteQuantity, "trade quote quantity")
			if decimal.Cmp(item.Quantity, "0") <= 0 || decimal.Cmp(item.QuoteQuantity, "0") <= 0 {
				t.Fatalf("trade quantity and quote quantity must be positive: %#v", item)
			}
			if item.QuoteQuantity != decimal.Mul(item.Quantity, item.Price) {
				t.Fatalf("quote quantity mismatch: %#v", item)
			}
			if takerSide == order.SideBuy && decimal.Cmp(item.Price, taker.Price) > 0 {
				t.Fatalf("buy crossed above limit: taker=%#v trade=%#v", taker, item)
			}
			if takerSide == order.SideSell && decimal.Cmp(item.Price, taker.Price) < 0 {
				t.Fatalf("sell crossed below limit: taker=%#v trade=%#v", taker, item)
			}
			takerTradeQty = decimal.Add(takerTradeQty, item.Quantity)
			makerTradeQty[item.MakerOrderID] = decimal.Add(makerTradeQty[item.MakerOrderID], item.Quantity)
		}

		if len(result.Makers) != len(makerTradeQty) {
			t.Fatalf("maker updates = %d, want %d", len(result.Makers), len(makerTradeQty))
		}
		assertDecimalEqual(t, result.Taker.FilledQuantity, takerTradeQty, "taker filled")
		assertDecimalEqual(t, result.Taker.RemainingQuantity, decimal.SubFloorZero(taker.RemainingQuantity, takerTradeQty), "taker remaining")
		assertNonNegativeDecimal(t, result.Taker.FilledQuantity, "taker filled")
		assertNonNegativeDecimal(t, result.Taker.RemainingQuantity, "taker remaining")
		assertStatusMatchesRemaining(t, result.Taker.Status, result.Taker.RemainingQuantity, "taker status")
		if decimal.Cmp(takerTradeQty, taker.RemainingQuantity) > 0 {
			t.Fatalf("overfilled taker: filled=%s original=%s", takerTradeQty, taker.RemainingQuantity)
		}

		for _, updated := range result.Makers {
			original, ok := originalMakers[updated.ID]
			if !ok {
				t.Fatalf("unknown maker update: %#v", updated)
			}
			if !eligibleMaker(taker, original) {
				t.Fatalf("updated ineligible maker: original=%#v updated=%#v", original, updated)
			}
			filled := makerTradeQty[updated.ID]
			if decimal.Cmp(filled, original.RemainingQuantity) > 0 {
				t.Fatalf("overfilled maker %s: filled=%s original=%s", updated.ID, filled, original.RemainingQuantity)
			}
			assertDecimalEqual(t, updated.FilledQuantity, filled, "maker filled")
			assertDecimalEqual(t, updated.RemainingQuantity, decimal.SubFloorZero(original.RemainingQuantity, filled), "maker remaining")
			assertNonNegativeDecimal(t, updated.FilledQuantity, "maker filled")
			assertNonNegativeDecimal(t, updated.RemainingQuantity, "maker remaining")
			assertStatusMatchesRemaining(t, updated.Status, updated.RemainingQuantity, "maker status")
		}
	})
}

func assertNonNegativeDecimal(t *testing.T, value string, label string) {
	t.Helper()
	if decimal.Cmp(value, "0") < 0 {
		t.Fatalf("%s is negative: %s", label, value)
	}
}

func assertDecimalEqual(t *testing.T, got string, want string, label string) {
	t.Helper()
	if decimal.Cmp(got, want) != 0 {
		t.Fatalf("%s = %s, want %s", label, got, want)
	}
}

func assertStatusMatchesRemaining(t *testing.T, status order.Status, remaining string, label string) {
	t.Helper()
	if decimal.Cmp(remaining, "0") <= 0 {
		if status != order.StatusFilled {
			t.Fatalf("%s = %s, want filled for remaining %s", label, status, remaining)
		}
		return
	}
	if status != order.StatusOpen && status != order.StatusPartiallyFilled {
		t.Fatalf("%s = %s, want open or partially_filled for remaining %s", label, status, remaining)
	}
}

func fixed6(units int64) string {
	whole := units / 1_000_000
	frac := units % 1_000_000
	return decimal.Trim(fmt.Sprintf("%d.%06d", whole, frac))
}
