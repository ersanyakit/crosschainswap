package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"exchange/internal/core/balance"
	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
	"exchange/internal/core/trade"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExchangeRepository struct {
	db *gorm.DB
}

type PriceLevelKey struct {
	Market string
	Side   order.Side
	Price  string
}

func NewExchangeRepository(db *gorm.DB) *ExchangeRepository {
	return &ExchangeRepository{db: db}
}

func (r *ExchangeRepository) Transaction(ctx context.Context, fn func(*ExchangeRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&ExchangeRepository{db: tx})
	})
}

func (r *ExchangeRepository) CreateOrder(ctx context.Context, item order.Order) error {
	model := orderToModel(item)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("failed to create order %s: %w", item.ID, err)
	}
	return nil
}

func (r *ExchangeRepository) SaveOrder(ctx context.Context, item order.Order) error {
	model := orderToModel(item)
	if err := r.db.WithContext(ctx).Save(&model).Error; err != nil {
		return fmt.Errorf("failed to save order %s: %w", item.ID, err)
	}
	return nil
}

func (r *ExchangeRepository) CreateTrades(ctx context.Context, items []trade.Trade) error {
	if len(items) == 0 {
		return nil
	}
	models := make([]ExchangeTrade, 0, len(items))
	for _, item := range items {
		models = append(models, tradeToModel(item))
	}
	if err := r.db.WithContext(ctx).Create(&models).Error; err != nil {
		return fmt.Errorf("failed to create trades: %w", err)
	}
	if err := r.UpdateCandles(ctx, items); err != nil {
		return fmt.Errorf("failed to update candles: %w", err)
	}
	return nil
}

func (r *ExchangeRepository) ListOrders(ctx context.Context, userID string, market string, status order.Status, limit int) ([]order.Order, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := r.db.WithContext(ctx).Where(&ExchangeOrder{UserID: userID}).Limit(limit)
	if market != "" {
		query = query.Where(&ExchangeOrder{Market: market})
	}
	if status != "" {
		query = query.Where(&ExchangeOrder{Status: string(status)})
	}
	query = query.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}, Desc: true}}})

	var models []ExchangeOrder
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return modelsToOrders(models), nil
}

func (r *ExchangeRepository) ListMarketTrades(ctx context.Context, market string, limit int) ([]trade.Trade, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var models []ExchangeTrade
	err := r.db.WithContext(ctx).
		Where(&ExchangeTrade{Market: market}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}, Desc: true}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return modelsToTrades(models), nil
}

func (r *ExchangeRepository) LastTrade(ctx context.Context, market string) (*trade.Trade, error) {
	var model ExchangeTrade
	err := r.db.WithContext(ctx).
		Where(&ExchangeTrade{Market: market}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}, Desc: true}}}).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	item := modelToTrade(model)
	return &item, nil
}

func (r *ExchangeRepository) ListUserTrades(ctx context.Context, userID string, market string, limit int) ([]trade.Trade, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := r.db.WithContext(ctx).Limit(limit)
	if market != "" {
		query = query.Where(&ExchangeTrade{Market: market, MakerUserID: userID}).
			Or(&ExchangeTrade{Market: market, TakerUserID: userID})
	} else {
		query = query.Where(&ExchangeTrade{MakerUserID: userID}).
			Or(&ExchangeTrade{TakerUserID: userID})
	}
	query = query.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}, Desc: true}}})

	var models []ExchangeTrade
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return modelsToTrades(models), nil
}

func (r *ExchangeRepository) ListCandles(ctx context.Context, market string, interval string, limit int) ([]trade.Candle, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	var models []ExchangeCandle
	err := r.db.WithContext(ctx).
		Where(&ExchangeCandle{Market: market, Interval: interval}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "open_time"}, Desc: true}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]trade.Candle, 0, len(models))
	for i := len(models) - 1; i >= 0; i-- {
		out = append(out, modelToCandle(models[i]))
	}
	return out, nil
}

