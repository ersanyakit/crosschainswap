package orders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/core/asset"
	"exchange/internal/core/balance"
	"exchange/internal/core/chain"
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

const (
	UpdatesChannel         = "exchange_updates"
	minPositiveMarketPrice = "0.000000000000000001"
)

type Service struct {
	markets      market.Registry
	assets       asset.Registry
	hasAssets    bool
	repo         *postgres.ExchangeRepository
	priceBandBps int64
	publish      Publisher
	gateway      GatewayWalletProvider
}

type Publisher func([]byte)

type GatewayWalletProvider interface {
	CreateStaticAddress(ctx context.Context, userID string, symbol string, chainID int64, label string) (*GatewayStaticAddress, error)
	QRCode(ctx context.Context, address string, size int) ([]byte, error)
}

type GatewayStaticAddress struct {
	WalletID string
	UserID   string
	Symbol   string
	Chain    string
	Address  string
	Label    string
}

type PlaceRequest struct {
	CommandID     string `json:"command_id"`
	ClientOrderID string `json:"client_order_id"`
	ReservationID string `json:"reservation_id"`
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

type DepositAddressRequest struct {
	Asset    string `json:"asset"`
	ChainKey string `json:"chain_key"`
	Label    string `json:"label"`
}

type DepositAddress struct {
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	ChainKey string `json:"chain_key"`
	ChainID  int64  `json:"chain_id"`
	Address  string `json:"address"`
	WalletID string `json:"wallet_id,omitempty"`
	Label    string `json:"label,omitempty"`
}

type GatewayDepositCallback struct {
	EventID       string `json:"event_id"`
	PaymentID     string `json:"payment_id"`
	TrackID       string `json:"track_id"`
	OrderID       string `json:"order_id"`
	UserID        string `json:"user_id"`
	Asset         string `json:"asset"`
	Symbol        string `json:"symbol"`
	SelectedAsset string `json:"selected_asset"`
	Amount        string `json:"amount"`
	AmountRaw     string `json:"amount_raw"`
	Decimals      int    `json:"decimals"`
	Status        string `json:"status"`
	ChainKey      string `json:"chain_key"`
	Chain         string `json:"chain"`
	SelectedChain string `json:"selected_chain"`
	TxHash        string `json:"tx_hash"`
}

type GatewayWithdrawalCallback struct {
	EventID      string `json:"event_id"`
	WithdrawalID string `json:"withdrawal_id"`
	PayoutID     string `json:"payout_id"`
	ID           string `json:"id"`
	Status       string `json:"status"`
	TxHash       string `json:"tx_hash"`
}

type GatewayCallbackResult struct {
	Status     string              `json:"status"`
	Action     string              `json:"action"`
	Balance    *balance.Balance    `json:"balance,omitempty"`
	Withdrawal *balance.Withdrawal `json:"withdrawal,omitempty"`
}

type MatchResult struct {
	Order  order.Order   `json:"order"`
	Trades []trade.Trade `json:"trades"`
}

type bookDelta struct {
	Market  string
	Version uint64
	Levels  []orderbook.PriceLevel
}

type matchEventPayload struct {
	Taker  order.Order            `json:"taker"`
	Makers []order.Order          `json:"makers"`
	Trades []trade.Trade          `json:"trades"`
	Levels []orderbook.PriceLevel `json:"levels,omitempty"`
}

type BookMatchPersistence struct {
	Result     MatchResult
	ReloadBook bool
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

func (s *Service) SetAssetRegistry(registry asset.Registry) {
	s.assets = registry
	s.hasAssets = true
}

func (s *Service) SetGatewayWalletProvider(provider GatewayWalletProvider) {
	s.gateway = provider
}

func (s *Service) Place(ctx context.Context, req PlaceRequest) (*MatchResult, error) {
	item, err := s.buildOrder(req)
	if err != nil {
		return nil, err
	}
	commandID := orderCommandID(req, item)
	commandPayload, commandPayloadHash, err := orderCommandPayload(item)
	if err != nil {
		return nil, err
	}

	result := &MatchResult{}
	publishResults := make([]MatchResult, 0, 1)
	bookDeltas := make([]bookDelta, 0, 1)
	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		commandExists := false
		if commandID != "" {
			existingCommand, err := tx.GetOrderCommandForUpdate(ctx, commandID)
			if err == nil {
				commandExists = true
				if existingCommand.PayloadHash != commandPayloadHash {
					return fmt.Errorf("%w: command_id payload conflict", ErrInvalidOrder)
				}
				if strings.TrimSpace(existingCommand.OrderID) != "" {
					existing, err := tx.GetOrder(ctx, order.ID(existingCommand.OrderID))
					if err == nil {
						result.Order = *existing
						return nil
					}
					if !errors.Is(err, gorm.ErrRecordNotFound) {
						return err
					}
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if item.ClientOrderID != "" {
			existing, err := tx.FindOrderByClientID(ctx, item.UserID, item.ClientOrderID)
			if err == nil {
				if !sameOrderCommandPayload(*existing, item) {
					return fmt.Errorf("%w: client_order_id payload conflict", ErrInvalidOrder)
				}
				if commandExists {
					if err := tx.CompleteOrderCommand(ctx, commandID, string(existing.ID), "duplicate"); err != nil {
						return err
					}
					if err := tx.ApplyOrderCommandLog(ctx, commandID, string(existing.ID)); err != nil {
						return err
					}
				}
				result.Order = *existing
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if !commandExists {
			if err := tx.CreateOrderCommand(ctx, postgres.OrderCommand{
				ID:            commandID,
				ClientOrderID: string(item.ClientOrderID),
				UserID:        item.UserID,
				Market:        item.Market,
				Type:          postgres.OrderCommandTypeNewOrder,
				PayloadHash:   commandPayloadHash,
				Payload:       commandPayload,
				Status:        postgres.OrderCommandStatusPending,
			}); err != nil {
				return err
			}
		}
		if _, err := tx.AppendOrderCommandLog(ctx, postgres.OrderCommandLog{
			CommandID:   commandID,
			Market:      item.Market,
			Type:        postgres.OrderCommandTypeNewOrder,
			Key:         item.Market,
			PayloadHash: commandPayloadHash,
			Payload:     commandPayload,
			Status:      postgres.OrderCommandLogStatusPending,
		}); err != nil {
			return err
		}
		if err := s.validatePriceBand(ctx, tx, item); err != nil {
			return err
		}
		if err := s.validateStopPlacement(ctx, tx, item); err != nil {
			return err
		}
		seq, err := tx.NextOrderSequence(ctx, item.Market)
		if err != nil {
			return err
		}
		item.SequenceID = seq
		if err := tx.CreateOrder(ctx, item); err != nil {
			return err
		}
		if err := tx.ReserveOrderFunds(ctx, item, balance.EventID(idgen.New("bev"))); err != nil {
			return err
		}
		if err := tx.CreateOrderEvents(ctx, []order.Event{newOrderEvent(item, order.EventOrderAccepted, "")}); err != nil {
			return err
		}
		if item.Status != order.StatusPendingStop && asyncMatchingEnabled() {
			item.Status = order.StatusPendingMatch
			item.UpdatedAt = time.Now()
			if err := tx.SaveOrder(ctx, item); err != nil {
				return err
			}
			if !commandLogMatchingEnabled() {
				if err := tx.CreateMatchJob(ctx, item.ID, item.Market); err != nil {
					return err
				}
			}
			if err := tx.CompleteOrderCommand(ctx, commandID, string(item.ID), string(item.Status)); err != nil {
				return err
			}
			if !commandLogMatchingEnabled() {
				if err := tx.ApplyOrderCommandLog(ctx, commandID, string(item.ID)); err != nil {
					return err
				}
			}
			result.Order = item
			publishResults = append(publishResults, MatchResult{Order: item})
			return nil
		}
		if item.Status == order.StatusPendingStop {
			if err := tx.CompleteOrderCommand(ctx, commandID, string(item.ID), string(item.Status)); err != nil {
				return err
			}
			if err := tx.ApplyOrderCommandLog(ctx, commandID, string(item.ID)); err != nil {
				return err
			}
			result.Order = item
			publishResults = append(publishResults, MatchResult{Order: item})
			return nil
		}

		levels := newLevelTracker()
		updated, makers, trades, err := s.match(ctx, tx, item, levels)
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
		if err := tx.CreateOrderEvents(ctx, matchEvents(updated, makers, trades)); err != nil {
			return err
		}
		activated, err := s.activateTriggeredStops(ctx, tx, updated.Market, lastTradePrice(trades), levels)
		if err != nil {
			return err
		}
		deltas, err := refreshBookProjection(ctx, tx, updated.Market, levels)
		if err != nil {
			return err
		}
		bookDeltas = append(bookDeltas, newBookDelta(updated.Market, updated.SequenceID, deltas))
		if err := tx.CompleteOrderCommand(ctx, commandID, string(updated.ID), string(updated.Status)); err != nil {
			return err
		}
		if err := tx.ApplyOrderCommandLog(ctx, commandID, string(updated.ID)); err != nil {
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
	for _, delta := range bookDeltas {
		s.publishOrderBookDelta(delta)
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
	var delta bookDelta
	mutated := false
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		item, err := tx.GetOrderForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrOrderNotFound, id)
		}
		if strings.TrimSpace(req.UserID) != "" && item.UserID != req.UserID {
			return fmt.Errorf("%w: user mismatch", ErrInvalidOrder)
		}
		if !cancelable(item.Status) {
			if cancelAlreadyTerminal(item.Status) {
				canceled = *item
				return nil
			}
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
		deltas, err := refreshBookProjection(ctx, tx, item.Market, levels)
		if err != nil {
			return err
		}
		delta = newBookDelta(item.Market, item.SequenceID, deltas)
		canceled = *item
		mutated = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if mutated {
		s.publishOrderUpdate("exchange.order_canceled", canceled, nil)
		s.publishOrderBookDelta(delta)
	}
	return &canceled, nil
}

func (s *Service) Book(ctx context.Context, marketSymbol string, depth int) (*orderbook.Snapshot, error) {
	marketSymbol = normalizeMarket(marketSymbol)
	if _, ok := s.markets.Get(marketSymbol); !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownMarket, marketSymbol)
	}
	if s.repo == nil {
		return &orderbook.Snapshot{Market: marketSymbol, Bids: nil, Asks: nil}, nil
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
	marketDef, _ := s.markets.Get(marketSymbol)
	interval := strings.ToLower(strings.TrimSpace(req.Interval))
	if interval == "" {
		interval = "1m"
	}
	intervalDef, ok := trade.IntervalByKey(interval)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported candle interval", ErrInvalidOrder)
	}
	if s.repo == nil {
		return fallbackCandles(marketSymbol, intervalDef, req.Limit, defaultMarketPrice(marketDef.BaseAsset)), nil
	}
	candles, err := s.repo.ListCandles(ctx, marketSymbol, interval, req.Limit)
	if err != nil {
		return nil, err
	}
	return candles, nil
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
	bookDeltas := make([]bookDelta, 0, 1)
	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		levels := newLevelTracker()
		activated, err := s.activateTriggeredStops(ctx, tx, marketSymbol, decimal.String(lastPrice), levels)
		if err != nil {
			return err
		}
		results = activated
		deltas, err := refreshBookProjection(ctx, tx, marketSymbol, levels)
		if err != nil {
			return err
		}
		bookDeltas = append(bookDeltas, newBookDelta(marketSymbol, maxResultSequence(results), deltas))
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		s.publishOrderUpdate("exchange.order_updated", result.Order, result.Trades)
	}
	for _, delta := range bookDeltas {
		s.publishOrderBookDelta(delta)
	}
	return results, nil
}

func (s *Service) MatchOrder(ctx context.Context, id order.ID) (*MatchResult, error) {
	var result MatchResult
	publishResults := make([]MatchResult, 0, 1)
	bookDeltas := make([]bookDelta, 0, 1)
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		item, err := tx.GetOrderForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrOrderNotFound, id)
		}
		switch item.Status {
		case order.StatusPendingMatch:
			item.Status = order.StatusOpen
			item.UpdatedAt = time.Now()
		case order.StatusOpen, order.StatusPartiallyFilled:
		default:
			result.Order = *item
			return nil
		}

		levels := newLevelTracker()
		updated, makers, trades, err := s.match(ctx, tx, *item, levels)
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
		if err := tx.CreateOrderEvents(ctx, matchEvents(updated, makers, trades)); err != nil {
			return err
		}
		activated, err := s.activateTriggeredStops(ctx, tx, updated.Market, lastTradePrice(trades), levels)
		if err != nil {
			return err
		}
		deltas, err := refreshBookProjection(ctx, tx, updated.Market, levels)
		if err != nil {
			return err
		}
		bookDeltas = append(bookDeltas, newBookDelta(updated.Market, updated.SequenceID, deltas))
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
		s.publishOrderUpdate("exchange.order_updated", publishResult.Order, publishResult.Trades)
	}
	for _, delta := range bookDeltas {
		s.publishOrderBookDelta(delta)
	}
	return &result, nil
}

func (s *Service) MatchOrderWithBook(ctx context.Context, id order.ID) (*MatchResult, error) {
	return s.MatchOrderWithBookAtSequence(ctx, id, 0, false)
}

func (s *Service) MatchOrderWithBookAtSequence(ctx context.Context, id order.ID, sequence uint64, saveSnapshot bool) (*MatchResult, error) {
	var result MatchResult
	publishResults := make([]MatchResult, 0, 1)
	bookDeltas := make([]bookDelta, 0, 1)
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		item, err := tx.GetOrderForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrOrderNotFound, id)
		}
		switch item.Status {
		case order.StatusPendingMatch:
			item.Status = order.StatusOpen
			item.UpdatedAt = time.Now()
		case order.StatusOpen, order.StatusPartiallyFilled:
		default:
			result.Order = *item
			return nil
		}

		levels := newLevelTracker()
		updated, makers, trades, err := s.matchWithBook(ctx, tx, *item, levels)
		if err != nil {
			return err
		}
		for _, maker := range makers {
			if err := tx.SaveOrder(ctx, maker); err != nil {
				return err
			}
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
		if err := tx.CreateOrderEvents(ctx, matchEvents(updated, makers, trades)); err != nil {
			return err
		}
		activated, err := s.activateTriggeredStops(ctx, tx, updated.Market, lastTradePrice(trades), levels)
		if err != nil {
			return err
		}
		deltas, err := refreshBookProjection(ctx, tx, updated.Market, levels)
		if err != nil {
			return err
		}
		if err := appendMatchEventLog(ctx, tx, sequence, updated, makers, trades, deltas); err != nil {
			return err
		}
		bookDeltas = append(bookDeltas, newBookDelta(updated.Market, bookVersion(updated, sequence), deltas))
		if saveSnapshot {
			snapshot, err := s.captureMatcherSnapshot(ctx, tx, updated.Market, updated.BaseAsset, updated.QuoteAsset, sequence)
			if err != nil {
				return err
			}
			if err := tx.SaveMatcherSnapshot(ctx, snapshot); err != nil {
				return err
			}
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
		s.publishOrderUpdate("exchange.order_updated", publishResult.Order, publishResult.Trades)
	}
	for _, delta := range bookDeltas {
		s.publishOrderBookDelta(delta)
	}
	return &result, nil
}

func (s *Service) PersistBookMatchResult(ctx context.Context, matchResult matching.Result, sequence uint64, saveSnapshot bool) (*BookMatchPersistence, error) {
	out := &BookMatchPersistence{}
	publishResults := make([]MatchResult, 0, 1)
	bookDeltas := make([]bookDelta, 0, 1)
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		current, err := tx.GetOrderForUpdate(ctx, matchResult.Taker.ID)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrOrderNotFound, matchResult.Taker.ID)
		}
		switch current.Status {
		case order.StatusPendingMatch, order.StatusOpen, order.StatusPartiallyFilled:
		default:
			out.Result = MatchResult{Order: *current}
			return nil
		}

		levels := newLevelTracker()
		levels.touch(matchResult.Taker)
		for _, maker := range matchResult.Makers {
			if _, err := tx.GetOrderForUpdate(ctx, maker.ID); err != nil {
				return fmt.Errorf("%w: maker %s", ErrOrderNotFound, maker.ID)
			}
			levels.touch(maker)
			if err := tx.SaveOrder(ctx, maker); err != nil {
				return err
			}
		}
		if err := tx.SaveOrder(ctx, matchResult.Taker); err != nil {
			return err
		}
		if err := tx.SettleTrades(ctx, matchResult.Trades); err != nil {
			return err
		}
		if shouldReleaseUnfilled(matchResult.Taker) {
			if err := tx.ReleaseOrderFunds(ctx, matchResult.Taker, balance.EventID(idgen.New("bev"))); err != nil {
				return err
			}
		}
		if err := tx.CreateTrades(ctx, matchResult.Trades); err != nil {
			return err
		}
		if err := tx.CreateOrderEvents(ctx, matchEvents(matchResult.Taker, matchResult.Makers, matchResult.Trades)); err != nil {
			return err
		}
		activated, err := s.activateTriggeredStops(ctx, tx, matchResult.Taker.Market, lastTradePrice(matchResult.Trades), levels)
		if err != nil {
			return err
		}
		if len(activated) > 0 {
			out.ReloadBook = true
		}
		deltas, err := refreshBookProjection(ctx, tx, matchResult.Taker.Market, levels)
		if err != nil {
			return err
		}
		if err := appendMatchEventLog(ctx, tx, sequence, matchResult.Taker, matchResult.Makers, matchResult.Trades, deltas); err != nil {
			return err
		}
		bookDeltas = append(bookDeltas, newBookDelta(matchResult.Taker.Market, bookVersion(matchResult.Taker, sequence), deltas))
		if saveSnapshot {
			snapshot, err := s.captureMatcherSnapshot(ctx, tx, matchResult.Taker.Market, matchResult.Taker.BaseAsset, matchResult.Taker.QuoteAsset, sequence)
			if err != nil {
				return err
			}
			if err := tx.SaveMatcherSnapshot(ctx, snapshot); err != nil {
				return err
			}
		}
		out.Result = MatchResult{Order: matchResult.Taker, Trades: matchResult.Trades}
		publishResults = append(publishResults, out.Result)
		publishResults = append(publishResults, activated...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, publishResult := range publishResults {
		s.publishOrderUpdate("exchange.order_updated", publishResult.Order, publishResult.Trades)
	}
	for _, delta := range bookDeltas {
		s.publishOrderBookDelta(delta)
	}
	return out, nil
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
	priceString := decimal.String(price)
	quantityString := decimal.String(quantity)
	if decimal.Cmp(decimal.Mul(priceString, quantityString), "0") <= 0 {
		return order.Order{}, fmt.Errorf("%w: notional is below supported precision", ErrInvalidOrder)
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
		ReservationID:     orderReservationID(req, strings.TrimSpace(req.UserID), clientOrderID),
		UserID:            strings.TrimSpace(req.UserID),
		Market:            marketSymbol,
		BaseAsset:         m.BaseAsset,
		QuoteAsset:        m.QuoteAsset,
		Side:              side,
		Type:              orderType,
		Status:            status,
		TimeInForce:       tif,
		Price:             priceString,
		StopPrice:         stopPrice,
		Quantity:          quantityString,
		FilledQuantity:    "0",
		RemainingQuantity: quantityString,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (s *Service) match(ctx context.Context, tx *postgres.ExchangeRepository, taker order.Order, levels *levelTracker) (order.Order, []order.Order, []trade.Trade, error) {
	makerSide := oppositeSide(taker.Side)
	makers := make([]order.Order, 0)
	trades := make([]trade.Trade, 0)
	levels.touch(taker)
	for decimal.Cmp(taker.RemainingQuantity, "0") > 0 {
		matchTaker := effectiveMatchTaker(taker)
		candidates, err := tx.ListMatchCandidates(ctx, matchTaker.Market, makerSide, matchTaker.Price, matchTaker.ID, 500)
		if err != nil {
			return taker, nil, nil, err
		}
		if len(candidates) == 0 {
			break
		}

		matchedAny := false
		result, err := matching.MatchLimit(matchTaker, candidates, func() trade.ID {
			return trade.ID(idgen.New("trd"))
		}, time.Now())
		if err != nil {
			return taker, nil, nil, err
		}
		result.Taker.Price = taker.Price
		for _, maker := range result.Makers {
			levels.touch(maker)
			if err := tx.SaveOrder(ctx, maker); err != nil {
				return taker, nil, nil, err
			}
			makers = append(makers, maker)
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
	return taker, makers, trades, nil
}

func (s *Service) matchWithBook(ctx context.Context, tx *postgres.ExchangeRepository, taker order.Order, levels *levelTracker) (order.Order, []order.Order, []trade.Trade, error) {
	levels.touch(taker)
	active, err := tx.ListActiveOrdersForUpdate(ctx, taker.Market, taker.ID, 0)
	if err != nil {
		return taker, nil, nil, err
	}

	book := matching.NewMarketBook(taker.Market, taker.BaseAsset, taker.QuoteAsset)
	if err := book.Load(active); err != nil {
		return taker, nil, nil, err
	}
	result, err := book.Apply(taker, func() trade.ID {
		return trade.ID(idgen.New("trd"))
	}, time.Now())
	if err != nil {
		return taker, nil, nil, err
	}
	result.Taker.Price = taker.Price
	levels.touch(result.Taker)
	for _, maker := range result.Makers {
		levels.touch(maker)
	}
	return result.Taker, result.Makers, result.Trades, nil
}

func (s *Service) captureMatcherSnapshot(ctx context.Context, tx *postgres.ExchangeRepository, marketSymbol string, baseAsset string, quoteAsset string, sequence uint64) (matching.BookStateSnapshot, error) {
	active, err := tx.ListActiveOrdersForUpdate(ctx, marketSymbol, "", 0)
	if err != nil {
		return matching.BookStateSnapshot{}, err
	}
	book := matching.NewMarketBook(marketSymbol, baseAsset, quoteAsset)
	if err := book.Load(active); err != nil {
		return matching.BookStateSnapshot{}, err
	}
	return book.CaptureState(sequence, time.Now()), nil
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
		updated, makers, trades, err := s.match(ctx, tx, item, levels)
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
		if err := tx.CreateOrderEvents(ctx, matchEvents(updated, makers, trades)); err != nil {
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

func stopPriceValidForLastPrice(item order.Order, lastPrice string) bool {
	if item.Type != order.TypeStopLimit || lastPrice == "" {
		return true
	}
	switch item.Side {
	case order.SideBuy:
		return decimal.Cmp(item.StopPrice, lastPrice) > 0
	case order.SideSell:
		return decimal.Cmp(item.StopPrice, lastPrice) < 0
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
	case order.StatusPendingMatch, order.StatusOpen, order.StatusPartiallyFilled, order.StatusPendingStop:
		return true
	default:
		return false
	}
}

func cancelAlreadyTerminal(status order.Status) bool {
	switch status {
	case order.StatusFilled, order.StatusCanceled, order.StatusExpired:
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

func effectiveMatchTaker(item order.Order) order.Order {
	if item.Type == order.TypeMarket && item.Side == order.SideSell {
		item.Price = minPositiveMarketPrice
	}
	return item
}

func knownStatus(status order.Status) bool {
	switch status {
	case order.StatusPendingMatch, order.StatusPendingStop, order.StatusOpen, order.StatusPartiallyFilled, order.StatusFilled, order.StatusCanceled, order.StatusExpired, order.StatusRejected:
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

func asyncMatchingEnabled() bool {
	mode := matchingMode()
	return mode == "async" || mode == "command_log"
}

func commandLogMatchingEnabled() bool {
	return matchingMode() == "command_log"
}

func matchingMode() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("MATCHING_MODE")))
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
	out, err := s.applyBalanceAmountWithEvent(ctx, userID, req, "", func(tx *postgres.ExchangeRepository, userID string, asset string, amount string, eventID balance.EventID) (*balance.Balance, error) {
		if eventID == "" {
			eventID = balance.EventID(idgen.New("bev"))
		}
		return tx.MarkDepositPending(ctx, userID, asset, amount, eventID)
	})
	if err != nil {
		return nil, err
	}
	s.publishBalanceUpdate("exchange.deposit_pending", out)
	return out, nil
}

func (s *Service) SettleDeposit(ctx context.Context, userID string, req BalanceAmountRequest) (*balance.Balance, error) {
	out, err := s.applyBalanceAmountWithEvent(ctx, userID, req, "", func(tx *postgres.ExchangeRepository, userID string, asset string, amount string, eventID balance.EventID) (*balance.Balance, error) {
		if eventID == "" {
			eventID = balance.EventID(idgen.New("bev"))
		}
		return tx.SettleDeposit(ctx, userID, asset, amount, eventID)
	})
	if err != nil {
		return nil, err
	}
	s.publishBalanceUpdate("exchange.deposit_settled", out)
	return out, nil
}

func (s *Service) ApplyGatewayDepositCallback(ctx context.Context, req GatewayDepositCallback) (*GatewayCallbackResult, error) {
	status := normalizeGatewayStatus(req.Status)
	if status != "pending" && status != "settled" {
		return &GatewayCallbackResult{Status: "ok", Action: "status_ignored"}, nil
	}
	userID := strings.TrimSpace(req.UserID)
	asset := s.gatewayDepositAsset(req)
	amount, err := gatewayDepositAmount(req)
	if err != nil {
		return nil, err
	}
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	if asset == "" {
		return nil, fmt.Errorf("%w: asset is required", ErrInvalidOrder)
	}
	if !validGatewayAssetSymbol(asset) {
		return nil, fmt.Errorf("%w: invalid asset %s", ErrInvalidOrder, asset)
	}
	ref := gatewayDepositReference(req)
	if ref == "" {
		return nil, fmt.Errorf("%w: gateway callback reference is required", ErrInvalidOrder)
	}
	pendingEventID := gatewayBalanceEventID("gwdep_p", ref)
	settleEventID := gatewayBalanceEventID("gwdep_s", ref)

	switch status {
	case "pending":
		out, processed, err := s.markDepositPendingOnce(ctx, userID, asset, amount, pendingEventID)
		if err != nil {
			return nil, err
		}
		action := "deposit_pending"
		if !processed {
			action = "duplicate_ignored"
		}
		return &GatewayCallbackResult{Status: "ok", Action: action, Balance: out}, nil
	case "settled":
		out, processed, err := s.settleGatewayDepositOnce(ctx, userID, asset, amount, pendingEventID, settleEventID)
		if err != nil {
			return nil, err
		}
		action := "deposit_settled"
		if !processed {
			action = "duplicate_ignored"
		}
		return &GatewayCallbackResult{Status: "ok", Action: action, Balance: out}, nil
	default:
		return &GatewayCallbackResult{Status: "ok", Action: "status_ignored"}, nil
	}
}

func (s *Service) ApplyGatewayWithdrawalCallback(ctx context.Context, req GatewayWithdrawalCallback) (*GatewayCallbackResult, error) {
	id := strings.TrimSpace(req.WithdrawalID)
	if id == "" {
		id = strings.TrimSpace(req.PayoutID)
	}
	if id == "" {
		id = strings.TrimSpace(req.ID)
	}
	if id == "" {
		return nil, fmt.Errorf("%w: withdrawal_id is required", ErrInvalidOrder)
	}
	status := normalizeGatewayStatus(req.Status)
	var out *balance.Withdrawal
	var err error
	switch status {
	case "settled":
		out, err = s.CompleteWithdrawal(ctx, balance.WithdrawalID(id))
	case "canceled":
		out, err = s.CancelWithdrawal(ctx, balance.WithdrawalID(id))
	default:
		return &GatewayCallbackResult{Status: "ok", Action: "status_ignored"}, nil
	}
	if err != nil {
		return nil, err
	}
	return &GatewayCallbackResult{Status: "ok", Action: "withdrawal_" + string(out.Status), Withdrawal: out}, nil
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

func (s *Service) markDepositPendingOnce(ctx context.Context, userID string, asset string, amount string, eventID balance.EventID) (*balance.Balance, bool, error) {
	var out *balance.Balance
	var processed bool
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		exists, err := tx.BalanceEventExists(ctx, eventID)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		updated, err := tx.MarkDepositPending(ctx, userID, asset, amount, eventID)
		if err != nil {
			return err
		}
		out = updated
		processed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if processed {
		s.publishBalanceUpdate("exchange.deposit_pending", out)
		return out, true, nil
	}
	out, err = s.currentBalance(ctx, userID, asset)
	return out, false, err
}

func (s *Service) settleGatewayDepositOnce(ctx context.Context, userID string, asset string, amount string, pendingEventID balance.EventID, settleEventID balance.EventID) (*balance.Balance, bool, error) {
	var out *balance.Balance
	var pendingOut *balance.Balance
	var processed bool
	var pendingCreated bool
	err := s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		settled, err := tx.BalanceEventExists(ctx, settleEventID)
		if err != nil {
			return err
		}
		if settled {
			return nil
		}
		pending, err := tx.BalanceEventExists(ctx, pendingEventID)
		if err != nil {
			return err
		}
		if !pending {
			updated, err := tx.MarkDepositPending(ctx, userID, asset, amount, pendingEventID)
			if err != nil {
				return err
			}
			pendingOut = updated
			pendingCreated = true
		}
		updated, err := tx.SettleDeposit(ctx, userID, asset, amount, settleEventID)
		if err != nil {
			return err
		}
		out = updated
		processed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if processed {
		if pendingCreated {
			s.publishBalanceUpdate("exchange.deposit_pending", pendingOut)
		}
		s.publishBalanceUpdate("exchange.deposit_settled", out)
		return out, true, nil
	}
	out, err = s.currentBalance(ctx, userID, asset)
	return out, false, err
}

func (s *Service) currentBalance(ctx context.Context, userID string, asset string) (*balance.Balance, error) {
	items, err := s.repo.ListBalances(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Asset, asset) {
			return &item, nil
		}
	}
	return nil, nil
}

func (s *Service) applyBalanceAmountWithEvent(ctx context.Context, userID string, req BalanceAmountRequest, eventID balance.EventID, fn func(*postgres.ExchangeRepository, string, string, string, balance.EventID) (*balance.Balance, error)) (*balance.Balance, error) {
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
		updated, err := fn(tx, userID, asset, decimal.String(amount), eventID)
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

func (s *Service) gatewayDepositAsset(req GatewayDepositCallback) string {
	return s.canonicalGatewayAssetSymbol(gatewayDepositRawAsset(req))
}

func gatewayDepositRawAsset(req GatewayDepositCallback) string {
	for _, raw := range []string{req.Asset, req.Symbol, req.SelectedAsset} {
		if value := strings.ToUpper(strings.TrimSpace(raw)); value != "" {
			return value
		}
	}
	return ""
}

func (s *Service) canonicalGatewayAssetSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" || !s.hasAssets {
		return symbol
	}
	if _, ok := s.assets.Get(symbol); ok {
		return symbol
	}
	for _, item := range s.assets.All() {
		for _, deployment := range item.Deployments {
			if strings.EqualFold(deployment.Symbol, symbol) {
				return strings.ToUpper(strings.TrimSpace(item.Symbol))
			}
		}
	}
	return symbol
}

func (s *Service) gatewayDepositSymbol(symbol string, chainKey string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" || !s.hasAssets {
		return symbol
	}
	item, ok := s.assets.Get(symbol)
	if !ok {
		return symbol
	}
	targetChain := canonicalDepositChainKey(chainKey)
	for _, deployment := range item.Deployments {
		if !deployment.Enabled {
			continue
		}
		if targetChain != "" && deployment.ChainKey != targetChain {
			continue
		}
		if value := strings.ToUpper(strings.TrimSpace(deployment.Symbol)); value != "" {
			return value
		}
		return symbol
	}
	return symbol
}

func canonicalDepositChainKey(value string) chain.ChainKey {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "bitcoin":
		return chain.ChainKey("bitcoin")
	case "ethereum":
		return chain.ChainKeyEthereum
	case "binance_smart_chain", "bnbchain", "bsc":
		return chain.ChainKeyBinanceSmartChain
	case "unichain":
		return chain.ChainKeyUnichain
	case "arbitrum":
		return chain.ChainKeyArbitrum
	case "base":
		return chain.ChainKeyBase
	case "avalanche", "avax":
		return chain.ChainKeyAvalanche
	case "chiliz":
		return chain.ChainKeyChiliz
	case "solana":
		return chain.ChainKeySolana
	case "tron":
		return chain.ChainKey("tron")
	default:
		return chain.ChainKey(strings.ToLower(strings.TrimSpace(value)))
	}
}

func validGatewayAssetSymbol(asset string) bool {
	asset = strings.TrimSpace(asset)
	if asset == "" || len(asset) > 32 {
		return false
	}
	for _, ch := range asset {
		switch {
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '.', ch == '-', ch == '_':
		default:
			return false
		}
	}
	return true
}

func gatewayDepositAmount(req GatewayDepositCallback) (string, error) {
	if strings.TrimSpace(req.Amount) != "" {
		amount, err := parsePositiveDecimal(req.Amount, "amount")
		if err != nil {
			return "", err
		}
		return decimal.String(amount), nil
	}
	if strings.TrimSpace(req.AmountRaw) == "" {
		return "", fmt.Errorf("%w: amount is required", ErrInvalidOrder)
	}
	if req.Decimals < 0 {
		return "", fmt.Errorf("%w: decimals must be non-negative", ErrInvalidOrder)
	}
	if req.Decimals == 0 {
		amount, err := parsePositiveDecimal(req.AmountRaw, "amount_raw")
		if err != nil {
			return "", err
		}
		return decimal.String(amount), nil
	}
	return rawAmountToDecimal(req.AmountRaw, req.Decimals)
}

func rawAmountToDecimal(raw string, decimals int) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: amount_raw is required", ErrInvalidOrder)
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return "", fmt.Errorf("%w: amount_raw must contain digits only", ErrInvalidOrder)
		}
	}
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		return "", fmt.Errorf("%w: amount_raw must be positive", ErrInvalidOrder)
	}
	if decimals > 0 {
		if len(raw) <= decimals {
			raw = strings.Repeat("0", decimals-len(raw)+1) + raw
		}
		idx := len(raw) - decimals
		raw = raw[:idx] + "." + raw[idx:]
		raw = strings.TrimRight(raw, "0")
		raw = strings.TrimRight(raw, ".")
	}
	amount, err := parsePositiveDecimal(raw, "amount_raw")
	if err != nil {
		return "", err
	}
	return decimal.String(amount), nil
}

