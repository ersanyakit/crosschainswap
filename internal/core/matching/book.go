package matching

import (
	"fmt"
	"sort"
	"time"

	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
	"exchange/internal/core/trade"
)

const bookMinPositiveMarketPrice = "0.000000000000000001"

type MarketBook struct {
	Market     string
	BaseAsset  string
	QuoteAsset string

	bids   bookSide
	asks   bookSide
	orders map[order.ID]bookOrderRef
}

type bookSide struct {
	side   order.Side
	prices []string
	levels map[string][]order.Order
}

type bookOrderRef struct {
	side  order.Side
	price string
}

func NewMarketBook(market string, baseAsset string, quoteAsset string) *MarketBook {
	return &MarketBook{
		Market:     market,
		BaseAsset:  baseAsset,
		QuoteAsset: quoteAsset,
		bids:       newBookSide(order.SideBuy),
		asks:       newBookSide(order.SideSell),
		orders:     make(map[order.ID]bookOrderRef),
	}
}

func (b *MarketBook) Load(items []order.Order) error {
	for _, item := range items {
		item = b.normalizeOrder(item)
		if !isBookable(item) {
			continue
		}
		if err := b.insertResting(item); err != nil {
			return err
		}
	}
	return nil
}

func (b *MarketBook) Apply(taker order.Order, newTradeID TradeIDFactory, now time.Time) (Result, error) {
	if b == nil {
		return Result{}, fmt.Errorf("%w: market book is nil", ErrInvalidMatch)
	}
	if newTradeID == nil {
		return Result{}, fmt.Errorf("%w: trade id factory is required", ErrInvalidMatch)
	}

	taker = b.normalizeOrder(taker)
	if _, exists := b.orders[taker.ID]; exists {
		return Result{}, fmt.Errorf("%w: order is already active in book", ErrInvalidMatch)
	}
	matchTaker := effectiveBookTaker(taker)
	if err := validateTaker(matchTaker); err != nil {
		return Result{}, err
	}
	if err := b.validateMarket(taker); err != nil {
		return Result{}, err
	}
	if taker.Type == order.TypeMarket {
		taker.TimeInForce = order.TimeInForceIOC
	}

	result := Result{
		Taker:  taker,
		Makers: make([]order.Order, 0),
		Trades: make([]trade.Trade, 0),
	}
	makerSide := b.side(oppositeBookSide(result.Taker.Side))

	for decimal.Cmp(result.Taker.RemainingQuantity, "0") > 0 {
		price, ok := makerSide.bestPrice()
		if !ok {
			break
		}
		queue := makerSide.levels[price]
		if len(queue) == 0 {
			makerSide.removePrice(price)
			continue
		}

		maker := queue[0]
		if !eligibleMaker(effectiveBookTaker(result.Taker), maker) {
			if !isBookable(maker) || maker.Market != result.Taker.Market || maker.Side == result.Taker.Side {
				b.removeAt(makerSide, price, 0)
				continue
			}
			break
		}

		qty := decimal.Min(result.Taker.RemainingQuantity, maker.RemainingQuantity)
		if decimal.Cmp(qty, "0") <= 0 {
			b.removeAt(makerSide, price, 0)
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

		if maker.Status == order.StatusFilled {
			b.removeAt(makerSide, price, 0)
			continue
		}
		makerSide.levels[price][0] = maker
	}

	if decimal.Cmp(result.Taker.RemainingQuantity, "0") <= 0 {
		result.Taker.Status = order.StatusFilled
		result.Taker.UpdatedAt = now
		return result, nil
	}
	if isImmediateOnlyBookOrder(result.Taker) {
		result.Taker.Status = order.StatusExpired
		result.Taker.UpdatedAt = now
		return result, nil
	}
	if len(result.Trades) == 0 {
		result.Taker.Status = order.StatusOpen
	} else {
		result.Taker.Status = order.StatusPartiallyFilled
	}
	result.Taker.UpdatedAt = now
	if err := b.insertResting(result.Taker); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (b *MarketBook) ApplyResult(result Result) error {
	if b == nil {
		return fmt.Errorf("%w: market book is nil", ErrInvalidMatch)
	}
	for _, maker := range result.Makers {
		b.removeOrder(maker.ID)
		maker = b.normalizeOrder(maker)
		if isBookable(maker) {
			if err := b.insertResting(maker); err != nil {
				return err
			}
		}
	}
	if result.Taker.ID != "" {
		b.removeOrder(result.Taker.ID)
		taker := b.normalizeOrder(result.Taker)
		if isBookable(taker) {
			if err := b.insertResting(taker); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *MarketBook) Cancel(id order.ID, now time.Time) (order.Order, bool) {
	if b == nil || id == "" {
		return order.Order{}, false
	}
	ref, ok := b.orders[id]
	if !ok {
		return order.Order{}, false
	}
	side := b.side(ref.side)
	queue := side.levels[ref.price]
	for idx, item := range queue {
		if item.ID != id {
			continue
		}
		removed := b.removeAt(side, ref.price, idx)
		removed.Status = order.StatusCanceled
		removed.UpdatedAt = now
		return removed, true
	}
	delete(b.orders, id)
	return order.Order{}, false
}

func (b *MarketBook) ActiveOrder(id order.ID) (order.Order, bool) {
	if b == nil || id == "" {
		return order.Order{}, false
	}
	ref, ok := b.orders[id]
	if !ok {
		return order.Order{}, false
	}
	queue := b.side(ref.side).levels[ref.price]
	for _, item := range queue {
		if item.ID == id {
			return item, true
		}
	}
	return order.Order{}, false
}

func (b *MarketBook) ActiveOrderCount() int {
	if b == nil {
		return 0
	}
	return len(b.orders)
}

func (b *MarketBook) BestBid() (string, bool) {
	if b == nil {
		return "", false
	}
	return b.bids.bestPrice()
}

func (b *MarketBook) BestAsk() (string, bool) {
	if b == nil {
		return "", false
	}
	return b.asks.bestPrice()
}

func (b *MarketBook) Snapshot(depth int) orderbook.Snapshot {
	if b == nil {
		return orderbook.Snapshot{}
	}
	return orderbook.Snapshot{
		Market: b.Market,
		Bids:   b.snapshotSide(&b.bids, depth),
		Asks:   b.snapshotSide(&b.asks, depth),
	}
}

func (b *MarketBook) snapshotSide(side *bookSide, depth int) []orderbook.PriceLevel {
	if depth <= 0 || depth > len(side.prices) {
		depth = len(side.prices)
	}
	out := make([]orderbook.PriceLevel, 0, depth)
	for _, price := range side.prices {
		if len(out) >= depth {
			break
		}
		queue := side.levels[price]
		quantity := "0"
		var count int64
		var firstSequence uint64
		var lastUpdated time.Time
		for _, item := range queue {
			if !isBookable(item) {
				continue
			}
			quantity = decimal.Add(quantity, item.RemainingQuantity)
			count++
			if firstSequence == 0 || (item.SequenceID != 0 && item.SequenceID < firstSequence) {
				firstSequence = item.SequenceID
			}
			if item.UpdatedAt.After(lastUpdated) {
				lastUpdated = item.UpdatedAt
			}
		}
		if count == 0 || decimal.Cmp(quantity, "0") <= 0 {
			continue
		}
		out = append(out, orderbook.PriceLevel{
			Market:          b.Market,
			Side:            side.side,
			Price:           price,
			Quantity:        quantity,
			OrderCount:      count,
			FirstSequenceID: firstSequence,
			LastUpdatedAt:   lastUpdated,
		})
	}
	return out
}

func (b *MarketBook) insertResting(item order.Order) error {
	if b == nil {
		return fmt.Errorf("%w: market book is nil", ErrInvalidMatch)
	}
	item = b.normalizeOrder(item)
	if err := b.validateResting(item); err != nil {
		return err
	}
	if _, exists := b.orders[item.ID]; exists {
		return fmt.Errorf("%w: order is already active in book", ErrInvalidMatch)
	}

	side := b.side(item.Side)
	if _, ok := side.levels[item.Price]; !ok {
		side.insertPrice(item.Price)
	}
	queue := side.levels[item.Price]
	insertAt := len(queue)
	if item.SequenceID != 0 {
		insertAt = sort.Search(len(queue), func(i int) bool {
			return queue[i].SequenceID != 0 && queue[i].SequenceID > item.SequenceID
		})
	}
	queue = append(queue, order.Order{})
	copy(queue[insertAt+1:], queue[insertAt:])
	queue[insertAt] = item
	side.levels[item.Price] = queue
	b.orders[item.ID] = bookOrderRef{side: item.Side, price: item.Price}
	return nil
}

func (b *MarketBook) removeAt(side *bookSide, price string, idx int) order.Order {
	queue := side.levels[price]
	if idx < 0 || idx >= len(queue) {
		return order.Order{}
	}
	removed := queue[idx]
	delete(b.orders, removed.ID)
	queue = append(queue[:idx], queue[idx+1:]...)
	if len(queue) == 0 {
		delete(side.levels, price)
		side.removePrice(price)
		return removed
	}
	side.levels[price] = queue
	return removed
}

func (b *MarketBook) removeOrder(id order.ID) (order.Order, bool) {
	if b == nil || id == "" {
		return order.Order{}, false
	}
	ref, ok := b.orders[id]
	if !ok {
		return order.Order{}, false
	}
	side := b.side(ref.side)
	queue := side.levels[ref.price]
	for idx, item := range queue {
		if item.ID == id {
			return b.removeAt(side, ref.price, idx), true
		}
	}
	delete(b.orders, id)
	return order.Order{}, false
}

func (b *MarketBook) side(side order.Side) *bookSide {
	if side == order.SideBuy {
		return &b.bids
	}
	return &b.asks
}

func (b *MarketBook) normalizeOrder(item order.Order) order.Order {
	if item.Type == "" {
		item.Type = order.TypeLimit
	}
	if item.TimeInForce == "" {
		item.TimeInForce = order.TimeInForceGTC
	}
	if item.Status == "" {
		item.Status = order.StatusOpen
	}
	if item.FilledQuantity == "" {
		item.FilledQuantity = "0"
	}
	if item.RemainingQuantity == "" {
		item.RemainingQuantity = item.Quantity
	}
	if item.Quantity == "" {
		item.Quantity = item.RemainingQuantity
	}
	if item.Market == "" {
		item.Market = b.Market
	}
	if item.BaseAsset == "" {
		item.BaseAsset = b.BaseAsset
	}
	if item.QuoteAsset == "" {
		item.QuoteAsset = b.QuoteAsset
	}
	return item
}

func (b *MarketBook) validateResting(item order.Order) error {
	if err := b.validateMarket(item); err != nil {
		return err
	}
	if item.ID == "" {
		return fmt.Errorf("%w: order id is required", ErrInvalidMatch)
	}
	if item.Side != order.SideBuy && item.Side != order.SideSell {
		return fmt.Errorf("%w: resting order side is invalid", ErrInvalidMatch)
	}
	if item.Type == order.TypeMarket {
		return fmt.Errorf("%w: market orders cannot rest in book", ErrInvalidMatch)
	}
	if decimal.Cmp(item.Price, "0") <= 0 {
		return fmt.Errorf("%w: resting order price must be positive", ErrInvalidMatch)
	}
	if decimal.Cmp(item.RemainingQuantity, "0") <= 0 {
		return fmt.Errorf("%w: resting order remaining quantity must be positive", ErrInvalidMatch)
	}
	if item.Status != order.StatusOpen && item.Status != order.StatusPartiallyFilled {
		return fmt.Errorf("%w: resting order status is not active", ErrInvalidMatch)
	}
	return nil
}

func (b *MarketBook) validateMarket(item order.Order) error {
	if b.Market != "" && item.Market != b.Market {
		return fmt.Errorf("%w: order market %s does not match book market %s", ErrInvalidMatch, item.Market, b.Market)
	}
	if b.BaseAsset != "" && item.BaseAsset != "" && item.BaseAsset != b.BaseAsset {
		return fmt.Errorf("%w: order base asset %s does not match book base asset %s", ErrInvalidMatch, item.BaseAsset, b.BaseAsset)
	}
	if b.QuoteAsset != "" && item.QuoteAsset != "" && item.QuoteAsset != b.QuoteAsset {
		return fmt.Errorf("%w: order quote asset %s does not match book quote asset %s", ErrInvalidMatch, item.QuoteAsset, b.QuoteAsset)
	}
	return nil
}

func newBookSide(side order.Side) bookSide {
	return bookSide{side: side, levels: make(map[string][]order.Order)}
}

func (s *bookSide) bestPrice() (string, bool) {
	if s == nil || len(s.prices) == 0 {
		return "", false
	}
	return s.prices[0], true
}

func (s *bookSide) insertPrice(price string) {
	idx := sort.Search(len(s.prices), func(i int) bool {
		cmp := decimal.Cmp(s.prices[i], price)
		if s.side == order.SideBuy {
			return cmp < 0
		}
		return cmp > 0
	})
	s.prices = append(s.prices, "")
	copy(s.prices[idx+1:], s.prices[idx:])
	s.prices[idx] = price
}

func (s *bookSide) removePrice(price string) {
	for idx, existing := range s.prices {
		if existing != price {
			continue
		}
		s.prices = append(s.prices[:idx], s.prices[idx+1:]...)
		return
	}
}

func oppositeBookSide(side order.Side) order.Side {
	if side == order.SideBuy {
		return order.SideSell
	}
	return order.SideBuy
}

func effectiveBookTaker(item order.Order) order.Order {
	if item.Type == order.TypeMarket && item.Side == order.SideSell && decimal.Cmp(item.Price, "0") <= 0 {
		item.Price = bookMinPositiveMarketPrice
	}
	return item
}

func isBookable(item order.Order) bool {
	if item.Type == order.TypeMarket {
		return false
	}
	if item.Status != order.StatusOpen && item.Status != order.StatusPartiallyFilled {
		return false
	}
	return decimal.Cmp(item.RemainingQuantity, "0") > 0
}

func isImmediateOnlyBookOrder(item order.Order) bool {
	return item.Type == order.TypeMarket || item.TimeInForce == order.TimeInForceIOC
}
