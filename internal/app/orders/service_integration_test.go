package orders

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	storage "exchange/internal/adapters/storage/postgres"
	"exchange/internal/core/balance"
	"exchange/internal/core/decimal"
	"exchange/internal/core/market"
	"exchange/internal/core/order"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
)

func TestMarketOrderBookCleanupIntegration(t *testing.T) {
	repo, db := integrationRepository(t)

	t.Run("market buy fills single ask and deletes level", func(t *testing.T) {
		base, marketSymbol := integrationMarket("TMB1")
		seller := integrationUser(t, "seller")
		buyer := integrationUser(t, "buyer")
		svc := integrationOrderService(repo, marketSymbol, base)
		cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
		defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

		fundUser(t, repo, seller, base, "5")
		fundUser(t, repo, buyer, "USD", "500")
		ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "100", "5")
		assertBookSide(t, svc, marketSymbol, order.SideSell, []priceLevelWant{{Price: "100", Quantity: "5", OrderCount: 1}})

		buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "100", "5")

		assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, buy.Order.ID, order.StatusFilled, "0")
		assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
		assertOpenOrders(t, repo, marketSymbol, order.SideSell, nil)
		assertTradeCount(t, repo, marketSymbol, 1)
		assertOrderEventExists(t, db, ask.Order.ID, order.EventOrderFilled)
		assertNoNegativeBalances(t, db, buyer, seller)
		assertLockedBalance(t, db, buyer, "USD", "0")
		assertLockedBalance(t, db, seller, base, "0")
		assertReservationState(t, repo, ask.Order.ReservationID, storage.ReservationStatusConsumed, "0")
		assertReservationState(t, repo, buy.Order.ReservationID, storage.ReservationStatusConsumed, "0")
	})

	t.Run("market buy consumes best ask and leaves next ask remainder", func(t *testing.T) {
		base, marketSymbol := integrationMarket("TMB2")
		sellerA := integrationUser(t, "seller-a")
		sellerB := integrationUser(t, "seller-b")
		buyer := integrationUser(t, "buyer")
		svc := integrationOrderService(repo, marketSymbol, base)
		cleanupIntegrationMarket(t, db, marketSymbol, buyer, sellerA, sellerB)
		defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, sellerA, sellerB)

		fundUser(t, repo, sellerA, base, "5")
		fundUser(t, repo, sellerB, base, "5")
		fundUser(t, repo, buyer, "USD", "707")
		firstAsk := placeIntegrationOrder(t, svc, sellerA, marketSymbol, order.SideSell, order.TypeLimit, "100", "5")
		secondAsk := placeIntegrationOrder(t, svc, sellerB, marketSymbol, order.SideSell, order.TypeLimit, "101", "5")

		buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "101", "7")

		assertOrderState(t, repo, firstAsk.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, secondAsk.Order.ID, order.StatusPartiallyFilled, "3")
		assertOrderState(t, repo, buy.Order.ID, order.StatusFilled, "0")
		assertBookSide(t, svc, marketSymbol, order.SideSell, []priceLevelWant{{Price: "101", Quantity: "3", OrderCount: 1}})
		assertOpenOrders(t, repo, marketSymbol, order.SideSell, []order.ID{secondAsk.Order.ID})
		assertTradeCount(t, repo, marketSymbol, 2)
		assertNoNegativeBalances(t, db, buyer, sellerA, sellerB)
		assertLockedBalance(t, db, buyer, "USD", "0")
		assertLockedBalance(t, db, sellerB, base, "3")
	})

	t.Run("market sell fills single bid and deletes level", func(t *testing.T) {
		base, marketSymbol := integrationMarket("TMS1")
		buyer := integrationUser(t, "buyer")
		seller := integrationUser(t, "seller")
		svc := integrationOrderService(repo, marketSymbol, base)
		cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
		defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

		fundUser(t, repo, buyer, "USD", "495")
		fundUser(t, repo, seller, base, "5")
		bid := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "99", "5")

		sell := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeMarket, "99", "5")

		assertOrderState(t, repo, bid.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, sell.Order.ID, order.StatusFilled, "0")
		assertBookSide(t, svc, marketSymbol, order.SideBuy, nil)
		assertOpenOrders(t, repo, marketSymbol, order.SideBuy, nil)
		assertTradeCount(t, repo, marketSymbol, 1)
		assertOrderEventExists(t, db, bid.Order.ID, order.EventOrderFilled)
		assertNoNegativeBalances(t, db, buyer, seller)
		assertLockedBalance(t, db, buyer, "USD", "0")
		assertLockedBalance(t, db, seller, base, "0")
		assertReservationState(t, repo, bid.Order.ReservationID, storage.ReservationStatusConsumed, "0")
		assertReservationState(t, repo, sell.Order.ReservationID, storage.ReservationStatusConsumed, "0")
	})

	t.Run("market sell consumes best bid and leaves next bid remainder", func(t *testing.T) {
		base, marketSymbol := integrationMarket("TMS2")
		buyerA := integrationUser(t, "buyer-a")
		buyerB := integrationUser(t, "buyer-b")
		seller := integrationUser(t, "seller")
		svc := integrationOrderService(repo, marketSymbol, base)
		cleanupIntegrationMarket(t, db, marketSymbol, buyerA, buyerB, seller)
		defer cleanupIntegrationMarket(t, db, marketSymbol, buyerA, buyerB, seller)

		fundUser(t, repo, buyerA, "USD", "495")
		fundUser(t, repo, buyerB, "USD", "490")
		fundUser(t, repo, seller, base, "7")
		firstBid := placeIntegrationOrder(t, svc, buyerA, marketSymbol, order.SideBuy, order.TypeLimit, "99", "5")
		secondBid := placeIntegrationOrder(t, svc, buyerB, marketSymbol, order.SideBuy, order.TypeLimit, "98", "5")

		sell := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeMarket, "99", "7")

		assertOrderState(t, repo, firstBid.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, secondBid.Order.ID, order.StatusPartiallyFilled, "3")
		assertOrderState(t, repo, sell.Order.ID, order.StatusFilled, "0")
		assertBookSide(t, svc, marketSymbol, order.SideBuy, []priceLevelWant{{Price: "98", Quantity: "3", OrderCount: 1}})
		assertOpenOrders(t, repo, marketSymbol, order.SideBuy, []order.ID{secondBid.Order.ID})
		assertTradeCount(t, repo, marketSymbol, 2)
		assertNoNegativeBalances(t, db, buyerA, buyerB, seller)
		assertLockedBalance(t, db, buyerB, "USD", "294")
		assertLockedBalance(t, db, seller, base, "0")
	})

	t.Run("market buy expires unfilled remainder without resting", func(t *testing.T) {
		base, marketSymbol := integrationMarket("TMB5")
		seller := integrationUser(t, "seller")
		buyer := integrationUser(t, "buyer")
		svc := integrationOrderService(repo, marketSymbol, base)
		cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
		defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

		fundUser(t, repo, seller, base, "3")
		fundUser(t, repo, buyer, "USD", "1000")
		ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "100", "3")

		buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "100", "10")

		assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, buy.Order.ID, order.StatusExpired, "7")
		assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
		assertOpenOrders(t, repo, marketSymbol, order.SideBuy, nil)
		assertOpenOrders(t, repo, marketSymbol, order.SideSell, nil)
		assertTradeCount(t, repo, marketSymbol, 1)
		assertNoNegativeBalances(t, db, buyer, seller)
		assertLockedBalance(t, db, buyer, "USD", "0")
		assertLockedBalance(t, db, seller, base, "0")
		assertReservationState(t, repo, buy.Order.ReservationID, storage.ReservationStatusClosed, "0")
	})

	t.Run("decimal dust stays exact and filled makers do not remain open", func(t *testing.T) {
		base, marketSymbol := integrationMarket("TMD6")
		seller := integrationUser(t, "seller")
		buyer := integrationUser(t, "buyer")
		svc := integrationOrderService(repo, marketSymbol, base)
		cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
		defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

		fundUser(t, repo, seller, base, "0.3")
		fundUser(t, repo, buyer, "USD", "0.3")
		ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "1", "0.3")

		buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "1", "0.1")
		buy2 := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "1", "0.2")

		assertOrderState(t, repo, buy.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, buy2.Order.ID, order.StatusFilled, "0")
		assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
		assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
		assertOpenOrders(t, repo, marketSymbol, order.SideSell, nil)
		assertTradeCount(t, repo, marketSymbol, 2)
		assertNoNegativeBalances(t, db, buyer, seller)
		assertLockedBalance(t, db, buyer, "USD", "0")
		assertLockedBalance(t, db, seller, base, "0")
	})
}