func gatewayDepositReference(req GatewayDepositCallback) string {
	for _, raw := range []string{req.EventID, req.PaymentID, req.TrackID, req.OrderID, req.TxHash} {
		if value := strings.TrimSpace(raw); value != "" {
			return value
		}
	}
	return ""
}

func gatewayBalanceEventID(prefix string, ref string) balance.EventID {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ref)))
	return balance.EventID(prefix + "_" + hex.EncodeToString(sum[:16]))
}

func normalizeGatewayStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "pending", "awaiting_payment", "processing", "requested":
		return "pending"
	case "paid", "confirmed", "complete", "completed", "settled", "success", "succeeded":
		return "settled"
	case "cancelled", "canceled", "failed", "rejected", "expired":
		return "canceled"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func (s *Service) ListWallets(ctx context.Context, userID string) ([]balance.Wallet, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	return s.repo.ListWallets(ctx, userID)
}

func (s *Service) DepositAddress(ctx context.Context, userID string, req DepositAddressRequest) (*DepositAddress, error) {
	userID = strings.TrimSpace(userID)
	asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	chainKey := strings.ToLower(strings.TrimSpace(req.ChainKey))
	label := strings.TrimSpace(req.Label)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	if asset == "" {
		return nil, fmt.Errorf("%w: asset is required", ErrInvalidOrder)
	}
	if chainKey == "" {
		return nil, fmt.Errorf("%w: chain_key is required", ErrInvalidOrder)
	}
	chainID, ok := gatewayChainID(chainKey)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported gateway chain_key %s", ErrInvalidOrder, chainKey)
	}
	if s.gateway == nil {
		return nil, fmt.Errorf("payment gateway static address provider is not configured")
	}
	gatewaySymbol := s.gatewayDepositSymbol(asset, chainKey)
	item, err := s.gateway.CreateStaticAddress(ctx, userID, gatewaySymbol, chainID, label)
	if err != nil {
		return nil, err
	}
	address := strings.TrimSpace(item.Address)
	if address == "" {
		return nil, fmt.Errorf("payment gateway static address response did not include an address")
	}
	if err := validateGatewayAddress(chainKey, address); err != nil {
		return nil, err
	}

	err = s.repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		_, err := tx.UpsertWallet(ctx, balance.Wallet{UserID: userID, ChainKey: chainKey, Address: address})
		return err
	})
	if err != nil {
		return nil, err
	}

	out := &DepositAddress{
		UserID:   userID,
		Asset:    asset,
		ChainKey: chainKey,
		ChainID:  chainID,
		Address:  address,
		WalletID: strings.TrimSpace(item.WalletID),
		Label:    firstNonEmptyString(label, strings.TrimSpace(item.Label)),
	}
	s.publishWalletUpdate("exchange.wallet_registered", &balance.Wallet{UserID: userID, ChainKey: chainKey, Address: address})
	return out, nil
}

