package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	storage "exchange/internal/adapters/storage/postgres"
	"exchange/internal/core/balance"
	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
)

func TestLimitBuyLocksQuoteBalanceIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TLBQ")
	buyer := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer)
	fundUser(t, repo, buyer, "USD", "100")

	// Act.
	bid := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "10", "2")

	// Assert.
	assertOrderState(t, repo, bid.Order.ID, order.StatusOpen, "2")
	assertBalance(t, db, buyer, "USD", "80", "20", "0")
	assertBalanceTotal(t, db, buyer, "USD", "100")
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  buyer,
		Asset:   "USD",
		Type:    balance.EventReserve,
		OrderID: string(bid.Order.ID),
	}, "20", 1)
	assertReservationState(t, repo, bid.Order.ReservationID, storage.ReservationStatusActive, "20")
}

func TestLimitSellLocksBaseBalanceIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TLSB")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, seller)
	fundUser(t, repo, seller, base, "5")

	// Act.
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "10", "2")

	// Assert.
	assertOrderState(t, repo, ask.Order.ID, order.StatusOpen, "2")
	assertBalance(t, db, seller, base, "3", "2", "0")
	assertBalanceTotal(t, db, seller, base, "5")
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  seller,
		Asset:   base,
		Type:    balance.EventReserve,
		OrderID: string(ask.Order.ID),
	}, "2", 1)
	assertReservationState(t, repo, ask.Order.ReservationID, storage.ReservationStatusActive, "2")
}

func TestCancelUnlocksRemainingBalanceIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TCUB")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	fundUser(t, repo, buyer, "USD", "30")
	fundUser(t, repo, seller, base, "1")
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "10", "1")
	bid := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "10", "2")

	// Act.
	canceled, err := svc.Cancel(context.Background(), bid.Order.ID, CancelRequest{UserID: buyer})
	if err != nil {
		t.Fatalf("cancel partial bid failed: %v", err)
	}

	// Assert.
	if canceled.Status != order.StatusCanceled {
		t.Fatalf("cancel status = %s, want canceled", canceled.Status)
	}
	assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, bid.Order.ID, order.StatusCanceled, "0")
	assertBalance(t, db, buyer, "USD", "20", "0", "0")
	assertBalance(t, db, buyer, base, "1", "0", "0")
	assertBalance(t, db, seller, "USD", "10", "0", "0")
	assertBalance(t, db, seller, base, "0", "0", "0")
	assertBalanceTotal(t, db, buyer, "USD", "20")
	assertBalanceTotal(t, db, buyer, base, "1")
	assertNoNegativeBalances(t, db, buyer, seller)
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  buyer,
		Asset:   "USD",
		Type:    balance.EventRelease,
		OrderID: string(bid.Order.ID),
		TradeID: "",
	}, "20", 2)
	assertReservationState(t, repo, bid.Order.ReservationID, storage.ReservationStatusClosed, "0")
}

func TestLedgerBalancesMatchWalletBalancesIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TLGR")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	fundUser(t, repo, buyer, "USD", "25")
	fundUser(t, repo, seller, base, "2")
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "10", "1")

	// Act.
	buy := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeMarket, "10", "1")
	trades, err := repo.ListMarketTrades(context.Background(), marketSymbol, 10)
	if err != nil {
		t.Fatalf("list trades failed: %v", err)
	}

	// Assert.
	if len(trades) != 1 {
		t.Fatalf("trades = %#v, want one trade", trades)
	}
	tradeID := string(trades[0].ID)
	assertOrderState(t, repo, ask.Order.ID, order.StatusFilled, "0")
	assertOrderState(t, repo, buy.Order.ID, order.StatusFilled, "0")
	assertBalance(t, db, buyer, "USD", "15", "0", "0")
	assertBalance(t, db, buyer, base, "1", "0", "0")
	assertBalance(t, db, seller, "USD", "10", "0", "0")
	assertBalance(t, db, seller, base, "1", "0", "0")
	assertUsersAssetTotal(t, db, []string{buyer, seller}, "USD", "25")
	assertUsersAssetTotal(t, db, []string{buyer, seller}, base, "2")
	assertPositiveBalanceEvents(t, db, buyer, seller)
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  buyer,
		Asset:   "USD",
		Type:    balance.EventDebitLocked,
		OrderID: string(buy.Order.ID),
		TradeID: tradeID,
	}, "10", 1)
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  buyer,
		Asset:   base,
		Type:    balance.EventSettlementReceive,
		OrderID: string(buy.Order.ID),
		TradeID: tradeID,
	}, "1", 1)
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  seller,
		Asset:   base,
		Type:    balance.EventDebitLocked,
		OrderID: string(ask.Order.ID),
		TradeID: tradeID,
	}, "1", 1)
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  seller,
		Asset:   "USD",
		Type:    balance.EventSettlementReceive,
		OrderID: string(ask.Order.ID),
		TradeID: tradeID,
	}, "10", 1)
}

