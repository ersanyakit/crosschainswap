package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/core/balance"
	"exchange/internal/core/decimal"
	"exchange/internal/core/market"
	"exchange/internal/core/matching"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
	"exchange/internal/core/trade"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
)

var (
	ErrInvalidOrder       = errors.New("invalid order")
	ErrUnknownMarket      = errors.New("market is not registered")
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderNotCancelable = errors.New("order is not cancelable")
	ErrPriceBandExceeded  = errors.New("price band exceeded")
)

type Service struct {
	markets      market.Registry
	repo         *postgres.ExchangeRepository
	priceBandBps int64
	publish      Publisher
}

type Publisher func([]byte)

type PlaceRequest struct {
	ClientOrderID string `json:"client_order_id"`
	UserID        string `json:"user_id"`
	Market        string `json:"market"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"time_in_force"`
	Price         string `json:"price"`
	StopPrice     string `json:"stop_price"`
	Quantity      string `json:"quantity"`
}

type CancelRequest struct {
	UserID string `json:"user_id"`
}

type TriggerRequest struct {
	Market    string `json:"market"`
	LastPrice string `json:"last_price"`
}

type HistoryRequest struct {
	UserID string
	Market string
	Status string
	Limit  int
}

type MarketHistoryRequest struct {
	Market   string
	Interval string
	Limit    int
}

type BalanceAmountRequest struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

type WithdrawalRequest struct {
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	ChainKey string `json:"chain_key"`
	Address  string `json:"address"`
}

type WalletRequest struct {
	ChainKey string `json:"chain_key"`
	Address  string `json:"address"`
}

type MatchResult struct {
	Order  order.Order   `json:"order"`
	Trades []trade.Trade `json:"trades"`
}

type MarketSummary struct {
	Symbol     string `json:"symbol"`
	BaseAsset  string `json:"base_asset"`
	QuoteAsset string `json:"quote_asset"`
	Enabled    bool   `json:"enabled"`

	LastPrice string `json:"last_price"`
	Change24h string `json:"change_24h"`
	High24h   string `json:"high_24h"`
	Low24h    string `json:"low_24h"`
	Volume24h string `json:"volume_24h"`
	Liquidity string `json:"liquidity"`
}

func NewService(markets market.Registry, repo *postgres.ExchangeRepository) *Service {
	return &Service{markets: markets, repo: repo, priceBandBps: defaultPriceBandBps()}
}

func (s *Service) SetPublisher(publisher Publisher) {
	s.publish = publisher
}