func (s *Service) GatewayQRCode(ctx context.Context, address string, size int) ([]byte, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("%w: address is required", ErrInvalidOrder)
	}
	if size <= 0 {
		size = 300
	}
	if size < 128 || size > 1024 {
		return nil, fmt.Errorf("%w: qrcode size must be between 128 and 1024", ErrInvalidOrder)
	}
	if s.gateway == nil {
		return nil, fmt.Errorf("payment gateway qrcode provider is not configured")
	}
	return s.gateway.QRCode(ctx, address, size)
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

type newOrderCommandPayload struct {
	CommandType   string `json:"command_type"`
	ClientOrderID string `json:"client_order_id"`
	ReservationID string `json:"reservation_id"`
	UserID        string `json:"user_id"`
	Market        string `json:"market"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"time_in_force"`
	Price         string `json:"price"`
	StopPrice     string `json:"stop_price"`
	Quantity      string `json:"quantity"`
}

func orderCommandID(req PlaceRequest, item order.Order) string {
	if value := strings.TrimSpace(req.CommandID); value != "" {
		return value
	}
	return strings.TrimSpace(string(item.ClientOrderID))
}

func orderCommandPayload(item order.Order) (string, string, error) {
	payload := newOrderCommandPayload{
		CommandType:   postgres.OrderCommandTypeNewOrder,
		ClientOrderID: string(item.ClientOrderID),
		ReservationID: item.ReservationID,
		UserID:        item.UserID,
		Market:        item.Market,
		Side:          string(item.Side),
		Type:          string(item.Type),
		TimeInForce:   string(item.TimeInForce),
		Price:         decimal.Trim(item.Price),
		StopPrice:     decimal.Trim(zeroIfEmptyString(item.StopPrice)),
		Quantity:      decimal.Trim(item.Quantity),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	return string(raw), hex.EncodeToString(sum[:]), nil
}

func sameOrderCommandPayload(existing order.Order, requested order.Order) bool {
	return existing.UserID == requested.UserID &&
		existing.ReservationID == requested.ReservationID &&
		existing.Market == requested.Market &&
		existing.Side == requested.Side &&
		existing.Type == requested.Type &&
		existing.TimeInForce == requested.TimeInForce &&
		decimal.Cmp(existing.Price, requested.Price) == 0 &&
		decimal.Cmp(zeroIfEmptyString(existing.StopPrice), zeroIfEmptyString(requested.StopPrice)) == 0 &&
		decimal.Cmp(existing.Quantity, requested.Quantity) == 0
}

func orderReservationID(req PlaceRequest, userID string, clientOrderID string) string {
	if value := strings.TrimSpace(req.ReservationID); value != "" {
		return value
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(userID) + "|" + strings.TrimSpace(clientOrderID)))
	return "res_" + hex.EncodeToString(sum[:12])
}