func TestLimitBuySweepsAsksAndRestsRemainderIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TLSW")
	sellerA := integrationUser(t, "seller-a")
	sellerB := integrationUser(t, "seller-b")
	sellerC := integrationUser(t, "seller-c")
	buyer := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, sellerA, sellerB, sellerC)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, sellerA, sellerB, sellerC)

	limitPrice := "1.01008001"
	prices := []string{"1.010071", "1.010075", limitPrice}
	quantities := []string{
		decimalQuo("5", prices[0]),
		decimalQuo("5", prices[1]),
		decimalQuo("5", prices[2]),
	}

	fundUser(t, repo, sellerA, base, quantities[0])
	fundUser(t, repo, sellerB, base, quantities[1])
	fundUser(t, repo, sellerC, base, quantities[2])
	fundUser(t, repo, buyer, "USD", "20")

	askA := placeIntegrationOrder(t, svc, sellerA, marketSymbol, order.SideSell, order.TypeLimit, prices[0], quantities[0])
	askB := placeIntegrationOrder(t, svc, sellerB, marketSymbol, order.SideSell, order.TypeLimit, prices[1], quantities[1])
	askC := placeIntegrationOrder(t, svc, sellerC, marketSymbol, order.SideSell, order.TypeLimit, prices[2], quantities[2])

	buyQty := decimalQuo("16", limitPrice)
	buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, limitPrice, buyQty)
	expectedFilled := decimal.Add(decimal.Add(quantities[0], quantities[1]), quantities[2])
	expectedRemaining := decimal.SubFloorZero(buyQty, expectedFilled)

	assertOrderState(t, repo, askA.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, askB.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, askC.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, buy.Order.ID, order.StatusPartiallyFilled, expectedRemaining)
	assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
	assertBookSide(t, svc, marketSymbol, order.SideBuy, []priceLevelWant{{Price: limitPrice, Quantity: expectedRemaining, OrderCount: 1}})
	assertOpenOrders(t, repo, marketSymbol, order.SideSell, nil)
	assertOpenOrders(t, repo, marketSymbol, order.SideBuy, []order.ID{buy.Order.ID})
	assertTradeCount(t, repo, marketSymbol, 3)
	assertNoNegativeBalances(t, db, buyer, sellerA, sellerB, sellerC)
	assertLockedBalance(t, db, buyer, "USD", decimal.Mul(expectedRemaining, limitPrice))
	assertLockedBalance(t, db, sellerA, base, "0")
	assertLockedBalance(t, db, sellerB, base, "0")
	assertLockedBalance(t, db, sellerC, base, "0")
	assertReservationState(t, repo, buy.Order.ReservationID, storage.ReservationStatusActive, decimal.Mul(expectedRemaining, limitPrice))
}