func TestConcurrentOrdersDoNotDoubleSpendIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TCON")
	buyer := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer)
	fundUser(t, repo, buyer, "USD", "100")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var successes int
	unexpected := make([]error, 0)

	// Act.
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Place(context.Background(), PlaceRequest{
				ClientOrderID: fmt.Sprintf("client_%s_%02d", strings.ToLower(base), i),
				UserID:        buyer,
				Market:        marketSymbol,
				Side:          string(order.SideBuy),
				Type:          string(order.TypeLimit),
				Price:         "10",
				Quantity:      "1",
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
				return
			}
			if !errors.Is(err, balance.ErrInsufficientBalance) {
				unexpected = append(unexpected, err)
			}
		}(i)
	}
	wg.Wait()

	// Assert.
	if len(unexpected) > 0 {
		t.Fatalf("unexpected concurrent place errors: %v", unexpected)
	}
	if successes > 10 {
		t.Fatalf("successful orders = %d, want at most 10 funded orders", successes)
	}
	wantLocked := fmt.Sprintf("%d", successes*10)
	wantAvailable := fmt.Sprintf("%d", 100-successes*10)
	assertBalance(t, db, buyer, "USD", wantAvailable, wantLocked, "0")
	assertBalanceTotal(t, db, buyer, "USD", "100")
	assertNoNegativeBalances(t, db, buyer)
	assertOpenOrderCountAtMost(t, repo, marketSymbol, order.SideBuy, 10)
}

func TestSamePriceConcurrentOrdersKeepSequenceFIFOIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TFIF")
	buyer := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	sellers := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		sellers = append(sellers, integrationUser(t, fmt.Sprintf("seller-%02d", i)))
	}
	users := append([]string{buyer}, sellers...)
	cleanupIntegrationMarket(t, db, marketSymbol, users...)
	defer cleanupIntegrationMarket(t, db, marketSymbol, users...)
	fundUser(t, repo, buyer, "USD", "120")
	for _, seller := range sellers {
		fundUser(t, repo, seller, base, "1")
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(sellers))
	placed := make(chan order.ID, len(sellers))
	start := make(chan struct{})

	// Act.
	for i, seller := range sellers {
		wg.Add(1)
		go func(i int, seller string) {
			defer wg.Done()
			<-start
			result, err := svc.Place(context.Background(), PlaceRequest{
				ClientOrderID: fmt.Sprintf("client_%s_fifo_%02d", strings.ToLower(base), i),
				UserID:        seller,
				Market:        marketSymbol,
				Side:          string(order.SideSell),
				Type:          string(order.TypeLimit),
				Price:         "10",
				Quantity:      "1",
			})
			if err != nil {
				errs <- err
				return
			}
			placed <- result.Order.ID
		}(i, seller)
	}
	close(start)
	wg.Wait()
	close(errs)
	close(placed)
	for err := range errs {
		if err != nil {
			t.Fatalf("place concurrent seller failed: %v", err)
		}
	}
	if len(placed) != len(sellers) {
		t.Fatalf("placed sellers = %d, want %d", len(placed), len(sellers))
	}
	sequenceByOrder := make(map[order.ID]uint64, len(sellers))
	for id := range placed {
		item, err := repo.GetOrder(context.Background(), id)
		if err != nil {
			t.Fatalf("get placed order %s failed: %v", id, err)
		}
		sequenceByOrder[id] = item.SequenceID
	}
	buy, err := svc.Place(context.Background(), PlaceRequest{
		ClientOrderID: "client_" + strings.ToLower(base) + "_sweep",
		UserID:        buyer,
		Market:        marketSymbol,
		Side:          string(order.SideBuy),
		Type:          string(order.TypeMarket),
		Price:         "10",
		Quantity:      fmt.Sprintf("%d", len(sellers)),
	})
	if err != nil {
		t.Fatalf("sweep same-price sellers failed: %v", err)
	}

	// Assert.
	if len(buy.Trades) != len(sellers) {
		t.Fatalf("trade count = %d, want %d: %#v", len(buy.Trades), len(sellers), buy.Trades)
	}
	var previous uint64
	for idx, item := range buy.Trades {
		seq := sequenceByOrder[item.MakerOrderID]
		if seq == 0 {
			t.Fatalf("trade %d references unknown maker sequence: %#v", idx, item)
		}
		if idx > 0 && seq < previous {
			t.Fatalf("same-price FIFO violated at trade %d: previous seq %d current seq %d trades=%#v", idx, previous, seq, buy.Trades)
		}
		previous = seq
	}
	assertBookSide(t, svc, marketSymbol, order.SideSell, nil)
	assertNoNegativeBalances(t, db, users...)
}

