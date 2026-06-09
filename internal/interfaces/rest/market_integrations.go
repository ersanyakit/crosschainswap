package rest

import (
	"fmt"
	"strings"
	"time"

	apporders "exchange/internal/app/orders"
	"exchange/internal/core/decimal"
	"exchange/internal/core/orderbook"

	"github.com/gofiber/fiber/v3"
)

func (s *Server) coinMarketCapSummary(c fiber.Ctx) error {
	items, err := s.publicTickers(c)
	if err != nil {
		return err
	}
	out := make(map[string]fiber.Map, len(items))
	for _, item := range items {
		pair := externalPairID(item.market.Symbol)
		out[pair] = fiber.Map{
			"trading_pairs":            pair,
			"base_currency":            item.market.BaseAsset,
			"quote_currency":           item.market.QuoteAsset,
			"last_price":               item.market.LastPrice,
			"lowest_ask":               item.lowestAsk,
			"highest_bid":              item.highestBid,
			"base_volume":              item.market.Volume24h,
			"quote_volume":             quoteVolume(item.market.Volume24h, item.market.LastPrice),
			"price_change_percent_24h": item.market.Change24h,
			"highest_price_24h":        item.market.High24h,
			"lowest_price_24h":         item.market.Low24h,
			"type":                     "spot",
		}
	}
	return okJSON(c, out)
}

func (s *Server) coinMarketCapAssets(c fiber.Ctx) error {
	assets := s.prices.Assets()
	out := make(map[string]fiber.Map, len(assets))
	for _, item := range assets {
		out[item.Symbol] = fiber.Map{
			"name":                   firstNonEmpty(item.Name, item.Symbol),
			"unified_cryptoasset_id": 0,
			"can_withdraw":           true,
			"can_deposit":            true,
			"min_withdraw":           "0",
			"max_withdraw":           "0",
		}
	}
	return okJSON(c, out)
}

func (s *Server) coinGeckoPairs(c fiber.Ctx) error {
	markets, err := s.orders.MarketSummaries(c.Context())
	if err != nil {
		return orderError(c, err)
	}
	out := make([]fiber.Map, 0, len(markets))
	for _, item := range markets {
		out = append(out, fiber.Map{
			"ticker_id":       externalPairID(item.Symbol),
			"base":            item.BaseAsset,
			"target":          item.QuoteAsset,
			"base_currency":   item.BaseAsset,
			"target_currency": item.QuoteAsset,
		})
	}
	return okJSON(c, out)
}

func (s *Server) coinGeckoTickers(c fiber.Ctx) error {
	items, err := s.publicTickers(c)
	if err != nil {
		return err
	}
	out := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		out = append(out, fiber.Map{
			"ticker_id":       externalPairID(item.market.Symbol),
			"base_currency":   item.market.BaseAsset,
			"target_currency": item.market.QuoteAsset,
			"last_price":      item.market.LastPrice,
			"base_volume":     item.market.Volume24h,
			"target_volume":   quoteVolume(item.market.Volume24h, item.market.LastPrice),
			"bid":             item.highestBid,
			"ask":             item.lowestAsk,
			"high":            item.market.High24h,
			"low":             item.market.Low24h,
			"type":            "spot",
		})
	}
	return okJSON(c, out)
}

func (s *Server) publicOrderBook(c fiber.Ctx) error {
	market, err := s.resolveMarketSymbol(c, firstNonEmpty(c.Query("market_pair"), c.Query("ticker_id"), c.Query("symbol"), marketParam(c)))
	if err != nil {
		return err
	}
	depth, err := queryInt(c, "depth", 100)
	if err != nil {
		return err
	}
	book, err := s.orders.Book(c.Context(), market, depth)
	if err != nil {
		return orderError(c, err)
	}
	return okJSON(c, fiber.Map{
		"ticker_id": externalPairID(book.Market),
		"timestamp": time.Now().UTC().UnixMilli(),
		"bids":      priceLevelsToArrays(book.Bids),
		"asks":      priceLevelsToArrays(book.Asks),
	})
}

func (s *Server) publicTrades(c fiber.Ctx) error {
	market, err := s.resolveMarketSymbol(c, firstNonEmpty(c.Query("market_pair"), c.Query("ticker_id"), c.Query("symbol"), marketParam(c)))
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 100)
	if err != nil {
		return err
	}
	items, err := s.orders.MarketTrades(c.Context(), apporders.MarketHistoryRequest{Market: market, Limit: limit})
	if err != nil {
		return orderError(c, err)
	}
	out := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		side := "buy"
		if item.TakerSide == "sell" {
			side = "sell"
		}
		out = append(out, fiber.Map{
			"trade_id":     string(item.ID),
			"price":        item.Price,
			"base_volume":  item.Quantity,
			"quote_volume": item.QuoteQuantity,
			"timestamp":    item.CreatedAt.UTC().UnixMilli(),
			"type":         side,
		})
	}
	return okJSON(c, out)
}

type publicTicker struct {
	market     apporders.MarketSummary
	book       *orderbook.Snapshot
	highestBid string
	lowestAsk  string
}

func (s *Server) publicTickers(c fiber.Ctx) ([]publicTicker, error) {
	markets, err := s.orders.MarketSummaries(c.Context())
	if err != nil {
		return nil, orderError(c, err)
	}
	filter := strings.TrimSpace(firstNonEmpty(c.Query("symbol"), c.Query("market_pair"), c.Query("ticker_id")))
	out := make([]publicTicker, 0, len(markets))
	for _, item := range markets {
		if filter != "" && !externalMarketMatches(filter, item.Symbol) {
			continue
		}
		book, err := s.orders.Book(c.Context(), item.Symbol, 1)
		if err != nil {
			return nil, orderError(c, err)
		}
		out = append(out, publicTicker{
			market:     item,
			book:       book,
			highestBid: firstLevelPrice(book.Bids),
			lowestAsk:  firstLevelPrice(book.Asks),
		})
	}
	return out, nil
}

func (s *Server) resolveMarketSymbol(c fiber.Ctx, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: "symbol or market_pair is required"})
	}
	markets, err := s.orders.MarketSummaries(c.Context())
	if err != nil {
		return "", orderError(c, err)
	}
	for _, item := range markets {
		if externalMarketMatches(raw, item.Symbol) {
			return item.Symbol, nil
		}
	}
	return "", c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: fmt.Sprintf("market %s is not registered", raw)})
}

func externalMarketMatches(raw string, market string) bool {
	rawKey := compactMarketID(raw)
	return rawKey == compactMarketID(market) || rawKey == compactMarketID(externalPairID(market))
}

func compactMarketID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	replacer := strings.NewReplacer("/", "", "_", "", "-", "")
	return replacer.Replace(value)
}

func externalPairID(market string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(market)), "/", "_")
}

func priceLevelsToArrays(levels []orderbook.PriceLevel) [][]string {
	out := make([][]string, 0, len(levels))
	for _, level := range levels {
		out = append(out, []string{level.Price, level.Quantity})
	}
	return out
}

func firstLevelPrice(levels []orderbook.PriceLevel) string {
	if len(levels) == 0 {
		return "0"
	}
	return levels[0].Price
}

func quoteVolume(baseVolume string, lastPrice string) string {
	return decimal.Mul(baseVolume, lastPrice)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