func TestLimitBuyAmountSweepsAsksAndRestsRemainderIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TAMT")
	sellerA := integrationUser(t, "seller-a")
	sellerB := integrationUser(t, "seller-b")
	sellerC := integrationUser(t, "seller-c")
	buyer := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, sellerA, sellerB, sellerC)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, sellerA, sellerB, sellerC)

	fundUser(t, repo, sellerA, base, "1")
	fundUser(t, repo, sellerB, base, "1")
	fundUser(t, repo, sellerC, base, "1")
	fundUser(t, repo, buyer, "USD", "12")

	askA := placeIntegrationOrder(t, svc, sellerA, marketSymbol, order.SideSell, order.TypeLimit, "1", "1")
	askB := placeIntegrationOrder(t, svc, sellerB, marketSymbol, order.SideSell, order.TypeLimit, "2", "1")
	askC := placeIntegrationOrder(t, svc, sellerC, marketSymbol, order.SideSell, order.TypeLimit, "3", "1")

	buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "3", "4")

	assertOrderState(t, repo, askA.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, askB.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, askC.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, buy.Order.ID, order.StatusPartiallyFilled, "1")
	assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
	assertBookSide(t, svc, marketSymbol, order.SideBuy, []priceLevelWant{{Price: "3", Quantity: "1", OrderCount: 1}})
	assertOpenOrders(t, repo, marketSymbol, order.SideSell, nil)
	assertOpenOrders(t, repo, marketSymbol, order.SideBuy, []order.ID{buy.Order.ID})
	assertTradeCount(t, repo, marketSymbol, 3)
	assertNoNegativeBalances(t, db, buyer, sellerA, sellerB, sellerC)
	assertLockedBalance(t, db, buyer, "USD", "3")
	assertLockedBalance(t, db, sellerA, base, "0")
	assertLockedBalance(t, db, sellerB, base, "0")
	assertLockedBalance(t, db, sellerC, base, "0")
	assertReservationState(t, repo, buy.Order.ReservationID, storage.ReservationStatusActive, "3")
}

func TestRealTradingOrderBookLifecycleIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TRTL")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

	fundUser(t, repo, buyer, "USD", "1000")
	fundUser(t, repo, seller, base, "1000")

	askA := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "1.01", "100")
	askB := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "1.02", "150")
	askC := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "1.03", "200")
	bidA := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "0.99", "100")
	bidB := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "0.98", "200")

	assertBookSide(t, svc, marketSymbol, order.SideSell, []priceLevelWant{
		{Price: "1.01", Quantity: "100", OrderCount: 1},
		{Price: "1.02", Quantity: "150", OrderCount: 1},
		{Price: "1.03", Quantity: "200", OrderCount: 1},
	})
	assertBookSide(t, svc, marketSymbol, order.SideBuy, []priceLevelWant{
		{Price: "0.99", Quantity: "100", OrderCount: 1},
		{Price: "0.98", Quantity: "200", OrderCount: 1},
	})
	assertBalance(t, db, buyer, "USD", "705", "295", "0")
	assertBalance(t, db, seller, base, "550", "450", "0")

	takerBuy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "1.02", "250")
	assertOrderState(t, repo, askA.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, askB.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, askC.Order.ID, order.StatusOpen, "200")
	assertOrderState(t, repo, takerBuy.Order.ID, order.StatusFilled, "0")
	assertBookSide(t, svc, marketSymbol, order.SideSell, []priceLevelWant{{Price: "1.03", Quantity: "200", OrderCount: 1}})
	assertBookSide(t, svc, marketSymbol, order.SideBuy, []priceLevelWant{
		{Price: "0.99", Quantity: "100", OrderCount: 1},
		{Price: "0.98", Quantity: "200", OrderCount: 1},
	})
	assertOpenOrders(t, repo, marketSymbol, order.SideSell, []order.ID{askC.Order.ID})
	assertOpenOrders(t, repo, marketSymbol, order.SideBuy, []order.ID{bidA.Order.ID, bidB.Order.ID})
	assertTradeCount(t, repo, marketSymbol, 2)
	assertBalance(t, db, buyer, "USD", "451", "295", "0")
	assertBalance(t, db, buyer, base, "250", "0", "0")
	assertBalance(t, db, seller, "USD", "254", "0", "0")
	assertBalance(t, db, seller, base, "550", "200", "0")
	assertNoNegativeBalances(t, db, buyer, seller)

	takerSell := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "0.98", "250")
	assertOrderState(t, repo, bidA.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, bidB.Order.ID, order.StatusPartiallyFilled, "50")
	assertOrderState(t, repo, takerSell.Order.ID, order.StatusFilled, "0")
	assertBookSide(t, svc, marketSymbol, order.SideSell, []priceLevelWant{{Price: "1.03", Quantity: "200", OrderCount: 1}})
	assertBookSide(t, svc, marketSymbol, order.SideBuy, []priceLevelWant{{Price: "0.98", Quantity: "50", OrderCount: 1}})
	assertTradeCount(t, repo, marketSymbol, 4)
	assertBalance(t, db, buyer, "USD", "451", "49", "0")
	assertBalance(t, db, buyer, base, "500", "0", "0")
	assertBalance(t, db, seller, "USD", "500", "0", "0")
	assertBalance(t, db, seller, base, "300", "200", "0")
	assertNoNegativeBalances(t, db, buyer, seller)

	canceledBid, err := svc.Cancel(context.Background(), bidB.Order.ID, CancelRequest{UserID: buyer})
	if err != nil {
		t.Fatalf("cancel remaining bid failed: %v", err)
	}
	if canceledBid.Status != order.StatusCanceled {
		t.Fatalf("remaining bid cancel status = %s, want canceled", canceledBid.Status)
	}
	canceledAsk, err := svc.Cancel(context.Background(), askC.Order.ID, CancelRequest{UserID: seller})
	if err != nil {
		t.Fatalf("cancel remaining ask failed: %v", err)
	}
	if canceledAsk.Status != order.StatusCanceled {
		t.Fatalf("remaining ask cancel status = %s, want canceled", canceledAsk.Status)
	}

	assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
	assertBookSide(t, svc, marketSymbol, order.SideBuy, nil)
	assertOpenOrders(t, repo, marketSymbol, order.SideSell, nil)
	assertOpenOrders(t, repo, marketSymbol, order.SideBuy, nil)
	assertBalance(t, db, buyer, "USD", "500", "0", "0")
	assertBalance(t, db, buyer, base, "500", "0", "0")
	assertBalance(t, db, seller, "USD", "500", "0", "0")
	assertBalance(t, db, seller, base, "500", "0", "0")
	assertNoNegativeBalances(t, db, buyer, seller)
}