func (r *ExchangeRepository) UpdateCandles(ctx context.Context, items []trade.Trade) error {
	for _, item := range items {
		for _, interval := range trade.SupportedIntervals {
			if err := r.updateCandle(ctx, item, interval); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ExchangeRepository) CreateOrderEvents(ctx context.Context, items []order.Event) error {
	if len(items) == 0 {
		return nil
	}
	models := make([]ExchangeOrderEvent, 0, len(items))
	for _, item := range items {
		models = append(models, orderEventToModel(item))
	}
	if err := r.db.WithContext(ctx).Create(&models).Error; err != nil {
		return fmt.Errorf("failed to create order events: %w", err)
	}
	return nil
}

func (r *ExchangeRepository) UpsertWallet(ctx context.Context, item balance.Wallet) (*balance.Wallet, error) {
	now := time.Now()
	model := ExchangeWallet{
		UserID:    strings.TrimSpace(item.UserID),
		ChainKey:  strings.ToLower(strings.TrimSpace(item.ChainKey)),
		Address:   strings.TrimSpace(item.Address),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Save(&model).Error; err != nil {
		return nil, err
	}
	out := modelToWallet(model)
	return &out, nil
}

func (r *ExchangeRepository) ListWallets(ctx context.Context, userID string) ([]balance.Wallet, error) {
	var models []ExchangeWallet
	if err := r.db.WithContext(ctx).Where(&ExchangeWallet{UserID: userID}).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]balance.Wallet, 0, len(models))
	for _, model := range models {
		out = append(out, modelToWallet(model))
	}
	return out, nil
}

func (r *ExchangeRepository) ListBalances(ctx context.Context, userID string) ([]balance.Balance, error) {
	var models []ExchangeBalance
	if err := r.db.WithContext(ctx).Where(&ExchangeBalance{UserID: userID}).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]balance.Balance, 0, len(models))
	for _, model := range models {
		out = append(out, modelToBalance(model))
	}
	return out, nil
}

func (r *ExchangeRepository) MarkDepositPending(ctx context.Context, userID string, asset string, amount string, eventID balance.EventID) (*balance.Balance, error) {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	model, err := r.balanceForUpdate(ctx, userID, asset, true)
	if err != nil {
		return nil, err
	}
	model.Pending = decimal.Add(model.Pending, amount)
	model.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return nil, err
	}
	if err := r.createBalanceEvent(ctx, balance.Event{
		ID:        eventID,
		UserID:    userID,
		Asset:     asset,
		Type:      balance.EventDepositPending,
		Amount:    amount,
		CreatedAt: time.Now(),
	}); err != nil {
		return nil, err
	}
	out := modelToBalance(*model)
	return &out, nil
}

func (r *ExchangeRepository) SettleDeposit(ctx context.Context, userID string, asset string, amount string, eventID balance.EventID) (*balance.Balance, error) {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	model, err := r.balanceForUpdate(ctx, userID, asset, false)
	if err != nil {
		return nil, err
	}
	if decimal.Cmp(model.Pending, amount) < 0 {
		return nil, balance.ErrInsufficientBalance
	}
	model.Pending = decimal.SubFloorZero(model.Pending, amount)
	model.Available = decimal.Add(model.Available, amount)
	model.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return nil, err
	}
	if err := r.createBalanceEvent(ctx, balance.Event{
		ID:        eventID,
		UserID:    userID,
		Asset:     asset,
		Type:      balance.EventDepositSettled,
		Amount:    amount,
		CreatedAt: time.Now(),
	}); err != nil {
		return nil, err
	}
	out := modelToBalance(*model)
	return &out, nil
}

func (r *ExchangeRepository) RequestWithdrawal(ctx context.Context, item balance.Withdrawal, eventID balance.EventID) (*balance.Withdrawal, error) {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	model, err := r.balanceForUpdate(ctx, item.UserID, item.Asset, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, balance.ErrInsufficientBalance
		}
		return nil, err
	}
	if decimal.Cmp(model.Available, item.Amount) < 0 {
		return nil, balance.ErrInsufficientBalance
	}
	now := time.Now()
	withdrawal := withdrawalToModel(item)
	withdrawal.Status = string(balance.WithdrawalRequested)
	withdrawal.CreatedAt = now
	withdrawal.UpdatedAt = now

	model.Available = decimal.SubFloorZero(model.Available, item.Amount)
	model.Pending = decimal.Add(model.Pending, item.Amount)
	model.UpdatedAt = now
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Create(&withdrawal).Error; err != nil {
		return nil, err
	}
	if err := r.createBalanceEvent(ctx, balance.Event{
		ID:           eventID,
		UserID:       item.UserID,
		Asset:        item.Asset,
		Type:         balance.EventWithdrawalRequested,
		Amount:       item.Amount,
		WithdrawalID: string(item.ID),
		CreatedAt:    now,
	}); err != nil {
		return nil, err
	}
	out := modelToWithdrawal(withdrawal)
	return &out, nil
}

