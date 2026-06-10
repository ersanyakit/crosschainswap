package matching

import (
	"errors"
	"fmt"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"
)

var ErrInvalidMatch = errors.New("invalid match command")

type TradeIDFactory func() trade.ID

type Result struct {
	Taker  order.Order
	Makers []order.Order
	Trades []trade.Trade
}

func MatchLimit(taker order.Order, makers []order.Order, newTradeID TradeIDFactory, now time.Time) (Result, error) {
	if newTradeID == nil {
		return Result{}, fmt.Errorf("%w: trade id factory is required", ErrInvalidMatch)
	}
	if err := validateTaker(taker); err != nil {
		return Result{}, err
	}

	result := Result{Taker: taker, Makers: make([]order.Order, 0, len(makers)), Trades: make([]trade.Trade, 0)}
	for _, maker := range makers {
		if decimal.Cmp(result.Taker.RemainingQuantity, "0") <= 0 {
			break
		}
		if !eligibleMaker(result.Taker, maker) {
			if isSelfTrade(result.Taker, maker) && crossesPrice(result.Taker, maker) {
				result.Taker.Status = order.StatusExpired
				result.Taker.UpdatedAt = now
				break
			}
			continue
		}

		qty := decimal.Min(result.Taker.RemainingQuantity, maker.RemainingQuantity)
		if decimal.Cmp(qty, "0") <= 0 {
			continue
		}

		quoteQuantity := decimal.Mul(qty, maker.Price)
		if decimal.Cmp(quoteQuantity, "0") <= 0 {
			return Result{}, fmt.Errorf("%w: quote quantity is below supported precision", ErrInvalidMatch)
		}

		tradeTime := now.Add(time.Duration(len(result.Trades)) * time.Microsecond)
		item := trade.Trade{
			ID:            newTradeID(),
			Market:        result.Taker.Market,
			BaseAsset:     result.Taker.BaseAsset,
			QuoteAsset:    result.Taker.QuoteAsset,
			MakerOrderID:  maker.ID,
			TakerOrderID:  result.Taker.ID,
			MakerUserID:   maker.UserID,
			TakerUserID:   result.Taker.UserID,
			TakerSide:     result.Taker.Side,
			Price:         maker.Price,
			Quantity:      qty,
			QuoteQuantity: quoteQuantity,
			CreatedAt:     tradeTime,
		}
		result.Trades = append(result.Trades, item)

		maker.FilledQuantity = decimal.Add(maker.FilledQuantity, qty)
		maker.RemainingQuantity = decimal.SubFloorZero(maker.RemainingQuantity, qty)
		maker.Status = statusForRemaining(maker.RemainingQuantity)
		maker.UpdatedAt = now
		result.Makers = append(result.Makers, maker)

		result.Taker.FilledQuantity = decimal.Add(result.Taker.FilledQuantity, qty)
		result.Taker.RemainingQuantity = decimal.SubFloorZero(result.Taker.RemainingQuantity, qty)
		result.Taker.Status = statusForRemaining(result.Taker.RemainingQuantity)
		result.Taker.UpdatedAt = now
	}
	if decimal.Cmp(result.Taker.RemainingQuantity, "0") > 0 && result.Taker.Status != order.StatusPartiallyFilled && result.Taker.Status != order.StatusExpired {
		result.Taker.Status = order.StatusOpen
	}
	return result, nil
}

func validateTaker(item order.Order) error {
	if item.ID == "" {
		return fmt.Errorf("%w: taker id is required", ErrInvalidMatch)
	}
	if item.Market == "" || item.BaseAsset == "" || item.QuoteAsset == "" {
		return fmt.Errorf("%w: taker market is incomplete", ErrInvalidMatch)
	}
	if item.Side != order.SideBuy && item.Side != order.SideSell {
		return fmt.Errorf("%w: taker side is invalid", ErrInvalidMatch)
	}
	if decimal.Cmp(item.Price, "0") <= 0 {
		return fmt.Errorf("%w: taker price must be positive", ErrInvalidMatch)
	}
	if decimal.Cmp(item.RemainingQuantity, "0") <= 0 {
		return fmt.Errorf("%w: taker remaining quantity must be positive", ErrInvalidMatch)
	}
	return nil
}

func eligibleMaker(taker order.Order, maker order.Order) bool {
	if maker.ID == "" || maker.ID == taker.ID {
		return false
	}
	if maker.Market != taker.Market || maker.Side == taker.Side {
		return false
	}
	if isSelfTrade(taker, maker) {
		return false
	}
	if maker.Status != order.StatusOpen && maker.Status != order.StatusPartiallyFilled {
		return false
	}
	if decimal.Cmp(maker.RemainingQuantity, "0") <= 0 {
		return false
	}
	switch taker.Side {
	case order.SideBuy:
		return maker.Side == order.SideSell && decimal.Cmp(maker.Price, taker.Price) <= 0
	case order.SideSell:
		return maker.Side == order.SideBuy && decimal.Cmp(maker.Price, taker.Price) >= 0
	default:
		return false
	}
}

func isSelfTrade(taker order.Order, maker order.Order) bool {
	return taker.UserID != "" && taker.UserID == maker.UserID
}

func crossesPrice(taker order.Order, maker order.Order) bool {
	if maker.ID == "" || maker.ID == taker.ID {
		return false
	}
	if maker.Market != taker.Market || maker.Side == taker.Side {
		return false
	}
	if maker.Status != order.StatusOpen && maker.Status != order.StatusPartiallyFilled {
		return false
	}
	if decimal.Cmp(maker.RemainingQuantity, "0") <= 0 {
		return false
	}
	switch taker.Side {
	case order.SideBuy:
		return maker.Side == order.SideSell && decimal.Cmp(maker.Price, taker.Price) <= 0
	case order.SideSell:
		return maker.Side == order.SideBuy && decimal.Cmp(maker.Price, taker.Price) >= 0
	default:
		return false
	}
}

func statusForRemaining(remaining string) order.Status {
	if decimal.Cmp(remaining, "0") <= 0 {
		return order.StatusFilled
	}
	return order.StatusPartiallyFilled
}