func TestCancelVsMatchRaceDoesNotDoubleSettleIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TCVM")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	fundUser(t, repo, buyer, "USD", "10")
	fundUser(t, repo, seller, base, "1")
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "10", "1")

	start := make(chan struct{})
	var wg sync.WaitGroup
	var cancelErr error
	var buyResult *MatchResult
	var buyErr error

	// Act.
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, cancelErr = svc.Cancel(context.Background(), ask.Order.ID, CancelRequest{UserID: seller})
	}()
	go func() {
		defer wg.Done()
		<-start
		buyResult, buyErr = svc.Place(context.Background(), PlaceRequest{
			ClientOrderID: "client_" + strings.ToLower(base) + "_race_buy",
			UserID:        buyer,
			Market:        marketSymbol,
			Side:          string(order.SideBuy),
			Type:          string(order.TypeMarket),
			Price:         "10",
			Quantity:      "1",
		})
	}()
	close(start)
	wg.Wait()

	// Assert.
	if cancelErr != nil {
		t.Fatalf("cancel race returned unexpected error: %v", cancelErr)
	}
	if buyErr != nil {
		t.Fatalf("buy race returned unexpected error: %v", buyErr)
	}
	finalAsk, err := repo.GetOrder(context.Background(), ask.Order.ID)
	if err != nil {
		t.Fatalf("get ask failed: %v", err)
	}
	trades, err := repo.ListMarketTrades(context.Background(), marketSymbol, 10)
	if err != nil {
		t.Fatalf("list trades failed: %v", err)
	}
	if finalAsk.Status == order.StatusFilled {
		if len(trades) != 1 || buyResult == nil || buyResult.Order.Status != order.StatusFilled {
			t.Fatalf("filled race should have exactly one filled buy and one trade: ask=%#v buy=%#v trades=%#v", finalAsk, buyResult, trades)
		}
		assertBalance(t, db, buyer, "USD", "0", "0", "0")
		assertBalance(t, db, buyer, base, "1", "0", "0")
		assertBalance(t, db, seller, "USD", "10", "0", "0")
		assertBalance(t, db, seller, base, "0", "0", "0")
	} else if finalAsk.Status == order.StatusCanceled {
		if len(trades) != 0 || buyResult == nil || buyResult.Order.Status != order.StatusExpired {
			t.Fatalf("canceled race should have expired buy and no trades: ask=%#v buy=%#v trades=%#v", finalAsk, buyResult, trades)
		}
		assertBalance(t, db, buyer, "USD", "10", "0", "0")
		assertBalance(t, db, seller, base, "1", "0", "0")
	} else {
		t.Fatalf("ask ended in invalid race status: %#v", finalAsk)
	}
	assertUsersAssetTotal(t, db, []string{buyer, seller}, "USD", "10")
	assertUsersAssetTotal(t, db, []string{buyer, seller}, base, "1")
	assertNoNegativeBalances(t, db, buyer, seller)
}

func TestOrderCreationAndLockRollbackIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TROL")
	buyer := integrationUser(t, "buyer")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer)
	fundUser(t, repo, buyer, "USD", "100")
	item, err := svc.buildOrder(PlaceRequest{
		ClientOrderID: "client_" + strings.ToLower(base) + "_rollback",
		UserID:        buyer,
		Market:        marketSymbol,
		Side:          string(order.SideBuy),
		Type:          string(order.TypeLimit),
		Price:         "10",
		Quantity:      "2",
	})
	if err != nil {
		t.Fatalf("build rollback order failed: %v", err)
	}
	item.SequenceID = 1
	injected := errors.New("injected rollback")

	// Act.
	err = repo.Transaction(context.Background(), func(tx *storage.ExchangeRepository) error {
		if err := tx.CreateOrder(context.Background(), item); err != nil {
			return err
		}
		if err := tx.ReserveOrderFunds(context.Background(), item, balance.EventID(idgen.New("bev"))); err != nil {
			return err
		}
		return injected
	})

	// Assert.
	if !errors.Is(err, injected) {
		t.Fatalf("rollback error = %v, want injected", err)
	}
	if _, err := repo.GetOrder(context.Background(), item.ID); err == nil {
		t.Fatalf("rolled back order %s still exists", item.ID)
	}
	assertBalance(t, db, buyer, "USD", "100", "0", "0")
	assertBalanceEventSum(t, db, balanceEventFilter{
		UserID:  buyer,
		Asset:   "USD",
		Type:    balance.EventReserve,
		OrderID: string(item.ID),
	}, "0", 0)
	var reservationCount int64
	if err := db.Model(&storage.ExchangeReservation{}).Where("order_id = ?", string(item.ID)).Count(&reservationCount).Error; err != nil {
		t.Fatalf("count reservations failed: %v", err)
	}
	if reservationCount != 0 {
		t.Fatalf("rolled back reservation count = %d, want 0", reservationCount)
	}
}

func TestAtomicSettlementRollbackIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TASR")
	buyer := integrationUser(t, "buyer")
	seller := integrationUser(t, "seller")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	defer cleanupIntegrationMarket(t, db, marketSymbol, buyer, seller)
	fundUser(t, repo, buyer, "USD", "10")
	fundUser(t, repo, seller, base, "1")
	bid := placeIntegrationOrder(t, svc, buyer, marketSymbol, order.SideBuy, order.TypeLimit, "10", "1")
	ask := placeIntegrationOrder(t, svc, seller, marketSymbol, order.SideSell, order.TypeLimit, "11", "1")
	item := trade.Trade{
		ID:            trade.ID(idgen.New("trd")),
		Market:        marketSymbol,
		BaseAsset:     base,
		QuoteAsset:    "USD",
		MakerOrderID:  bid.Order.ID,
		TakerOrderID:  ask.Order.ID,
		MakerUserID:   buyer,
		TakerUserID:   seller,
		TakerSide:     order.SideSell,
		Price:         "10",
		Quantity:      "1",
		QuoteQuantity: "10",
		CreatedAt:     time.Now().UTC(),
	}
	injected := errors.New("injected settlement rollback")

	// Act.
	err := repo.Transaction(context.Background(), func(tx *storage.ExchangeRepository) error {
		updatedBid := bid.Order
		updatedBid.Status = order.StatusFilled
		updatedBid.FilledQuantity = "1"
		updatedBid.RemainingQuantity = "0"
		updatedAsk := ask.Order
		updatedAsk.Status = order.StatusFilled
		updatedAsk.FilledQuantity = "1"
		updatedAsk.RemainingQuantity = "0"
		if err := tx.SaveOrder(context.Background(), updatedBid); err != nil {
			return err
		}
		if err := tx.SaveOrder(context.Background(), updatedAsk); err != nil {
			return err
		}
		if err := tx.SettleTrades(context.Background(), []trade.Trade{item}); err != nil {
			return err
		}
		if err := tx.CreateTrades(context.Background(), []trade.Trade{item}); err != nil {
			return err
		}
		return injected
	})

	// Assert.
	if !errors.Is(err, injected) {
		t.Fatalf("rollback error = %v, want injected", err)
	}
	assertOrderState(t, repo, bid.Order.ID, order.StatusOpen, "1")
	assertOrderState(t, repo, ask.Order.ID, order.StatusOpen, "1")
	assertBalance(t, db, buyer, "USD", "0", "10", "0")
	assertBalance(t, db, seller, base, "0", "1", "0")
	assertTradeCount(t, repo, marketSymbol, 0)
	assertUsersAssetTotal(t, db, []string{buyer, seller}, "USD", "10")
	assertUsersAssetTotal(t, db, []string{buyer, seller}, base, "1")
}