func (s *Service) Place(ctx context.Context, req PlaceRequest) (*MatchResult, error) {
	item, err := s.buildOrder(req)
	if err != nil {
		return nil, err
	}

	result := &MatchResult{}
	publishResults := make([]MatchResult, 0, 1)
	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		if item.ClientOrderID != "" {
			existing, err := tx.FindOrderByClientID(ctx, item.UserID, item.ClientOrderID)
			if err == nil {
				result.Order = *existing
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err := s.validatePriceBand(ctx, tx, item); err != nil {
			return err
		}
		if err := tx.CreateOrder(ctx, item); err != nil {
			return err
		}
		if err := tx.ReserveOrderFunds(ctx, item, balance.EventID(idgen.New("bev"))); err != nil {
			return err
		}
		if err := tx.CreateOrderEvents(ctx, []order.Event{newOrderEvent(item, order.EventOrderAccepted, "")}); err != nil {
			return err
		}
		if item.Status == order.StatusPendingStop {
			result.Order = item
			publishResults = append(publishResults, MatchResult{Order: item})
			return nil
		}

		levels := newLevelTracker()
		updated, trades, err := s.match(ctx, tx, item, levels)
		if err != nil {
			return err
		}
		if err := tx.SaveOrder(ctx, updated); err != nil {
			return err
		}
		if err := tx.SettleTrades(ctx, trades); err != nil {
			return err
		}
		if shouldReleaseUnfilled(updated) {
			if err := tx.ReleaseOrderFunds(ctx, updated, balance.EventID(idgen.New("bev"))); err != nil {
				return err
			}
		}
		if err := tx.CreateTrades(ctx, trades); err != nil {
			return err
		}
		if err := tx.CreateOrderEvents(ctx, terminalEvents(updated, trades)); err != nil {
			return err
		}
		activated, err := s.activateTriggeredStops(ctx, tx, updated.Market, lastTradePrice(trades), levels)
		if err != nil {
			return err
		}
		if err := tx.RefreshPriceLevels(ctx, levels.keys()); err != nil {
			return err
		}
		result.Order = updated
		result.Trades = trades
		publishResults = append(publishResults, MatchResult{Order: updated, Trades: trades})
		publishResults = append(publishResults, activated...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, publishResult := range publishResults {
		s.publishOrderUpdate("exchange.order_accepted", publishResult.Order, publishResult.Trades)
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, id order.ID) (*order.Order, error) {
	item, err := s.repo.GetOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}
	return item, nil
}

func (s *Service) Cancel(ctx context.Context, id order.ID, req CancelRequest) (*order.Order, error) {
	var canceled order.Order
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		item, err := tx.GetOrderForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrOrderNotFound, id)
		}
		if strings.TrimSpace(req.UserID) != "" && item.UserID != req.UserID {
			return fmt.Errorf("%w: user mismatch", ErrInvalidOrder)
		}
		if !cancelable(item.Status) {
			return fmt.Errorf("%w: %s", ErrOrderNotCancelable, item.Status)
		}
		release := *item
		item.Status = order.StatusCanceled
		item.RemainingQuantity = "0"
		item.UpdatedAt = time.Now()
		if err := tx.SaveOrder(ctx, *item); err != nil {
			return err
		}
		if err := tx.ReleaseOrderFunds(ctx, release, balance.EventID(idgen.New("bev"))); err != nil {
			return err
		}
		if err := tx.CreateOrderEvents(ctx, []order.Event{newOrderEvent(*item, order.EventOrderCanceled, "")}); err != nil {
			return err
		}
		levels := newLevelTracker()
		levels.touch(*item)
		if err := tx.RefreshPriceLevels(ctx, levels.keys()); err != nil {
			return err
		}
		canceled = *item
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publishOrderUpdate("exchange.order_canceled", canceled, nil)
	return &canceled, nil
}

func (s *Service) Book(ctx context.Context, marketSymbol string, depth int) (*orderbook.Snapshot, error) {
	marketSymbol = normalizeMarket(marketSymbol)
	if _, ok := s.markets.Get(marketSymbol); !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownMarket, marketSymbol)
	}
	bids, err := s.repo.ListPriceLevels(ctx, marketSymbol, order.SideBuy, depth)
	if err != nil {
		return nil, err
	}
	asks, err := s.repo.ListPriceLevels(ctx, marketSymbol, order.SideSell, depth)
	if err != nil {
		return nil, err
	}
	return &orderbook.Snapshot{Market: marketSymbol, Bids: bids, Asks: asks}, nil
}

func (s *Service) OrderHistory(ctx context.Context, req HistoryRequest) ([]order.Order, error) {
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	marketSymbol, err := s.optionalMarket(req.Market)
	if err != nil {
		return nil, err
	}
	status := order.Status(strings.ToLower(strings.TrimSpace(req.Status)))
	if status != "" && !knownStatus(status) {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidOrder)
	}
	return s.repo.ListOrders(ctx, userID, marketSymbol, status, req.Limit)
}

func (s *Service) UserTrades(ctx context.Context, req HistoryRequest) ([]trade.Trade, error) {
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	marketSymbol, err := s.optionalMarket(req.Market)
	if err != nil {
		return nil, err
	}
	return s.repo.ListUserTrades(ctx, userID, marketSymbol, req.Limit)
}

func (s *Service) MarketTrades(ctx context.Context, req MarketHistoryRequest) ([]trade.Trade, error) {
	marketSymbol, err := s.requiredMarket(req.Market)
	if err != nil {
		return nil, err
	}
	return s.repo.ListMarketTrades(ctx, marketSymbol, req.Limit)
}

func (s *Service) Candles(ctx context.Context, req MarketHistoryRequest) ([]trade.Candle, error) {
	marketSymbol, err := s.requiredMarket(req.Market)
	if err != nil {
		return nil, err
	}
	interval := strings.ToLower(strings.TrimSpace(req.Interval))
	if interval == "" {
		interval = "1m"
	}
	if _, ok := trade.IntervalByKey(interval); !ok {
		return nil, fmt.Errorf("%w: unsupported candle interval", ErrInvalidOrder)
	}
	return s.repo.ListCandles(ctx, marketSymbol, interval, req.Limit)
}