func TestMarketSummariesReadTickerFromExchangeMarketTableIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TMKT")
	svc := integrationOrderService(repo, marketSymbol, base)
	if err := storage.SyncExchangeMarkets(db, []market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}); err != nil {
		t.Fatalf("sync exchange market failed: %v", err)
	}
	defer func() {
		if err := db.Where("symbol = ?", marketSymbol).Delete(&storage.ExchangeMarket{}).Error; err != nil {
			t.Fatalf("cleanup exchange market failed: %v", err)
		}
	}()
	if err := repo.UpdateExchangeMarketStats(context.Background(), marketSymbol, storage.ExchangeMarketStats{
		LastPrice: "1.23456789",
		Change24h: "12.5",
		High24h:   "1.3",
		Low24h:    "1.1",
		Volume24h: "987.654321",
	}); err != nil {
		t.Fatalf("update market stats failed: %v", err)
	}

	summaries, err := svc.MarketSummaries(context.Background())
	if err != nil {
		t.Fatalf("market summaries failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %#v, want one market", summaries)
	}
	got := summaries[0]
	if decimal.Cmp(got.LastPrice, "1.23456789") != 0 ||
		decimal.Cmp(got.Change24h, "12.5") != 0 ||
		decimal.Cmp(got.High24h, "1.3") != 0 ||
		decimal.Cmp(got.Low24h, "1.1") != 0 ||
		decimal.Cmp(got.Volume24h, "987.654321") != 0 {
		t.Fatalf("summary did not use exchange market ticker fields: %#v", got)
	}
}

func TestRebuildPriceLevelsAggregatesAllBookableOrders(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TRLB")
	userID := integrationUser(t, "maker")
	cleanupIntegrationMarket(t, db, marketSymbol, userID)
	defer cleanupIntegrationMarket(t, db, marketSymbol, userID)

	now := time.Now().UTC()
	models := make([]storage.ExchangeOrder, 0, 1208)
	for i := 0; i < 1005; i++ {
		seq := uint64(i + 1)
		models = append(models, integrationBookOrder(fmt.Sprintf("ord_%s_a_%04d", base, i), fmt.Sprintf("cli_%s_a_%04d", base, i), userID, marketSymbol, base, order.SideBuy, "1", seq, now))
	}
	for i := 0; i < 200; i++ {
		seq := uint64(1006 + i)
		models = append(models, integrationBookOrder(fmt.Sprintf("ord_%s_b_%04d", base, i), fmt.Sprintf("cli_%s_b_%04d", base, i), userID, marketSymbol, base, order.SideBuy, "2", seq, now))
	}
	filteredMarket := integrationBookOrder("ord_"+base+"_market", "cli_"+base+"_market", userID, marketSymbol, base, order.SideBuy, "2", 2000, now)
	filteredMarket.Type = string(order.TypeMarket)
	filteredFilled := integrationBookOrder("ord_"+base+"_filled", "cli_"+base+"_filled", userID, marketSymbol, base, order.SideBuy, "2", 2001, now)
	filteredFilled.Status = string(order.StatusFilled)
	filteredZero := integrationBookOrder("ord_"+base+"_zero", "cli_"+base+"_zero", userID, marketSymbol, base, order.SideBuy, "2", 2002, now)
	filteredZero.RemainingQuantity = "0"
	filteredDust := integrationBookOrder("ord_"+base+"_dust", "cli_"+base+"_dust", userID, marketSymbol, base, order.SideBuy, "2", 2003, now)
	filteredDust.RemainingQuantity = "0.000000000000235016"
	models = append(models, filteredMarket, filteredFilled, filteredZero, filteredDust)

	if err := db.CreateInBatches(&models, 500).Error; err != nil {
		t.Fatalf("seed orders failed: %v", err)
	}
	if err := repo.RebuildActiveOrders(context.Background(), marketSymbol); err != nil {
		t.Fatalf("rebuild active orders failed: %v", err)
	}
	if err := repo.RebuildPriceLevels(context.Background(), marketSymbol); err != nil {
		t.Fatalf("rebuild price levels failed: %v", err)
	}

	levels, err := repo.ListPriceLevels(context.Background(), marketSymbol, order.SideBuy, 10)
	if err != nil {
		t.Fatalf("list price levels failed: %v", err)
	}
	want := []priceLevelWant{
		{Price: "2", Quantity: "200", OrderCount: 200},
		{Price: "1", Quantity: "1005", OrderCount: 1005},
	}
	if len(levels) != len(want) {
		t.Fatalf("levels = %#v, want %#v", levels, want)
	}
	for i := range want {
		if decimal.Cmp(levels[i].Price, want[i].Price) != 0 || decimal.Cmp(levels[i].Quantity, want[i].Quantity) != 0 || levels[i].OrderCount != want[i].OrderCount {
			t.Fatalf("level %d = %#v, want %#v", i, levels[i], want[i])
		}
	}
	if levels[0].FirstSequenceID != 1006 || levels[1].FirstSequenceID != 1 {
		t.Fatalf("first sequence ids = %d/%d, want 1006/1", levels[0].FirstSequenceID, levels[1].FirstSequenceID)
	}
}

func TestPlaceOrderCommandIdempotencyIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TIDC")
	userID := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, userID)
	defer cleanupIntegrationMarket(t, db, marketSymbol, userID)

	fundUser(t, repo, userID, "USD", "100")
	req := PlaceRequest{
		CommandID:     "cmd_" + base + "_1",
		ClientOrderID: "client_" + base + "_1",
		UserID:        userID,
		Market:        marketSymbol,
		Side:          string(order.SideBuy),
		Type:          string(order.TypeLimit),
		Price:         "10",
		Quantity:      "2",
	}

	first, err := svc.Place(context.Background(), req)
	if err != nil {
		t.Fatalf("first place failed: %v", err)
	}
	second, err := svc.Place(context.Background(), req)
	if err != nil {
		t.Fatalf("duplicate place failed: %v", err)
	}
	if first.Order.ID != second.Order.ID {
		t.Fatalf("duplicate returned order %s, want %s", second.Order.ID, first.Order.ID)
	}
	thirdReq := req
	thirdReq.CommandID = "cmd_" + base + "_2"
	third, err := svc.Place(context.Background(), thirdReq)
	if err != nil {
		t.Fatalf("duplicate client id with different command id failed: %v", err)
	}
	if third.Order.ID != first.Order.ID {
		t.Fatalf("duplicate client id returned order %s, want %s", third.Order.ID, first.Order.ID)
	}

	var orderCount int64
	if err := db.Model(&storage.ExchangeOrder{}).Where("market = ?", marketSymbol).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders failed: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("order count = %d, want 1", orderCount)
	}
	assertLockedBalance(t, db, userID, "USD", "20")

	command, err := repo.FindOrderCommandByClientID(context.Background(), userID, req.ClientOrderID)
	if err != nil {
		t.Fatalf("find command failed: %v", err)
	}
	if command.Status != storage.OrderCommandStatusCompleted || command.OrderID != string(first.Order.ID) {
		t.Fatalf("unexpected command state: %#v", command)
	}
	var commandLog storage.ExchangeOrderCommandLog
	if err := db.First(&commandLog, "command_id = ?", req.CommandID).Error; err != nil {
		t.Fatalf("load command log failed: %v", err)
	}
	if commandLog.Status != storage.OrderCommandLogStatusApplied || commandLog.AppliedOrderID != string(first.Order.ID) || commandLog.SequenceID == 0 {
		t.Fatalf("unexpected command log state: %#v", commandLog)
	}
	assertReservationState(t, repo, first.Order.ReservationID, storage.ReservationStatusActive, "20")
	var reservationCount int64
	if err := db.Model(&storage.ExchangeReservation{}).Where("order_id = ?", string(first.Order.ID)).Count(&reservationCount).Error; err != nil {
		t.Fatalf("count reservations failed: %v", err)
	}
	if reservationCount != 1 {
		t.Fatalf("reservation count = %d, want 1", reservationCount)
	}

	conflict := req
	conflict.Quantity = "3"
	if _, err := svc.Place(context.Background(), conflict); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("expected ErrInvalidOrder for command payload conflict, got %v", err)
	}
	if err := db.Model(&storage.ExchangeOrder{}).Where("market = ?", marketSymbol).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders after conflict failed: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("order count after conflict = %d, want 1", orderCount)
	}
	assertLockedBalance(t, db, userID, "USD", "20")
	if err := db.Model(&storage.ExchangeReservation{}).Where("order_id = ?", string(first.Order.ID)).Count(&reservationCount).Error; err != nil {
		t.Fatalf("count reservations after conflict failed: %v", err)
	}
	if reservationCount != 1 {
		t.Fatalf("reservation count after conflict = %d, want 1", reservationCount)
	}
	command, err = repo.FindOrderCommandByClientID(context.Background(), userID, req.ClientOrderID)
	if err != nil {
		t.Fatalf("find command after conflict failed: %v", err)
	}
	if command.Status != storage.OrderCommandStatusCompleted || command.OrderID != string(first.Order.ID) {
		t.Fatalf("command terminal state changed after conflict: %#v", command)
	}
	if err := db.First(&commandLog, "command_id = ?", req.CommandID).Error; err != nil {
		t.Fatalf("load command log after conflict failed: %v", err)
	}
	if commandLog.Status != storage.OrderCommandLogStatusApplied || commandLog.AppliedOrderID != string(first.Order.ID) {
		t.Fatalf("command log terminal state changed after conflict: %#v", commandLog)
	}
}