func (r *ExchangeRepository) CompleteWithdrawal(ctx context.Context, id balance.WithdrawalID, eventID balance.EventID) (*balance.Withdrawal, error) {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	return r.finalizeWithdrawal(ctx, id, balance.WithdrawalCompleted, balance.EventWithdrawalCompleted, eventID)
}

func (r *ExchangeRepository) CancelWithdrawal(ctx context.Context, id balance.WithdrawalID, eventID balance.EventID) (*balance.Withdrawal, error) {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	return r.finalizeWithdrawal(ctx, id, balance.WithdrawalCanceled, balance.EventWithdrawalCanceled, eventID)
}

func (r *ExchangeRepository) ListWithdrawals(ctx context.Context, userID string, limit int) ([]balance.Withdrawal, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var models []ExchangeWithdrawal
	err := r.db.WithContext(ctx).
		Where(&ExchangeWithdrawal{UserID: userID}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}, Desc: true}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]balance.Withdrawal, 0, len(models))
	for _, model := range models {
		out = append(out, modelToWithdrawal(model))
	}
	return out, nil
}

func (r *ExchangeRepository) ReserveOrderFunds(ctx context.Context, item order.Order, eventID balance.EventID) error {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	asset, amount := orderReserveAssetAmount(item)
	if decimal.Cmp(amount, "0") <= 0 {
		return nil
	}
	model, err := r.balanceForUpdate(ctx, item.UserID, asset, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return balance.ErrInsufficientBalance
		}
		return err
	}
	if decimal.Cmp(model.Available, amount) < 0 {
		return balance.ErrInsufficientBalance
	}
	model.Available = decimal.SubFloorZero(model.Available, amount)
	model.Locked = decimal.Add(model.Locked, amount)
	model.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return err
	}
	return r.createBalanceEvent(ctx, balance.Event{
		ID:        eventID,
		UserID:    item.UserID,
		Asset:     asset,
		Type:      balance.EventReserve,
		Amount:    amount,
		OrderID:   string(item.ID),
		CreatedAt: time.Now(),
	})
}

func (r *ExchangeRepository) ReleaseOrderFunds(ctx context.Context, item order.Order, eventID balance.EventID) error {
	if eventID == "" {
		eventID = balance.EventID(idgen.New("bev"))
	}
	asset, amount := orderReserveAssetAmount(item)
	if decimal.Cmp(amount, "0") <= 0 {
		return nil
	}
	return r.moveLockedToAvailable(ctx, item.UserID, asset, amount, balance.Event{
		ID:        eventID,
		UserID:    item.UserID,
		Asset:     asset,
		Type:      balance.EventRelease,
		Amount:    amount,
		OrderID:   string(item.ID),
		CreatedAt: time.Now(),
	})
}

func (r *ExchangeRepository) SettleTrades(ctx context.Context, items []trade.Trade) error {
	for _, item := range items {
		maker, err := r.GetOrder(ctx, item.MakerOrderID)
		if err != nil {
			return err
		}
		taker, err := r.GetOrder(ctx, item.TakerOrderID)
		if err != nil {
			return err
		}
		if err := r.settleTrade(ctx, item, *maker, *taker); err != nil {
			return err
		}
	}
	return nil
}

func (r *ExchangeRepository) GetOrder(ctx context.Context, id order.ID) (*order.Order, error) {
	return r.getOrder(ctx, id, false)
}

func (r *ExchangeRepository) GetOrderForUpdate(ctx context.Context, id order.ID) (*order.Order, error) {
	return r.getOrder(ctx, id, true)
}

func (r *ExchangeRepository) FindOrderByClientID(ctx context.Context, userID string, clientOrderID order.ClientOrderID) (*order.Order, error) {
	var model ExchangeOrder
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrder{UserID: userID, ClientOrderID: string(clientOrderID)}).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	item := modelToOrder(model)
	return &item, nil
}

func (r *ExchangeRepository) getOrder(ctx context.Context, id order.ID, lock bool) (*order.Order, error) {
	var model ExchangeOrder
	query := r.db.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&model, "id = ?", string(id)).Error; err != nil {
		return nil, err
	}
	item := modelToOrder(model)
	return &item, nil
}