func zeroIfEmptyString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
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
	hasChainMetadata := false
	for _, item := range markets {
		for _, key := range item.ChainKeys {
			hasChainMetadata = true
			if strings.EqualFold(key, chainKey) {
				return true
			}
		}
	}
	return !hasChainMetadata
}

func gatewayChainID(chainKey string) (int64, bool) {
	switch strings.ToLower(strings.TrimSpace(chainKey)) {
	case "bitcoin":
		return 0, true
	case "ethereum":
		return 1, true
	case "binance_smart_chain", "bnbchain", "bsc":
		return 56, true
	case "unichain":
		return 130, true
	case "arbitrum":
		return 42161, true
	case "base":
		return 8453, true
	case "avalanche", "avax":
		return 43114, true
	case "chiliz":
		return 88888, true
	case "solana":
		return 99999999, true
	case "tron":
		return 99999998, true
	default:
		return 0, false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func validateGatewayAddress(chainKey string, address string) error {
	chainKey = strings.ToLower(strings.TrimSpace(chainKey))
	address = strings.TrimSpace(address)
	switch chainKey {
	case "ethereum", "base", "chiliz", "avalanche", "avax", "unichain", "arbitrum", "binance_smart_chain", "bnbchain", "bsc":
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

func refreshBookProjection(ctx context.Context, tx *postgres.ExchangeRepository, marketSymbol string, levels *levelTracker) ([]orderbook.PriceLevel, error) {
	keys := levels.keys()
	if err := tx.RefreshPriceLevels(ctx, keys); err != nil {
		return nil, err
	}
	if err := tx.PruneStalePriceLevels(ctx, marketSymbol); err != nil {
		return nil, err
	}
	return tx.PriceLevelDeltas(ctx, keys)
}

func newBookDelta(marketSymbol string, version uint64, levels []orderbook.PriceLevel) bookDelta {
	return bookDelta{
		Market:  marketSymbol,
		Version: version,
		Levels:  levels,
	}
}

func bookVersion(item order.Order, sequence uint64) uint64 {
	if sequence > 0 {
		return sequence
	}
	return item.SequenceID
}

func maxResultSequence(results []MatchResult) uint64 {
	var max uint64
	for _, result := range results {
		if result.Order.SequenceID > max {
			max = result.Order.SequenceID
		}
	}
	return max
}

func appendMatchEventLog(ctx context.Context, tx *postgres.ExchangeRepository, sequence uint64, taker order.Order, makers []order.Order, trades []trade.Trade, levels []orderbook.PriceLevel) error {
	if sequence == 0 {
		return nil
	}
	payload := matchEventPayload{
		Taker:  taker,
		Makers: makers,
		Trades: trades,
		Levels: levels,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	_, err = tx.AppendMatchEventLog(ctx, postgres.MatchEventLog{
		Market:      taker.Market,
		SequenceID:  sequence,
		Type:        postgres.MatchEventTypeResult,
		PayloadHash: hex.EncodeToString(sum[:]),
		Payload:     string(raw),
	})
	return err
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

func matchEvents(taker order.Order, makers []order.Order, trades []trade.Trade) []order.Event {
	events := eventsForTrades(trades)
	for _, maker := range makers {
		if maker.Status == order.StatusFilled {
			events = append(events, newOrderEvent(maker, order.EventOrderFilled, ""))
		}
	}
	return append(events, terminalOrderEvents(taker)...)
}

func terminalEvents(item order.Order, trades []trade.Trade) []order.Event {
	return append(eventsForTrades(trades), terminalOrderEvents(item)...)
}

func terminalOrderEvents(item order.Order) []order.Event {
	switch item.Status {
	case order.StatusFilled:
		return []order.Event{newOrderEvent(item, order.EventOrderFilled, "")}
	case order.StatusExpired:
		return []order.Event{newOrderEvent(item, order.EventOrderExpired, "")}
	default:
		return nil
	}
}

type socketEvent struct {
	Type       string                 `json:"type"`
	Market     string                 `json:"market,omitempty"`
	UserID     string                 `json:"user_id,omitempty"`
	Version    uint64                 `json:"version,omitempty"`
	Order      *order.Order           `json:"order,omitempty"`
	Trades     []trade.Trade          `json:"trades,omitempty"`
	Bids       []orderbook.PriceLevel `json:"bids,omitempty"`
	Asks       []orderbook.PriceLevel `json:"asks,omitempty"`
	Balance    *balance.Balance       `json:"balance,omitempty"`
	Withdrawal *balance.Withdrawal    `json:"withdrawal,omitempty"`
	Wallet     *balance.Wallet        `json:"wallet,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
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
		Version:   item.SequenceID,
		CreatedAt: time.Now(),
	})
}

func (s *Service) publishOrderBookDelta(delta bookDelta) {
	if s.publish == nil || strings.TrimSpace(delta.Market) == "" || len(delta.Levels) == 0 {
		return
	}
	bids := make([]orderbook.PriceLevel, 0)
	asks := make([]orderbook.PriceLevel, 0)
	for _, level := range delta.Levels {
		switch level.Side {
		case order.SideBuy:
			bids = append(bids, level)
		case order.SideSell:
			asks = append(asks, level)
		}
	}
	s.publishJSON(socketEvent{
		Type:      "exchange.orderbook_delta",
		Market:    delta.Market,
		Version:   delta.Version,
		Bids:      bids,
		Asks:      asks,
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

func fallbackCandles(marketSymbol string, interval trade.Interval, limit int, price string) []trade.Candle {
	if limit <= 0 || limit > 1000 {
		limit = 120
	}
	if limit < 2 {
		limit = 2
	}
	base := decimal.Parse(price)
	if base.Sign() <= 0 {
		base = big.NewRat(1, 1)
	}
	now := time.Now().UTC().Truncate(interval.Duration)
	out := make([]trade.Candle, 0, limit)
	prevClose := ratByBps(base, -5)
	for i := 0; i < limit; i++ {
		offset := time.Duration(limit-1-i) * interval.Duration
		openTime := now.Add(-offset)
		close := ratByBps(base, int64((i%11)-5))
		if i == limit-1 {
			close = new(big.Rat).Set(base)
		}
		high := maxRat(prevClose, close)
		high = ratByBps(high, 2)
		low := minRat(prevClose, close)
		low = ratByBps(low, -2)
		out = append(out, trade.Candle{
			Market:      marketSymbol,
			Interval:    interval.Key,
			OpenTime:    openTime,
			CloseTime:   openTime.Add(interval.Duration),
			Open:        decimal.String(prevClose),
			High:        decimal.String(high),
			Low:         decimal.String(low),
			Close:       decimal.String(close),
			VolumeBase:  "1",
			VolumeQuote: decimal.String(close),
			TradeCount:  0,
			LastTradeAt: openTime,
		})
		prevClose = close
	}
	return out
}

func ratByBps(value *big.Rat, bps int64) *big.Rat {
	factor := big.NewRat(10000+bps, 10000)
	return new(big.Rat).Mul(value, factor)
}

func maxRat(left *big.Rat, right *big.Rat) *big.Rat {
	if left.Cmp(right) >= 0 {
		return new(big.Rat).Set(left)
	}
	return new(big.Rat).Set(right)
}

func minRat(left *big.Rat, right *big.Rat) *big.Rat {
	if left.Cmp(right) <= 0 {
		return new(big.Rat).Set(left)
	}
	return new(big.Rat).Set(right)
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

func (s *Service) validateStopPlacement(ctx context.Context, tx *postgres.ExchangeRepository, item order.Order) error {
	if item.Type != order.TypeStopLimit {
		return nil
	}
	last, err := tx.LastTrade(ctx, item.Market)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if stopPriceValidForLastPrice(item, last.Price) {
		return nil
	}
	switch item.Side {
	case order.SideBuy:
		return fmt.Errorf("%w: buy stop_price must be above last_price", ErrInvalidOrder)
	case order.SideSell:
		return fmt.Errorf("%w: sell stop_price must be below last_price", ErrInvalidOrder)
	default:
		return fmt.Errorf("%w: stop side is invalid", ErrInvalidOrder)
	}
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