func TestCancelIsIdempotentForTerminalOrdersIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TCXL")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

	fundUser(t, repo, buyer, "USD", "100")
	open := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "10", "2")
	firstCancel, err := svc.Cancel(context.Background(), open.Order.ID, CancelRequest{UserID: buyer})
	if err != nil {
		t.Fatalf("first cancel failed: %v", err)
	}
	if firstCancel.Status != order.StatusCanceled {
		t.Fatalf("first cancel status = %s, want canceled", firstCancel.Status)
	}
	secondCancel, err := svc.Cancel(context.Background(), open.Order.ID, CancelRequest{UserID: buyer})
	if err != nil {
		t.Fatalf("second cancel should be idempotent, got %v", err)
	}
	if secondCancel.Status != order.StatusCanceled {
		t.Fatalf("second cancel status = %s, want canceled", secondCancel.Status)
	}
	assertLockedBalance(t, db, buyer, "USD", "0")
	assertOpenOrders(t, repo, marketSymbol, order.SideBuy, nil)

	fundUser(t, repo, seller, base, "1")
	fundUser(t, repo, buyer, "USD", "10")
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "10", "1")
	bid := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "10", "1")
	assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, bid.Order.ID, order.StatusFilled, "0")
	filledCancel, err := svc.Cancel(context.Background(), ask.Order.ID, CancelRequest{UserID: seller})
	if err != nil {
		t.Fatalf("cancel on filled order should be idempotent, got %v", err)
	}
	if filledCancel.Status != order.StatusFilled {
		t.Fatalf("filled cancel status = %s, want filled", filledCancel.Status)
	}
}

func TestCancelClampsDustRemainderReleaseIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TCDU")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)

	price := "1.01013"
	askQty := "4.700000000001744984"
	buyQty := "4.70000000000198"
	fundUser(t, repo, seller, base, askQty)
	fundUser(t, repo, buyer, "USD", "10")

	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, price, askQty)
	bid := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, price, buyQty)

	assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, bid.Order.ID, order.StatusPartiallyFilled, decimal.SubFloorZero(buyQty, askQty))
	assertBookSide(t, svc, marketSymbol, order.SideBuy, nil)
	assertOpenOrders(t, repo, marketSymbol, order.SideBuy, nil)

	canceled, err := svc.Cancel(context.Background(), bid.Order.ID, CancelRequest{UserID: buyer})
	if err != nil {
		t.Fatalf("cancel dust partial failed: %v", err)
	}
	if canceled.Status != order.StatusCanceled {
		t.Fatalf("cancel dust partial status = %s, want canceled", canceled.Status)
	}
	assertOrderState(t, repo, bid.Order.ID, order.StatusCanceled, "0")
	assertLockedBalance(t, db, buyer, "USD", "0")
	assertReservationState(t, repo, bid.Order.ReservationID, storage.ReservationStatusClosed, "0")
	assertNoNegativeBalances(t, db, buyer, seller)
}

func TestMatchJobFailureQuarantinesAfterMaxAttempts(t *testing.T) {
	repo, db := integrationRepository(t)
	_, marketSymbol := integrationMarket("TQNT")
	cleanupIntegrationMarket(t, db, marketSymbol)
	defer cleanupIntegrationMarket(t, db, marketSymbol)

	job := storage.ExchangeMatchJob{
		ID:          "mjob_TQNT_1",
		OrderID:     "ord_TQNT_1",
		Market:      marketSymbol,
		Status:      storage.MatchJobStatusProcessing,
		Attempts:    5,
		AvailableAt: time.Now().UTC(),
		LockedBy:    "test",
		LockedAt:    time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create match job failed: %v", err)
	}
	if err := repo.FailMatchJob(context.Background(), job.ID, "poison command", 5, time.Second); err != nil {
		t.Fatalf("fail match job failed: %v", err)
	}

	var got storage.ExchangeMatchJob
	if err := db.First(&got, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("load match job failed: %v", err)
	}
	if got.Status != storage.MatchJobStatusQuarantined {
		t.Fatalf("match job status = %s, want %s: %#v", got.Status, storage.MatchJobStatusQuarantined, got)
	}
	if got.LastError != "poison command" {
		t.Fatalf("last error = %q, want poison command", got.LastError)
	}
}

func TestOrderCommandLogClaimAndQuarantineIntegration(t *testing.T) {
	repo, db := integrationRepository(t)
	_, marketSymbol := integrationMarket("TCMD")
	cleanupIntegrationMarket(t, db, marketSymbol)
	defer cleanupIntegrationMarket(t, db, marketSymbol)

	first, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_TCMD_1",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-1",
		Payload:     `{"id":"1"}`,
	})
	if err != nil {
		t.Fatalf("append first command log failed: %v", err)
	}
	duplicate, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   first.CommandID,
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-1",
		Payload:     `{"id":"1"}`,
	})
	if err != nil {
		t.Fatalf("append duplicate command log failed: %v", err)
	}
	if duplicate.SequenceID != first.SequenceID {
		t.Fatalf("duplicate sequence = %d, want %d", duplicate.SequenceID, first.SequenceID)
	}
	second, err := repo.AppendOrderCommandLog(context.Background(), storage.OrderCommandLog{
		CommandID:   "cmd_TCMD_2",
		Market:      marketSymbol,
		Type:        storage.OrderCommandTypeNewOrder,
		Key:         marketSymbol,
		PayloadHash: "hash-2",
		Payload:     `{"id":"2"}`,
	})
	if err != nil {
		t.Fatalf("append second command log failed: %v", err)
	}
	if second.SequenceID != first.SequenceID+1 {
		t.Fatalf("second sequence = %d, want %d", second.SequenceID, first.SequenceID+1)
	}

	claimed, err := repo.ClaimOrderCommandLogs(context.Background(), marketSymbol, "worker-1", 10, time.Minute)
	if err != nil {
		t.Fatalf("claim command logs failed: %v", err)
	}
	if len(claimed) != 2 || claimed[0].CommandID != first.CommandID || claimed[1].CommandID != second.CommandID {
		t.Fatalf("unexpected claimed command logs: %#v", claimed)
	}
	if claimed[0].Status != storage.OrderCommandLogStatusProcessing || claimed[0].Attempts != 1 {
		t.Fatalf("unexpected claimed status: %#v", claimed[0])
	}

	if err := repo.FailOrderCommandLog(context.Background(), first.CommandID, "poison command", 1, time.Second); err != nil {
		t.Fatalf("fail command log failed: %v", err)
	}
	var got storage.ExchangeOrderCommandLog
	if err := db.First(&got, "command_id = ?", first.CommandID).Error; err != nil {
		t.Fatalf("load command log failed: %v", err)
	}
	if got.Status != storage.OrderCommandLogStatusQuarantined || got.LastError != "poison command" || got.QuarantinedAt.IsZero() {
		t.Fatalf("unexpected quarantined command log: %#v", got)
	}
}