func (s *Service) TriggerStops(ctx context.Context, req TriggerRequest) ([]MatchResult, error) {
	marketSymbol := normalizeMarket(req.Market)
	lastPrice, err := parsePositiveDecimal(req.LastPrice, "last_price")
	if err != nil {
		return nil, err
	}
	if _, ok := s.markets.Get(marketSymbol); !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownMarket, marketSymbol)
	}

	results := make([]MatchResult, 0)
	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		levels := newLevelTracker()
		activated, err := s.activateTriggeredStops(ctx, tx, marketSymbol, decimal.String(lastPrice), levels)
		if err != nil {
			return err
		}
		results = activated
		return tx.RefreshPriceLevels(ctx, levels.keys())
	})
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		s.publishOrderUpdate("exchange.order_updated", result.Order, result.Trades)
	}
	return results, nil
}

func (s *Service) Markets() []market.Market {
	return s.markets.All()
}

func (s *Service) MarketSummaries(ctx context.Context) ([]MarketSummary, error) {
	markets := s.markets.All()
	out := make([]MarketSummary, 0, len(markets))
	for _, item := range markets {
		summary, err := s.marketSummary(ctx, item)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

func (s *Service) marketSummary(ctx context.Context, item market.Market) (MarketSummary, error) {
	summary := MarketSummary{
		Symbol:     item.Symbol,
		BaseAsset:  item.BaseAsset,
		QuoteAsset: item.QuoteAsset,
		Enabled:    item.Enabled,
		LastPrice:  defaultMarketPrice(item.BaseAsset),
		Change24h:  "0",
		High24h:    defaultMarketPrice(item.BaseAsset),
		Low24h:     defaultMarketPrice(item.BaseAsset),
		Volume24h:  "0",
		Liquidity:  "0",
	}
	if s.repo == nil {
		return summary, nil
	}

	candles, err := s.repo.ListCandles(ctx, item.Symbol, "1m", 1440)
	if err != nil {
		return summary, err
	}
	if len(candles) > 0 {
		summary = applyCandleStats(summary, candles)
	} else if last, err := s.repo.LastTrade(ctx, item.Symbol); err == nil {
		summary.LastPrice = last.Price
		summary.High24h = last.Price
		summary.Low24h = last.Price
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, err
	}

	liquidity, err := s.marketLiquidity(ctx, item.Symbol)
	if err != nil {
		return summary, err
	}
	summary.Liquidity = liquidity
	return summary, nil
}

func (s *Service) buildOrder(req PlaceRequest) (order.Order, error) {
	marketSymbol := normalizeMarket(req.Market)
	m, ok := s.markets.Get(marketSymbol)
	if !ok || !m.Enabled {
		return order.Order{}, fmt.Errorf("%w: %s", ErrUnknownMarket, marketSymbol)
	}
	side := order.NormalizeSide(req.Side)
	if err := side.Validate(); err != nil {
		return order.Order{}, fmt.Errorf("%w: %s", ErrInvalidOrder, err)
	}
	orderType := order.NormalizeType(req.Type)
	if err := orderType.Validate(); err != nil {
		return order.Order{}, fmt.Errorf("%w: %s", ErrInvalidOrder, err)
	}
	if strings.TrimSpace(req.UserID) == "" {
		return order.Order{}, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	clientOrderID := strings.TrimSpace(req.ClientOrderID)
	if clientOrderID == "" {
		return order.Order{}, fmt.Errorf("%w: client_order_id is required", ErrInvalidOrder)
	}
	price, err := parsePositiveDecimal(req.Price, "price")
	if err != nil {
		return order.Order{}, err
	}
	quantity, err := parsePositiveDecimal(req.Quantity, "quantity")
	if err != nil {
		return order.Order{}, err
	}
	tif := normalizeOrderTimeInForce(orderType, req.TimeInForce)
	if err := tif.Validate(); err != nil {
		return order.Order{}, fmt.Errorf("%w: %s", ErrInvalidOrder, err)
	}
	if orderType == order.TypeMarket && tif != order.TimeInForceIOC {
		return order.Order{}, fmt.Errorf("%w: market orders must use ioc time_in_force", ErrInvalidOrder)
	}
	if orderType != order.TypeMarket && tif == order.TimeInForceIOC && orderType != order.TypeLimit {
		return order.Order{}, fmt.Errorf("%w: only limit and market orders can use ioc time_in_force", ErrInvalidOrder)
	}
	stopPrice := "0"
	status := order.StatusOpen
	if orderType == order.TypeStopLimit {
		parsedStop, err := parsePositiveDecimal(req.StopPrice, "stop_price")
		if err != nil {
			return order.Order{}, err
		}
		stopPrice = decimal.String(parsedStop)
		status = order.StatusPendingStop
	}

	now := time.Now()
	return order.Order{
		ID:                order.ID(idgen.New("ord")),
		ClientOrderID:     order.ClientOrderID(clientOrderID),
		UserID:            strings.TrimSpace(req.UserID),
		Market:            marketSymbol,
		BaseAsset:         m.BaseAsset,
		QuoteAsset:        m.QuoteAsset,
		Side:              side,
		Type:              orderType,
		Status:            status,
		TimeInForce:       tif,
		Price:             decimal.String(price),
		StopPrice:         stopPrice,
		Quantity:          decimal.String(quantity),
		FilledQuantity:    "0",
		RemainingQuantity: decimal.String(quantity),
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (s *Service) match(ctx context.Context, tx *postgres.ExchangeRepository, taker order.Order, levels *levelTracker) (order.Order, []trade.Trade, error) {
	makerSide := oppositeSide(taker.Side)
	trades := make([]trade.Trade, 0)
	levels.touch(taker)
	for decimal.Cmp(taker.RemainingQuantity, "0") > 0 {
		candidates, err := tx.ListMatchCandidates(ctx, taker.Market, makerSide, taker.Price, taker.ID, taker.UserID, 500)
		if err != nil {
			return taker, nil, err
		}
		if len(candidates) == 0 {
			break
		}

		matchedAny := false
		result, err := matching.MatchLimit(taker, candidates, func() trade.ID {
			return trade.ID(idgen.New("trd"))
		}, time.Now())
		if err != nil {
			return taker, nil, err
		}
		for _, maker := range result.Makers {
			levels.touch(maker)
			if err := tx.SaveOrder(ctx, maker); err != nil {
				return taker, nil, err
			}
		}
		if len(result.Trades) > 0 {
			trades = append(trades, result.Trades...)
			taker = result.Taker
			levels.touch(taker)
			matchedAny = true
		}
		if !matchedAny {
			break
		}
	}
	if decimal.Cmp(taker.RemainingQuantity, "0") > 0 && isImmediateOnly(taker) {
		taker.Status = order.StatusExpired
	} else if decimal.Cmp(taker.RemainingQuantity, "0") > 0 && taker.Status != order.StatusPartiallyFilled {
		taker.Status = order.StatusOpen
	}
	levels.touch(taker)
	return taker, trades, nil
}

func (s *Service) activateTriggeredStops(ctx context.Context, tx *postgres.ExchangeRepository, marketSymbol string, lastPrice string, levels *levelTracker) ([]MatchResult, error) {
	if lastPrice == "" {
		return nil, nil
	}
	pending, err := tx.ListPendingStops(ctx, marketSymbol, 500)
	if err != nil {
		return nil, err
	}
	results := make([]MatchResult, 0)
	for _, item := range pending {
		if !stopTriggered(item, lastPrice) {
			continue
		}
		if err := s.validatePriceBand(ctx, tx, item); err != nil {
			return nil, err
		}
		item.Status = order.StatusOpen
		item.UpdatedAt = time.Now()
		updated, trades, err := s.match(ctx, tx, item, levels)
		if err != nil {
			return nil, err
		}
		if err := tx.SaveOrder(ctx, updated); err != nil {
			return nil, err
		}
		if err := tx.SettleTrades(ctx, trades); err != nil {
			return nil, err
		}
		if err := tx.CreateTrades(ctx, trades); err != nil {
			return nil, err
		}
		if err := tx.CreateOrderEvents(ctx, terminalEvents(updated, trades)); err != nil {
			return nil, err
		}
		results = append(results, MatchResult{Order: updated, Trades: trades})
	}
	return results, nil
}

func stopTriggered(item order.Order, lastPrice string) bool {
	switch item.Side {
	case order.SideBuy:
		return decimal.Cmp(lastPrice, item.StopPrice) >= 0
	case order.SideSell:
		return decimal.Cmp(lastPrice, item.StopPrice) <= 0
	default:
		return false
	}
}

func lastTradePrice(trades []trade.Trade) string {
	if len(trades) == 0 {
		return ""
	}
	return trades[len(trades)-1].Price
}

func cancelable(status order.Status) bool {
	switch status {
	case order.StatusOpen, order.StatusPartiallyFilled, order.StatusPendingStop:
		return true
	default:
		return false
	}
}

func oppositeSide(side order.Side) order.Side {
	if side == order.SideBuy {
		return order.SideSell
	}
	return order.SideBuy
}

func knownStatus(status order.Status) bool {
	switch status {
	case order.StatusPendingStop, order.StatusOpen, order.StatusPartiallyFilled, order.StatusFilled, order.StatusCanceled, order.StatusExpired, order.StatusRejected:
		return true
	default:
		return false
	}
}

func normalizeOrderTimeInForce(orderType order.Type, value string) order.TimeInForce {
	if strings.TrimSpace(value) == "" && orderType == order.TypeMarket {
		return order.TimeInForceIOC
	}
	return order.NormalizeTimeInForce(value)
}

func isImmediateOnly(item order.Order) bool {
	return item.Type == order.TypeMarket || item.TimeInForce == order.TimeInForceIOC
}

func shouldReleaseUnfilled(item order.Order) bool {
	return isImmediateOnly(item) && item.Status == order.StatusExpired && decimal.Cmp(item.RemainingQuantity, "0") > 0
}

func (s *Service) requiredMarket(value string) (string, error) {
	marketSymbol := normalizeMarket(value)
	if _, ok := s.markets.Get(marketSymbol); !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownMarket, marketSymbol)
	}
	return marketSymbol, nil
}

func (s *Service) optionalMarket(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return s.requiredMarket(value)
}

func (s *Service) ListBalances(ctx context.Context, userID string) ([]balance.Balance, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	return s.repo.ListBalances(ctx, userID)
}

func (s *Service) MarkDepositPending(ctx context.Context, userID string, req BalanceAmountRequest) (*balance.Balance, error) {
	out, err := s.applyBalanceAmount(ctx, userID, req, func(tx *postgres.ExchangeRepository, userID string, asset string, amount string) (*balance.Balance, error) {
		return tx.MarkDepositPending(ctx, userID, asset, amount, balance.EventID(idgen.New("bev")))
	})
	if err != nil {
		return nil, err
	}
	s.publishBalanceUpdate("exchange.deposit_pending", out)
	return out, nil
}

func (s *Service) SettleDeposit(ctx context.Context, userID string, req BalanceAmountRequest) (*balance.Balance, error) {
	out, err := s.applyBalanceAmount(ctx, userID, req, func(tx *postgres.ExchangeRepository, userID string, asset string, amount string) (*balance.Balance, error) {
		return tx.SettleDeposit(ctx, userID, asset, amount, balance.EventID(idgen.New("bev")))
	})
	if err != nil {
		return nil, err
	}
	s.publishBalanceUpdate("exchange.deposit_settled", out)
	return out, nil
}

func (s *Service) RequestWithdrawal(ctx context.Context, userID string, req WithdrawalRequest) (*balance.Withdrawal, error) {
	userID = strings.TrimSpace(userID)
	asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	chainKey := strings.ToLower(strings.TrimSpace(req.ChainKey))
	address := strings.TrimSpace(req.Address)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	if asset == "" {
		return nil, fmt.Errorf("%w: asset is required", ErrInvalidOrder)
	}
	if !s.knownAsset(asset) {
		return nil, fmt.Errorf("%w: unknown asset %s", ErrInvalidOrder, asset)
	}
	if chainKey == "" {
		return nil, fmt.Errorf("%w: chain_key is required", ErrInvalidOrder)
	}
	if !s.knownChain(chainKey) {
		return nil, fmt.Errorf("%w: unknown chain_key %s", ErrInvalidOrder, chainKey)
	}
	if address == "" {
		return nil, fmt.Errorf("%w: address is required", ErrInvalidOrder)
	}
	if err := validateGatewayAddress(chainKey, address); err != nil {
		return nil, err
	}
	amount, err := parsePositiveDecimal(req.Amount, "amount")
	if err != nil {
		return nil, err
	}
	item := balance.Withdrawal{
		ID:       balance.WithdrawalID(idgen.New("wd")),
		UserID:   userID,
		Asset:    asset,
		Amount:   decimal.String(amount),
		ChainKey: chainKey,
		Address:  address,
	}
	var out *balance.Withdrawal
	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		created, err := tx.RequestWithdrawal(ctx, item, balance.EventID(idgen.New("bev")))
		if err != nil {
			return err
		}
		out = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publishWithdrawalUpdate("exchange.withdrawal_requested", out)
	return out, nil
}

func (s *Service) CompleteWithdrawal(ctx context.Context, id balance.WithdrawalID) (*balance.Withdrawal, error) {
	out, err := s.finalizeWithdrawal(ctx, id, func(tx *postgres.ExchangeRepository) (*balance.Withdrawal, error) {
		return tx.CompleteWithdrawal(ctx, id, balance.EventID(idgen.New("bev")))
	})
	if err != nil {
		return nil, err
	}
	s.publishWithdrawalUpdate("exchange.withdrawal_completed", out)
	return out, nil
}

func (s *Service) CancelWithdrawal(ctx context.Context, id balance.WithdrawalID) (*balance.Withdrawal, error) {
	out, err := s.finalizeWithdrawal(ctx, id, func(tx *postgres.ExchangeRepository) (*balance.Withdrawal, error) {
		return tx.CancelWithdrawal(ctx, id, balance.EventID(idgen.New("bev")))
	})
	if err != nil {
		return nil, err
	}
	s.publishWithdrawalUpdate("exchange.withdrawal_canceled", out)
	return out, nil
}

func (s *Service) ListWithdrawals(ctx context.Context, userID string, limit int) ([]balance.Withdrawal, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	return s.repo.ListWithdrawals(ctx, userID, limit)
}

func (s *Service) finalizeWithdrawal(ctx context.Context, id balance.WithdrawalID, fn func(*postgres.ExchangeRepository) (*balance.Withdrawal, error)) (*balance.Withdrawal, error) {
	if strings.TrimSpace(string(id)) == "" {
		return nil, fmt.Errorf("%w: withdrawal id is required", ErrInvalidOrder)
	}
	var out *balance.Withdrawal
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		updated, err := fn(tx)
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) applyBalanceAmount(ctx context.Context, userID string, req BalanceAmountRequest, fn func(*postgres.ExchangeRepository, string, string, string) (*balance.Balance, error)) (*balance.Balance, error) {
	userID = strings.TrimSpace(userID)
	asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	if asset == "" {
		return nil, fmt.Errorf("%w: asset is required", ErrInvalidOrder)
	}
	if !s.knownAsset(asset) {
		return nil, fmt.Errorf("%w: unknown asset %s", ErrInvalidOrder, asset)
	}
	amount, err := parsePositiveDecimal(req.Amount, "amount")
	if err != nil {
		return nil, err
	}
	var out *balance.Balance
	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		updated, err := fn(tx, userID, asset, decimal.String(amount))
		if err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) ListWallets(ctx context.Context, userID string) ([]balance.Wallet, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	return s.repo.ListWallets(ctx, userID)
}

func (s *Service) RegisterGatewayWallet(ctx context.Context, userID string, req WalletRequest) (*balance.Wallet, error) {
	userID = strings.TrimSpace(userID)
	chainKey := strings.ToLower(strings.TrimSpace(req.ChainKey))
	address := strings.TrimSpace(req.Address)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	if chainKey == "" {
		return nil, fmt.Errorf("%w: chain_key is required", ErrInvalidOrder)
	}
	if !s.knownChain(chainKey) {
		return nil, fmt.Errorf("%w: unknown chain_key %s", ErrInvalidOrder, chainKey)
	}
	if address == "" {
		return nil, fmt.Errorf("%w: address is required", ErrInvalidOrder)
	}
	if err := validateGatewayAddress(chainKey, address); err != nil {
		return nil, err
	}
	var out *balance.Wallet
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		wallet, err := tx.UpsertWallet(ctx, balance.Wallet{UserID: userID, ChainKey: chainKey, Address: address})
		if err != nil {
			return err
		}
		out = wallet
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publishWalletUpdate("exchange.wallet_registered", out)
	return out, nil
}

func normalizeMarket(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func (s *Service) knownAsset(asset string) bool {
	markets := s.markets.All()
	if len(markets) == 0 {
		return true
	}
	for _, item := range markets {
		if strings.EqualFold(item.BaseAsset, asset) || strings.EqualFold(item.QuoteAsset, asset) {
			return true
		}
	}
	return false
}

func (s *Service) knownChain(chainKey string) bool {
	markets := s.markets.All()
	if len(markets) == 0 {
		return true
	}
	for _, item := range markets {
		for _, key := range item.ChainKeys {
			if strings.EqualFold(key, chainKey) {
				return true
			}
		}
	}
	return false
}

func validateGatewayAddress(chainKey string, address string) error {
	chainKey = strings.ToLower(strings.TrimSpace(chainKey))
	address = strings.TrimSpace(address)
	switch chainKey {
	case "ethereum", "base", "chiliz", "avalanche", "avax", "unichain":
		if !strings.HasPrefix(address, "0x") || len(address) != 42 {
			return fmt.Errorf("%w: address must be a 20-byte EVM address", ErrInvalidOrder)
		}
	case "solana":
		if strings.HasPrefix(address, "0x") || len(address) < 32 || len(address) > 64 {
			return fmt.Errorf("%w: address must be a Solana base58 address", ErrInvalidOrder)
		}
	default:
		if len(address) < 8 {
			return fmt.Errorf("%w: address is too short", ErrInvalidOrder)
		}
	}
	return nil
}

func parsePositiveDecimal(value string, field string) (*decimalRat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("%w: %s is required", ErrInvalidOrder, field)
	}
	if decimal.FractionalDigits(value) > decimal.Scale {
		return nil, fmt.Errorf("%w: %s supports at most %d decimal places", ErrInvalidOrder, field, decimal.Scale)
	}
	out := decimal.Parse(value)
	if out.Sign() <= 0 {
		return nil, fmt.Errorf("%w: %s must be a positive decimal", ErrInvalidOrder, field)
	}
	if decimal.String(out) == "0" {
		return nil, fmt.Errorf("%w: %s is below supported precision", ErrInvalidOrder, field)
	}
	return out, nil
}

type decimalRat = big.Rat

type levelTracker struct {
	byKey map[string]postgres.PriceLevelKey
}

func newLevelTracker() *levelTracker {
	return &levelTracker{byKey: make(map[string]postgres.PriceLevelKey)}
}

func (t *levelTracker) touch(item order.Order) {
	if t == nil || item.Market == "" || item.Side == "" || item.Price == "" {
		return
	}
	key := postgres.PriceLevelKey{Market: item.Market, Side: item.Side, Price: item.Price}
	t.byKey[key.Market+"|"+string(key.Side)+"|"+key.Price] = key
}

func (t *levelTracker) keys() []postgres.PriceLevelKey {
	if t == nil || len(t.byKey) == 0 {
		return nil
	}
	out := make([]postgres.PriceLevelKey, 0, len(t.byKey))
	for _, key := range t.byKey {
		out = append(out, key)
	}
	return out
}

func newOrderEvent(item order.Order, eventType order.EventType, refID string) order.Event {
	return order.Event{
		ID:        order.EventID(idgen.New("evt")),
		OrderID:   item.ID,
		UserID:    item.UserID,
		Market:    item.Market,
		Type:      eventType,
		RefID:     refID,
		CreatedAt: time.Now(),
	}
}

func eventsForTrades(items []trade.Trade) []order.Event {
	if len(items) == 0 {
		return nil
	}
	events := make([]order.Event, 0, len(items)*3)
	for _, item := range items {
		refID := string(item.ID)
		events = append(events, order.Event{
			ID:        order.EventID(idgen.New("evt")),
			OrderID:   item.MakerOrderID,
			UserID:    item.MakerUserID,
			Market:    item.Market,
			Type:      order.EventTradeCreated,
			RefID:     refID,
			CreatedAt: item.CreatedAt,
		})
		events = append(events, order.Event{
			ID:        order.EventID(idgen.New("evt")),
			OrderID:   item.TakerOrderID,
			UserID:    item.TakerUserID,
			Market:    item.Market,
			Type:      order.EventTradeCreated,
			RefID:     refID,
			CreatedAt: item.CreatedAt,
		})
	}
	return events
}

func terminalEvents(item order.Order, trades []trade.Trade) []order.Event {
	events := eventsForTrades(trades)
	switch item.Status {
	case order.StatusFilled:
		events = append(events, newOrderEvent(item, order.EventOrderFilled, ""))
	case order.StatusExpired:
		events = append(events, newOrderEvent(item, order.EventOrderExpired, ""))
	}
	return events
}

type socketEvent struct {
	Type       string              `json:"type"`
	Market     string              `json:"market,omitempty"`
	UserID     string              `json:"user_id,omitempty"`
	Order      *order.Order        `json:"order,omitempty"`
	Trades     []trade.Trade       `json:"trades,omitempty"`
	Balance    *balance.Balance    `json:"balance,omitempty"`
	Withdrawal *balance.Withdrawal `json:"withdrawal,omitempty"`
	Wallet     *balance.Wallet     `json:"wallet,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
}

func (s *Service) publishOrderUpdate(eventType string, item order.Order, trades []trade.Trade) {
	if s.publish == nil {
		return
	}
	switch item.Status {
	case order.StatusFilled:
		eventType = "exchange.order_filled"
	case order.StatusExpired:
		eventType = "exchange.order_expired"
	}
	s.publishJSON(socketEvent{
		Type:      eventType,
		Market:    item.Market,
		UserID:    item.UserID,
		Order:     &item,
		Trades:    trades,
		CreatedAt: time.Now(),
	})
	if len(trades) > 0 {
		s.publishJSON(socketEvent{
			Type:      "exchange.trades_created",
			Market:    item.Market,
			UserID:    item.UserID,
			Trades:    trades,
			CreatedAt: time.Now(),
		})
	}
	s.publishJSON(socketEvent{
		Type:      "exchange.orderbook_updated",
		Market:    item.Market,
		CreatedAt: time.Now(),
	})
}

func (s *Service) publishBalanceUpdate(eventType string, item *balance.Balance) {
	if s.publish == nil || item == nil {
		return
	}
	s.publishJSON(socketEvent{
		Type:      eventType,
		UserID:    item.UserID,
		Balance:   item,
		CreatedAt: time.Now(),
	})
}

func (s *Service) publishWithdrawalUpdate(eventType string, item *balance.Withdrawal) {
	if s.publish == nil || item == nil {
		return
	}
	s.publishJSON(socketEvent{
		Type:       eventType,
		UserID:     item.UserID,
		Withdrawal: item,
		CreatedAt:  time.Now(),
	})
}

func (s *Service) publishWalletUpdate(eventType string, item *balance.Wallet) {
	if s.publish == nil || item == nil {
		return
	}
	s.publishJSON(socketEvent{
		Type:      eventType,
		UserID:    item.UserID,
		Wallet:    item,
		CreatedAt: time.Now(),
	})
}

func (s *Service) publishJSON(event socketEvent) {
	payload, err := json.Marshal(event)
	if err != nil || len(payload) == 0 {
		return
	}
	s.publish(payload)
}

func (s *Service) marketLiquidity(ctx context.Context, marketSymbol string) (string, error) {
	bids, err := s.repo.ListPriceLevels(ctx, marketSymbol, order.SideBuy, 100)
	if err != nil {
		return "0", err
	}
	asks, err := s.repo.ListPriceLevels(ctx, marketSymbol, order.SideSell, 100)
	if err != nil {
		return "0", err
	}
	total := "0"
	for _, level := range bids {
		total = decimal.Add(total, decimal.Mul(level.Price, level.Quantity))
	}
	for _, level := range asks {
		total = decimal.Add(total, decimal.Mul(level.Price, level.Quantity))
	}
	return total, nil
}

func applyCandleStats(summary MarketSummary, candles []trade.Candle) MarketSummary {
	first := candles[0]
	last := candles[len(candles)-1]
	high := first.High
	low := first.Low
	volume := "0"
	for _, item := range candles {
		if decimal.Cmp(item.High, high) > 0 {
			high = item.High
		}
		if decimal.Cmp(item.Low, low) < 0 {
			low = item.Low
		}
		volume = decimal.Add(volume, item.VolumeBase)
	}

	summary.LastPrice = last.Close
	summary.High24h = high
	summary.Low24h = low
	summary.Volume24h = volume
	summary.Change24h = percentChange(first.Open, last.Close)
	return summary
}

func percentChange(open string, close string) string {
	openRat := decimal.Parse(open)
	if openRat.Sign() <= 0 {
		return "0"
	}
	diff := new(big.Rat).Sub(decimal.Parse(close), openRat)
	pct := new(big.Rat).Mul(diff, big.NewRat(100, 1))
	pct.Quo(pct, openRat)
	return decimal.String(pct)
}

func defaultMarketPrice(baseAsset string) string {
	switch strings.ToUpper(strings.TrimSpace(baseAsset)) {
	case "PEPPER":
		return "0.000000001"
	case "CHZ":
		return "0.08"
	case "SOL":
		return "184.25"
	case "ETH":
		return "3412.8"
	case "AVAX":
		return "32.4"
	case "USDC":
		return "1"
	default:
		return "1"
	}
}

func (s *Service) validatePriceBand(ctx context.Context, tx *postgres.ExchangeRepository, item order.Order) error {
	if s.priceBandBps <= 0 {
		return nil
	}
	last, err := tx.LastTrade(ctx, item.Market)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	reference := decimal.Parse(last.Price)
	limit := decimal.Parse(item.Price)
	band := new(big.Rat).SetFrac(big.NewInt(s.priceBandBps), big.NewInt(10000))
	min := new(big.Rat).Mul(reference, new(big.Rat).Sub(big.NewRat(1, 1), band))
	max := new(big.Rat).Mul(reference, new(big.Rat).Add(big.NewRat(1, 1), band))
	if limit.Cmp(min) < 0 || limit.Cmp(max) > 0 {
		return fmt.Errorf("%w: price %s is outside %s-%s", ErrPriceBandExceeded, item.Price, decimal.String(min), decimal.String(max))
	}
	return nil
}

func defaultPriceBandBps() int64 {
	raw := strings.TrimSpace(os.Getenv("EXCHANGE_PRICE_BAND_BPS"))
	if raw == "" {
		return 2000
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 2000
	}
	return value
}