func (r *ExchangeRepository) ListOpenOrders(ctx context.Context, market string, side order.Side, limit int) ([]order.Order, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var models []ExchangeOrder
	query := r.db.WithContext(ctx).
		Where(&ExchangeOrder{Market: market, Side: string(side)}).
		Where("status IN ?", openStatuses()).
		Limit(limit)

	if side == order.SideBuy {
		query = query.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "price"}, Desc: true},
			{Column: clause.Column{Name: "created_at"}},
		}})
	} else {
		query = query.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "price"}},
			{Column: clause.Column{Name: "created_at"}},
		}})
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return modelsToOrders(models), nil
}

func (r *ExchangeRepository) ListMatchCandidates(ctx context.Context, market string, makerSide order.Side, takerPrice string, excludeOrderID order.ID, excludeUserID string, limit int) ([]order.Order, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}

	var models []ExchangeOrder
	query := r.db.WithContext(ctx).
		Where(&ExchangeOrder{Market: market, Side: string(makerSide)}).
		Where("status IN ?", openStatuses()).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Limit(limit)
	if excludeOrderID != "" {
		query = query.Where("id <> ?", string(excludeOrderID))
	}
	if excludeUserID != "" {
		query = query.Where("user_id <> ?", excludeUserID)
	}

	if makerSide == order.SideSell {
		query = query.Where("price <= ?", takerPrice).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "price"}},
				{Column: clause.Column{Name: "created_at"}},
			}})
	} else {
		query = query.Where("price >= ?", takerPrice).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "price"}, Desc: true},
				{Column: clause.Column{Name: "created_at"}},
			}})
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return modelsToOrders(models), nil
}

func (r *ExchangeRepository) ListPendingStops(ctx context.Context, market string, limit int) ([]order.Order, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}

	var models []ExchangeOrder
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrder{Market: market, Status: string(order.StatusPendingStop)}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return modelsToOrders(models), nil
}

func (r *ExchangeRepository) RefreshPriceLevels(ctx context.Context, keys []PriceLevelKey) error {
	keys = uniquePriceLevelKeys(keys)
	for _, key := range keys {
		if key.Market == "" || key.Side == "" || key.Price == "" {
			continue
		}

		var models []ExchangeOrder
		err := r.db.WithContext(ctx).
			Select("remaining_quantity").
			Where(&ExchangeOrder{Market: key.Market, Side: string(key.Side), Price: key.Price}).
			Where("status IN ?", openStatuses()).
			Find(&models).Error
		if err != nil {
			return err
		}

		if len(models) == 0 {
			err := r.db.WithContext(ctx).
				Where(&ExchangePriceLevel{Market: key.Market, Side: string(key.Side), Price: key.Price}).
				Delete(&ExchangePriceLevel{}).Error
			if err != nil {
				return err
			}
			continue
		}

		level := ExchangePriceLevel{
			Market:        key.Market,
			Side:          string(key.Side),
			Price:         key.Price,
			Quantity:      "0",
			LastUpdatedAt: time.Now(),
		}
		for _, item := range models {
			if item.RemainingQuantity == "" || item.RemainingQuantity == "0" {
				continue
			}
			level.Quantity = decimal.Add(level.Quantity, item.RemainingQuantity)
			level.OrderCount++
		}
		if level.OrderCount == 0 {
			err := r.db.WithContext(ctx).
				Where(&ExchangePriceLevel{Market: key.Market, Side: string(key.Side), Price: key.Price}).
				Delete(&ExchangePriceLevel{}).Error
			if err != nil {
				return err
			}
			continue
		}
		if err := r.db.WithContext(ctx).Save(&level).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *ExchangeRepository) RebuildPriceLevels(ctx context.Context, market string) error {
	if err := r.db.WithContext(ctx).Where(&ExchangePriceLevel{Market: market}).Delete(&ExchangePriceLevel{}).Error; err != nil {
		return err
	}

	for _, side := range []order.Side{order.SideBuy, order.SideSell} {
		orders, err := r.ListOpenOrders(ctx, market, side, 1000)
		if err != nil {
			return err
		}
		levels := aggregateLevels(market, side, orders)
		if len(levels) == 0 {
			continue
		}
		models := make([]ExchangePriceLevel, 0, len(levels))
		for _, level := range levels {
			models = append(models, priceLevelToModel(level))
		}
		if err := r.db.WithContext(ctx).Create(&models).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *ExchangeRepository) ListPriceLevels(ctx context.Context, market string, side order.Side, limit int) ([]orderbook.PriceLevel, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var models []ExchangePriceLevel
	query := r.db.WithContext(ctx).Where(&ExchangePriceLevel{Market: market, Side: string(side)}).Limit(limit)
	if side == order.SideBuy {
		query = query.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "price"}, Desc: true}}})
	} else {
		query = query.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "price"}}}})
	}
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]orderbook.PriceLevel, 0, len(models))
	for _, model := range models {
		out = append(out, modelToPriceLevel(model))
	}
	return out, nil
}