func TestSelfTradeIsPreventedIntegration(t *testing.T) {
	// Arrange.
	repo, db := integrationRepository(t)
	base, marketSymbol := integrationMarket("TSTP")
	user := integrationUser(t, "user")
	svc := integrationOrderService(repo, marketSymbol, base)
	cleanupIntegrationMarket(t, db, marketSymbol, user)
	defer cleanupIntegrationMarket(t, db, marketSymbol, user)
	fundUser(t, repo, user, "USD", "10")
	fundUser(t, repo, user, base, "1")
	ask := placeIntegrationOrder(t, svc, user, marketSymbol, order.SideSell, order.TypeLimit, "10", "1")

	// Act.
	bid := placeIntegrationOrder(t, svc, user, marketSymbol, order.SideBuy, order.TypeLimit, "10", "1")

	// Assert.
	assertOrderState(t, repo, ask.Order.ID, order.StatusOpen, "1")
	assertOrderState(t, repo, bid.Order.ID, order.StatusExpired, "1")
	assertTradeCount(t, repo, marketSymbol, 0)
	assertBookSide(t, svc, marketSymbol, order.SideSell, []priceLevelWant{{Price: "10", Quantity: "1", OrderCount: 1}})
	assertBookSide(t, svc, marketSymbol, order.SideBuy, nil)
	assertBalance(t, db, user, "USD", "10", "0", "0")
	assertBalance(t, db, user, base, "0", "1", "0")
	assertNoNegativeBalances(t, db, user)
}

type balanceEventFilter struct {
	UserID  string
	Asset   string
	Type    balance.EventType
	OrderID string
	TradeID string
}

func assertBalanceEventSum(t *testing.T, db *gorm.DB, filter balanceEventFilter, wantAmount string, wantCount int) {
	t.Helper()
	var events []storage.ExchangeBalanceEvent
	query := db.Where(&storage.ExchangeBalanceEvent{
		UserID:  filter.UserID,
		Asset:   strings.ToUpper(filter.Asset),
		Type:    string(filter.Type),
		OrderID: filter.OrderID,
	})
	if filter.TradeID != "" {
		query = query.Where("trade_id = ?", filter.TradeID)
	} else {
		query = query.Where("trade_id = ''")
	}
	if err := query.Find(&events).Error; err != nil {
		t.Fatalf("list balance events failed: %v", err)
	}
	if len(events) != wantCount {
		t.Fatalf("balance event count = %d, want %d: %#v", len(events), wantCount, events)
	}
	sum := "0"
	for _, event := range events {
		sum = decimal.Add(sum, event.Amount)
	}
	if decimal.Cmp(sum, wantAmount) != 0 {
		t.Fatalf("balance event sum = %s, want %s: %#v", sum, wantAmount, events)
	}
}

func assertBalanceTotal(t *testing.T, db *gorm.DB, userID string, asset string, want string) {
	t.Helper()
	var item storage.ExchangeBalance
	if err := db.Where(&storage.ExchangeBalance{UserID: userID, Asset: strings.ToUpper(asset)}).First(&item).Error; err != nil {
		t.Fatalf("get balance %s/%s failed: %v", userID, asset, err)
	}
	total := decimal.Add(decimal.Add(item.Available, item.Locked), item.Pending)
	if decimal.Cmp(total, want) != 0 {
		t.Fatalf("balance total %s/%s = %s, want %s: %#v", userID, asset, total, want, item)
	}
}

func assertUsersAssetTotal(t *testing.T, db *gorm.DB, users []string, asset string, want string) {
	t.Helper()
	var items []storage.ExchangeBalance
	if err := db.Where("user_id IN ? AND asset = ?", users, strings.ToUpper(asset)).Find(&items).Error; err != nil {
		t.Fatalf("list balances failed: %v", err)
	}
	total := "0"
	for _, item := range items {
		total = decimal.Add(total, decimal.Add(decimal.Add(item.Available, item.Locked), item.Pending))
	}
	if decimal.Cmp(total, want) != 0 {
		t.Fatalf("asset total %s = %s, want %s: %#v", asset, total, want, items)
	}
}

func assertPositiveBalanceEvents(t *testing.T, db *gorm.DB, users ...string) {
	t.Helper()
	var events []storage.ExchangeBalanceEvent
	if err := db.Where("user_id IN ?", users).Find(&events).Error; err != nil {
		t.Fatalf("list balance events failed: %v", err)
	}
	for _, event := range events {
		if decimal.Cmp(event.Amount, "0") <= 0 {
			t.Fatalf("non-positive balance event found: %#v", event)
		}
	}
}

func assertOpenOrderCountAtMost(t *testing.T, repo *storage.ExchangeRepository, marketSymbol string, side order.Side, max int) {
	t.Helper()
	items, err := repo.ListOpenOrders(context.Background(), marketSymbol, side, 100)
	if err != nil {
		t.Fatalf("list open orders failed: %v", err)
	}
	if len(items) > max {
		t.Fatalf("open order count = %d, want at most %d: %#v", len(items), max, items)
	}
}