type priceLevelWant struct {
	Price      string
	Quantity   string
	OrderCount int64
}

func integrationRepository(t *testing.T) (*storage.ExchangeRepository, *gorm.DB) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		_ = storage.LoadEnv(".")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("DATABASE_URL is required for exchange integration tests")
	}
	db, err := storage.ConnectWithOptions(storage.ConnectOptions{AutoMigrate: true})
	if err != nil {
		t.Skipf("postgres integration database unavailable: %v", err)
	}
	return storage.NewExchangeRepository(db), db
}

func integrationMarket(base string) (string, string) {
	base = strings.ToUpper(strings.TrimSpace(base))
	return base, base + "/USD"
}

func integrationUser(t *testing.T, suffix string) string {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(strings.ToLower(t.Name()))
	return fmt.Sprintf("it_%s_%s", name, suffix)
}

func integrationBookOrder(id string, clientID string, userID string, marketSymbol string, base string, side order.Side, price string, sequenceID uint64, now time.Time) storage.ExchangeOrder {
	return storage.ExchangeOrder{
		ID:                id,
		ClientOrderID:     clientID,
		UserID:            userID,
		Market:            marketSymbol,
		BaseAsset:         base,
		QuoteAsset:        "USD",
		Side:              string(side),
		Type:              string(order.TypeLimit),
		Status:            string(order.StatusOpen),
		TimeInForce:       string(order.TimeInForceGTC),
		Price:             price,
		StopPrice:         "0",
		Quantity:          "1",
		FilledQuantity:    "0",
		RemainingQuantity: "1",
		SequenceID:        sequenceID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func integrationOrderService(repo *storage.ExchangeRepository, marketSymbol string, base string) *Service {
	svc := NewService(market.NewRegistry([]market.Market{
		{Symbol: marketSymbol, BaseAsset: base, QuoteAsset: "USD", Enabled: true},
	}), repo)
	svc.priceBandBps = 0
	return svc
}

func cleanupIntegrationMarket(t *testing.T, db *gorm.DB, marketSymbol string, users ...string) {
	t.Helper()
	for _, model := range []any{
		&storage.ExchangeOrderEvent{},
		&storage.ExchangeTrade{},
		&storage.ExchangeCandle{},
		&storage.ExchangePriceLevel{},
		&storage.ExchangeMatchJob{},
		&storage.ExchangeActiveOrder{},
		&storage.ExchangeOrderCommandLog{},
		&storage.ExchangeMatchEventLog{},
		&storage.ExchangeOrderCommand{},
		&storage.ExchangeReservation{},
		&storage.ExchangeOrder{},
		&storage.ExchangeOrderCommandSequence{},
		&storage.ExchangeOrderSequence{},
	} {
		if err := db.Where("market = ?", marketSymbol).Delete(model).Error; err != nil {
			t.Fatalf("cleanup %T failed: %v", model, err)
		}
	}
	if len(users) == 0 {
		return
	}
	if err := db.Where("user_id IN ?", users).Delete(&storage.ExchangeBalanceEvent{}).Error; err != nil {
		t.Fatalf("cleanup balance events failed: %v", err)
	}
	if err := db.Where("user_id IN ?", users).Delete(&storage.ExchangeBalance{}).Error; err != nil {
		t.Fatalf("cleanup balances failed: %v", err)
	}
}

func fundUser(t *testing.T, repo *storage.ExchangeRepository, userID string, asset string, amount string) {
	t.Helper()
	ctx := context.Background()
	pendingID := balance.EventID(idgen.New("bev"))
	settleID := balance.EventID(idgen.New("bev"))
	if _, err := repo.MarkDepositPending(ctx, userID, asset, amount, pendingID); err != nil {
		t.Fatalf("fund pending %s %s failed: %v", userID, asset, err)
	}
	if _, err := repo.SettleDeposit(ctx, userID, asset, amount, settleID); err != nil {
		t.Fatalf("fund settle %s %s failed: %v", userID, asset, err)
	}
}

func placeIntegrationOrder(t *testing.T, svc *Service, userID string, marketSymbol string, side order.Side, orderType order.Type, price string, quantity string) *MatchResult {
	t.Helper()
	clientID := idgen.New("cli")
	result, err := svc.Place(context.Background(), PlaceRequest{
		ClientOrderID: clientID,
		UserID:        userID,
		Market:        marketSymbol,
		Side:          string(side),
		Type:          string(orderType),
		Price:         price,
		Quantity:      quantity,
	})
	if err != nil {
		t.Fatalf("place %s %s %s@%s failed: %v", orderType, side, quantity, price, err)
	}
	return result
}

func assertOrderState(t *testing.T, repo *storage.ExchangeRepository, id order.ID, status order.Status, remaining string) {
	t.Helper()
	item, err := repo.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatalf("get order %s failed: %v", id, err)
	}
	if item.Status != status || decimal.Cmp(item.RemainingQuantity, remaining) != 0 {
		t.Fatalf("order %s status/remaining = %s/%s, want %s/%s: %#v", id, item.Status, item.RemainingQuantity, status, remaining, item)
	}
}

func assertBookSide(t *testing.T, svc *Service, marketSymbol string, side order.Side, want []priceLevelWant) {
	t.Helper()
	book, err := svc.Book(context.Background(), marketSymbol, 10)
	if err != nil {
		t.Fatalf("book failed: %v", err)
	}
	got := book.Asks
	if side == order.SideBuy {
		got = book.Bids
	}
	if len(got) != len(want) {
		t.Fatalf("%s levels = %#v, want %#v", side, got, want)
	}
	for i := range want {
		if decimal.Cmp(got[i].Price, want[i].Price) != 0 || decimal.Cmp(got[i].Quantity, want[i].Quantity) != 0 || got[i].OrderCount != want[i].OrderCount {
			t.Fatalf("%s level %d = %#v, want %#v", side, i, got[i], want[i])
		}
	}
}

func assertOpenOrders(t *testing.T, repo *storage.ExchangeRepository, marketSymbol string, side order.Side, want []order.ID) {
	t.Helper()
	got, err := repo.ListOpenOrders(context.Background(), marketSymbol, side, 20)
	if err != nil {
		t.Fatalf("list open orders failed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("open %s orders = %#v, want %v", side, got, want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("open %s order %d = %s, want %s", side, i, got[i].ID, want[i])
		}
		if decimal.Cmp(got[i].RemainingQuantity, "0") <= 0 || got[i].Type == order.TypeMarket {
			t.Fatalf("non-bookable order returned as open: %#v", got[i])
		}
	}
}

func assertTradeCount(t *testing.T, repo *storage.ExchangeRepository, marketSymbol string, want int) {
	t.Helper()
	items, err := repo.ListMarketTrades(context.Background(), marketSymbol, 100)
	if err != nil {
		t.Fatalf("list trades failed: %v", err)
	}
	if len(items) != want {
		t.Fatalf("trade count = %d, want %d: %#v", len(items), want, items)
	}
}

func assertOrderEventExists(t *testing.T, db *gorm.DB, orderID order.ID, eventType order.EventType) {
	t.Helper()
	var count int64
	if err := db.Model(&storage.ExchangeOrderEvent{}).
		Where("order_id = ? AND type = ?", string(orderID), string(eventType)).
		Count(&count).Error; err != nil {
		t.Fatalf("count order events failed: %v", err)
	}
	if count == 0 {
		t.Fatalf("missing %s event for order %s", eventType, orderID)
	}
}

func assertNoNegativeBalances(t *testing.T, db *gorm.DB, users ...string) {
	t.Helper()
	var items []storage.ExchangeBalance
	if err := db.Where("user_id IN ?", users).Find(&items).Error; err != nil {
		t.Fatalf("list balances failed: %v", err)
	}
	for _, item := range items {
		if decimal.Cmp(item.Available, "0") < 0 || decimal.Cmp(item.Locked, "0") < 0 || decimal.Cmp(item.Pending, "0") < 0 {
			t.Fatalf("negative balance found: %#v", item)
		}
	}
}

func assertLockedBalance(t *testing.T, db *gorm.DB, userID string, asset string, want string) {
	t.Helper()
	var item storage.ExchangeBalance
	if err := db.Where(&storage.ExchangeBalance{UserID: userID, Asset: asset}).First(&item).Error; err != nil {
		t.Fatalf("get balance %s/%s failed: %v", userID, asset, err)
	}
	if decimal.Cmp(item.Locked, want) != 0 {
		t.Fatalf("locked balance %s/%s = %s, want %s: %#v", userID, asset, item.Locked, want, item)
	}
}

func assertBalance(t *testing.T, db *gorm.DB, userID string, asset string, available string, locked string, pending string) {
	t.Helper()
	var item storage.ExchangeBalance
	if err := db.Where(&storage.ExchangeBalance{UserID: userID, Asset: asset}).First(&item).Error; err != nil {
		t.Fatalf("get balance %s/%s failed: %v", userID, asset, err)
	}
	if decimal.Cmp(item.Available, available) != 0 || decimal.Cmp(item.Locked, locked) != 0 || decimal.Cmp(item.Pending, pending) != 0 {
		t.Fatalf("balance %s/%s = available %s locked %s pending %s, want %s/%s/%s: %#v", userID, asset, item.Available, item.Locked, item.Pending, available, locked, pending, item)
	}
}

func assertReservationState(t *testing.T, repo *storage.ExchangeRepository, id string, status string, remaining string) {
	t.Helper()
	item, err := repo.GetReservation(context.Background(), id)
	if err != nil {
		t.Fatalf("get reservation %s failed: %v", id, err)
	}
	if item.Status != status || decimal.Cmp(item.RemainingAmount, remaining) != 0 {
		t.Fatalf("reservation %s status/remaining = %s/%s, want %s/%s: %#v", id, item.Status, item.RemainingAmount, status, remaining, item)
	}
}

func decimalQuo(left string, right string) string {
	return decimal.String(new(big.Rat).Quo(decimal.Parse(left), decimal.Parse(right)))
}