func openStatuses() []string {
	return []string{string(order.StatusOpen), string(order.StatusPartiallyFilled)}
}

func orderToModel(item order.Order) ExchangeOrder {
	return ExchangeOrder{
		ID:                string(item.ID),
		ClientOrderID:     string(item.ClientOrderID),
		UserID:            item.UserID,
		Market:            item.Market,
		BaseAsset:         item.BaseAsset,
		QuoteAsset:        item.QuoteAsset,
		Side:              string(item.Side),
		Type:              string(item.Type),
		Status:            string(item.Status),
		TimeInForce:       string(item.TimeInForce),
		Price:             item.Price,
		StopPrice:         zeroIfEmpty(item.StopPrice),
		Quantity:          item.Quantity,
		FilledQuantity:    zeroIfEmpty(item.FilledQuantity),
		RemainingQuantity: item.RemainingQuantity,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}

func modelToOrder(model ExchangeOrder) order.Order {
	return order.Order{
		ID:                order.ID(model.ID),
		ClientOrderID:     order.ClientOrderID(model.ClientOrderID),
		UserID:            model.UserID,
		Market:            model.Market,
		BaseAsset:         model.BaseAsset,
		QuoteAsset:        model.QuoteAsset,
		Side:              order.Side(model.Side),
		Type:              order.Type(model.Type),
		Status:            order.Status(model.Status),
		TimeInForce:       order.TimeInForce(model.TimeInForce),
		Price:             model.Price,
		StopPrice:         model.StopPrice,
		Quantity:          model.Quantity,
		FilledQuantity:    model.FilledQuantity,
		RemainingQuantity: model.RemainingQuantity,
		CreatedAt:         model.CreatedAt,
		UpdatedAt:         model.UpdatedAt,
	}
}

func modelsToOrders(models []ExchangeOrder) []order.Order {
	out := make([]order.Order, 0, len(models))
	for _, model := range models {
		out = append(out, modelToOrder(model))
	}
	return out
}

func tradeToModel(item trade.Trade) ExchangeTrade {
	return ExchangeTrade{
		ID:            string(item.ID),
		Market:        item.Market,
		BaseAsset:     item.BaseAsset,
		QuoteAsset:    item.QuoteAsset,
		MakerOrderID:  string(item.MakerOrderID),
		TakerOrderID:  string(item.TakerOrderID),
		MakerUserID:   item.MakerUserID,
		TakerUserID:   item.TakerUserID,
		TakerSide:     string(item.TakerSide),
		Price:         item.Price,
		Quantity:      item.Quantity,
		QuoteQuantity: item.QuoteQuantity,
		CreatedAt:     item.CreatedAt,
	}
}

func modelToTrade(model ExchangeTrade) trade.Trade {
	return trade.Trade{
		ID:            trade.ID(model.ID),
		Market:        model.Market,
		BaseAsset:     model.BaseAsset,
		QuoteAsset:    model.QuoteAsset,
		MakerOrderID:  order.ID(model.MakerOrderID),
		TakerOrderID:  order.ID(model.TakerOrderID),
		MakerUserID:   model.MakerUserID,
		TakerUserID:   model.TakerUserID,
		TakerSide:     order.Side(model.TakerSide),
		Price:         model.Price,
		Quantity:      model.Quantity,
		QuoteQuantity: model.QuoteQuantity,
		CreatedAt:     model.CreatedAt,
	}
}

func modelsToTrades(models []ExchangeTrade) []trade.Trade {
	out := make([]trade.Trade, 0, len(models))
	for _, model := range models {
		out = append(out, modelToTrade(model))
	}
	return out
}

func modelToCandle(model ExchangeCandle) trade.Candle {
	return trade.Candle{
		Market:      model.Market,
		Interval:    model.Interval,
		OpenTime:    model.OpenTime,
		CloseTime:   model.CloseTime,
		Open:        model.Open,
		High:        model.High,
		Low:         model.Low,
		Close:       model.Close,
		VolumeBase:  model.VolumeBase,
		VolumeQuote: model.VolumeQuote,
		TradeCount:  model.TradeCount,
		LastTradeAt: model.LastTradeAt,
	}
}

func orderEventToModel(item order.Event) ExchangeOrderEvent {
	return ExchangeOrderEvent{
		ID:        string(item.ID),
		OrderID:   string(item.OrderID),
		UserID:    item.UserID,
		Market:    item.Market,
		Type:      string(item.Type),
		RefID:     item.RefID,
		CreatedAt: item.CreatedAt,
	}
}

func modelToWallet(model ExchangeWallet) balance.Wallet {
	return balance.Wallet{
		UserID:    model.UserID,
		ChainKey:  model.ChainKey,
		Address:   model.Address,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}

func modelToBalance(model ExchangeBalance) balance.Balance {
	return balance.Balance{
		UserID:    model.UserID,
		Asset:     model.Asset,
		Available: zeroIfEmpty(model.Available),
		Locked:    zeroIfEmpty(model.Locked),
		Pending:   zeroIfEmpty(model.Pending),
		UpdatedAt: model.UpdatedAt,
	}
}

func withdrawalToModel(item balance.Withdrawal) ExchangeWithdrawal {
	return ExchangeWithdrawal{
		ID:        string(item.ID),
		UserID:    strings.TrimSpace(item.UserID),
		Asset:     strings.ToUpper(strings.TrimSpace(item.Asset)),
		Amount:    item.Amount,
		ChainKey:  strings.ToLower(strings.TrimSpace(item.ChainKey)),
		Address:   strings.TrimSpace(item.Address),
		Status:    string(item.Status),
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func modelToWithdrawal(model ExchangeWithdrawal) balance.Withdrawal {
	return balance.Withdrawal{
		ID:        balance.WithdrawalID(model.ID),
		UserID:    model.UserID,
		Asset:     model.Asset,
		Amount:    model.Amount,
		ChainKey:  model.ChainKey,
		Address:   model.Address,
		Status:    balance.WithdrawalStatus(model.Status),
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}

func priceLevelToModel(level orderbook.PriceLevel) ExchangePriceLevel {
	return ExchangePriceLevel{
		Market:        level.Market,
		Side:          string(level.Side),
		Price:         level.Price,
		Quantity:      level.Quantity,
		OrderCount:    level.OrderCount,
		LastUpdatedAt: level.LastUpdatedAt,
	}
}

func (r *ExchangeRepository) updateCandle(ctx context.Context, item trade.Trade, interval trade.Interval) error {
	openTime := item.CreatedAt.UTC().Truncate(interval.Duration)
	closeTime := openTime.Add(interval.Duration)
	model := ExchangeCandle{Market: item.Market, Interval: interval.Key, OpenTime: openTime}
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(&ExchangeCandle{Market: item.Market, Interval: interval.Key, OpenTime: openTime}).
		First(&model).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		model = ExchangeCandle{
			Market:      item.Market,
			Interval:    interval.Key,
			OpenTime:    openTime,
			CloseTime:   closeTime,
			Open:        item.Price,
			High:        item.Price,
			Low:         item.Price,
			Close:       item.Price,
			VolumeBase:  item.Quantity,
			VolumeQuote: item.QuoteQuantity,
			TradeCount:  1,
			LastTradeAt: item.CreatedAt,
		}
		return r.db.WithContext(ctx).Create(&model).Error
	}

	if decimal.Cmp(item.Price, model.High) > 0 {
		model.High = item.Price
	}
	if decimal.Cmp(item.Price, model.Low) < 0 {
		model.Low = item.Price
	}
	if item.CreatedAt.After(model.LastTradeAt) || item.CreatedAt.Equal(model.LastTradeAt) {
		model.Close = item.Price
		model.LastTradeAt = item.CreatedAt
	}
	model.VolumeBase = decimal.Add(model.VolumeBase, item.Quantity)
	model.VolumeQuote = decimal.Add(model.VolumeQuote, item.QuoteQuantity)
	model.TradeCount++
	return r.db.WithContext(ctx).Save(&model).Error
}

func (r *ExchangeRepository) finalizeWithdrawal(ctx context.Context, id balance.WithdrawalID, nextStatus balance.WithdrawalStatus, eventType balance.EventType, eventID balance.EventID) (*balance.Withdrawal, error) {
	var model ExchangeWithdrawal
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&model, "id = ?", string(id)).Error
	if err != nil {
		return nil, err
	}
	if model.Status != string(balance.WithdrawalRequested) {
		return nil, fmt.Errorf("withdrawal %s is not requested", id)
	}

	bal, err := r.balanceForUpdate(ctx, model.UserID, model.Asset, false)
	if err != nil {
		return nil, err
	}
	if decimal.Cmp(bal.Pending, model.Amount) < 0 {
		return nil, balance.ErrInsufficientBalance
	}
	now := time.Now()
	bal.Pending = decimal.SubFloorZero(bal.Pending, model.Amount)
	if nextStatus == balance.WithdrawalCanceled {
		bal.Available = decimal.Add(bal.Available, model.Amount)
	}
	bal.UpdatedAt = now
	if err := r.db.WithContext(ctx).Save(bal).Error; err != nil {
		return nil, err
	}

	model.Status = string(nextStatus)
	model.UpdatedAt = now
	if err := r.db.WithContext(ctx).Save(&model).Error; err != nil {
		return nil, err
	}
	if err := r.createBalanceEvent(ctx, balance.Event{
		ID:           eventID,
		UserID:       model.UserID,
		Asset:        model.Asset,
		Type:         eventType,
		Amount:       model.Amount,
		WithdrawalID: model.ID,
		CreatedAt:    now,
	}); err != nil {
		return nil, err
	}
	out := modelToWithdrawal(model)
	return &out, nil
}

func (r *ExchangeRepository) balanceForUpdate(ctx context.Context, userID string, asset string, create bool) (*ExchangeBalance, error) {
	now := time.Now()
	model := ExchangeBalance{UserID: strings.TrimSpace(userID), Asset: strings.ToUpper(strings.TrimSpace(asset))}
	query := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	if create {
		if err := query.Where(&ExchangeBalance{UserID: model.UserID, Asset: model.Asset}).Attrs(ExchangeBalance{
			Available: "0",
			Locked:    "0",
			Pending:   "0",
			UpdatedAt: now,
		}).FirstOrCreate(&model).Error; err != nil {
			return nil, err
		}
		return &model, nil
	}
	if err := query.Where(&ExchangeBalance{UserID: model.UserID, Asset: model.Asset}).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

func (r *ExchangeRepository) createBalanceEvent(ctx context.Context, item balance.Event) error {
	model := ExchangeBalanceEvent{
		ID:           string(item.ID),
		UserID:       item.UserID,
		Asset:        strings.ToUpper(strings.TrimSpace(item.Asset)),
		Type:         string(item.Type),
		Amount:       item.Amount,
		OrderID:      item.OrderID,
		TradeID:      item.TradeID,
		WithdrawalID: item.WithdrawalID,
		CreatedAt:    item.CreatedAt,
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r *ExchangeRepository) settleTrade(ctx context.Context, item trade.Trade, maker order.Order, taker order.Order) error {
	buyer := maker
	seller := taker
	if item.TakerSide == order.SideBuy {
		buyer = taker
		seller = maker
	}

	buyerLockedQuote := decimal.Mul(item.Quantity, buyer.Price)
	buyerQuoteRelease := decimal.SubFloorZero(buyerLockedQuote, item.QuoteQuantity)
	if err := r.debitLocked(ctx, buyer.UserID, item.QuoteAsset, buyerLockedQuote, balance.Event{
		ID:        balance.EventID(""),
		UserID:    buyer.UserID,
		Asset:     item.QuoteAsset,
		Type:      balance.EventDebitLocked,
		Amount:    buyerLockedQuote,
		OrderID:   string(buyer.ID),
		TradeID:   string(item.ID),
		CreatedAt: item.CreatedAt,
	}); err != nil {
		return err
	}
	if decimal.Cmp(buyerQuoteRelease, "0") > 0 {
		if err := r.addAvailableBalance(ctx, buyer.UserID, item.QuoteAsset, buyerQuoteRelease, balance.EventRelease, string(buyer.ID), string(item.ID), item.CreatedAt); err != nil {
			return err
		}
	}
	if err := r.addAvailableBalance(ctx, buyer.UserID, item.BaseAsset, item.Quantity, balance.EventSettlementReceive, string(buyer.ID), string(item.ID), item.CreatedAt); err != nil {
		return err
	}
	if err := r.debitLocked(ctx, seller.UserID, item.BaseAsset, item.Quantity, balance.Event{
		ID:        balance.EventID(""),
		UserID:    seller.UserID,
		Asset:     item.BaseAsset,
		Type:      balance.EventDebitLocked,
		Amount:    item.Quantity,
		OrderID:   string(seller.ID),
		TradeID:   string(item.ID),
		CreatedAt: item.CreatedAt,
	}); err != nil {
		return err
	}
	return r.addAvailableBalance(ctx, seller.UserID, item.QuoteAsset, item.QuoteQuantity, balance.EventSettlementReceive, string(seller.ID), string(item.ID), item.CreatedAt)
}

func (r *ExchangeRepository) moveLockedToAvailable(ctx context.Context, userID string, asset string, amount string, event balance.Event) error {
	if err := r.debitLocked(ctx, userID, asset, amount, balance.Event{
		ID:        event.ID,
		UserID:    userID,
		Asset:     asset,
		Type:      balance.EventRelease,
		Amount:    amount,
		OrderID:   event.OrderID,
		TradeID:   event.TradeID,
		CreatedAt: event.CreatedAt,
	}); err != nil {
		return err
	}
	return r.addAvailableBalance(ctx, userID, asset, amount, balance.EventRelease, event.OrderID, event.TradeID, event.CreatedAt)
}

func (r *ExchangeRepository) debitLocked(ctx context.Context, userID string, asset string, amount string, event balance.Event) error {
	model, err := r.balanceForUpdate(ctx, userID, asset, false)
	if err != nil {
		return err
	}
	if decimal.Cmp(model.Locked, amount) < 0 {
		return balance.ErrInsufficientBalance
	}
	model.Locked = decimal.SubFloorZero(model.Locked, amount)
	model.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return err
	}
	if event.ID == "" {
		event.ID = balance.EventID(idgen.New("bev"))
	}
	event.Asset = asset
	event.Amount = amount
	return r.createBalanceEvent(ctx, event)
}

func (r *ExchangeRepository) addAvailableBalance(ctx context.Context, userID string, asset string, amount string, eventType balance.EventType, orderID string, tradeID string, createdAt time.Time) error {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	model, err := r.balanceForUpdate(ctx, userID, asset, true)
	if err != nil {
		return err
	}
	model.Available = decimal.Add(model.Available, amount)
	model.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return err
	}
	return r.createBalanceEvent(ctx, balance.Event{
		ID:        balance.EventID(idgen.New("bev")),
		UserID:    userID,
		Asset:     asset,
		Type:      eventType,
		Amount:    amount,
		OrderID:   orderID,
		TradeID:   tradeID,
		CreatedAt: createdAt,
	})
}

func orderReserveAssetAmount(item order.Order) (string, string) {
	if item.Side == order.SideBuy {
		return item.QuoteAsset, decimal.Mul(item.Price, item.RemainingQuantity)
	}
	return item.BaseAsset, item.RemainingQuantity
}

func modelToPriceLevel(model ExchangePriceLevel) orderbook.PriceLevel {
	return orderbook.PriceLevel{
		Market:        model.Market,
		Side:          order.Side(model.Side),
		Price:         model.Price,
		Quantity:      model.Quantity,
		OrderCount:    model.OrderCount,
		LastUpdatedAt: model.LastUpdatedAt,
	}
}

func aggregateLevels(market string, side order.Side, orders []order.Order) []orderbook.PriceLevel {
	now := time.Now()
	byPrice := make(map[string]int)
	levels := make([]orderbook.PriceLevel, 0)
	for _, item := range orders {
		if item.RemainingQuantity == "" || item.RemainingQuantity == "0" {
			continue
		}
		idx, ok := byPrice[item.Price]
		if !ok {
			byPrice[item.Price] = len(levels)
			levels = append(levels, orderbook.PriceLevel{
				Market:        market,
				Side:          side,
				Price:         item.Price,
				Quantity:      item.RemainingQuantity,
				OrderCount:    1,
				LastUpdatedAt: now,
			})
			continue
		}
		levels[idx].Quantity = decimal.Add(levels[idx].Quantity, item.RemainingQuantity)
		levels[idx].OrderCount++
	}
	return levels
}

func uniquePriceLevelKeys(keys []PriceLevelKey) []PriceLevelKey {
	seen := make(map[string]struct{}, len(keys))
	out := make([]PriceLevelKey, 0, len(keys))
	for _, key := range keys {
		id := key.Market + "|" + string(key.Side) + "|" + key.Price
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, key)
	}
	return out
}

func zeroIfEmpty(value string) string {
	if value == "" {
		return "0"
	}
	return value
}
