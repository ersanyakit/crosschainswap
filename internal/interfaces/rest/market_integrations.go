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

func (s *Server) binanceTime(c fiber.Ctx) error {
	return okJSON(c, fiber.Map{"serverTime": time.Now().UTC().UnixMilli()})
}

func (s *Server) binanceExchangeInfo(c fiber.Ctx) error {
	markets, err := s.orders.MarketSummaries(c.Context())
	if err != nil {
		return orderError(c, err)
	}
	symbols := make([]fiber.Map, 0, len(markets))
	for _, item := range markets {
		symbols = append(symbols, fiber.Map{
			"symbol":                     binanceSymbol(item.Symbol),
			"status":                     "TRADING",
			"baseAsset":                  item.BaseAsset,
			"quoteAsset":                 item.QuoteAsset,
			"baseAssetPrecision":         18,
			"quotePrecision":             18,
			"quoteAssetPrecision":        18,
			"orderTypes":                 []string{"LIMIT", "MARKET", "STOP_LOSS_LIMIT"},
			"icebergAllowed":             false,
			"ocoAllowed":                 false,
			"quoteOrderQtyMarketAllowed": false,
			"isSpotTradingAllowed":       true,
			"isMarginTradingAllowed":     false,
			"permissions":                []string{"SPOT"},
			"filters": []fiber.Map{
				{"filterType": "PRICE_FILTER", "minPrice": "0.000000000000000001", "maxPrice": "1000000000", "tickSize": "0.000000000000000001"},
				{"filterType": "LOT_SIZE", "minQty": "0.000000000000000001", "maxQty": "1000000000000000000", "stepSize": "0.000000000000000001"},
				{"filterType": "MIN_NOTIONAL", "minNotional": "0", "applyToMarket": true, "avgPriceMins": 0},
			},
		})
	}
	return okJSON(c, fiber.Map{
		"timezone":   "UTC",
		"serverTime": time.Now().UTC().UnixMilli(),
		"symbols":    symbols,
	})
}

func (s *Server) binanceTicker24h(c fiber.Ctx) error {
	items, err := s.publicTickers(c)
	if err != nil {
		return err
	}
	out := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		out = append(out, fiber.Map{
			"symbol":             binanceSymbol(item.market.Symbol),
			"priceChangePercent": item.market.Change24h,
			"lastPrice":          item.market.LastPrice,
			"lastQty":            "0",
			"bidPrice":           item.highestBid,
			"bidQty":             firstLevelQuantity(item.book.Bids),
			"askPrice":           item.lowestAsk,
			"askQty":             firstLevelQuantity(item.book.Asks),
			"openPrice":          openPriceFromChange(item.market.LastPrice, item.market.Change24h),
			"highPrice":          item.market.High24h,
			"lowPrice":           item.market.Low24h,
			"volume":             item.market.Volume24h,
			"quoteVolume":        quoteVolume(item.market.Volume24h, item.market.LastPrice),
			"openTime":           time.Now().Add(-24 * time.Hour).UTC().UnixMilli(),
			"closeTime":          time.Now().UTC().UnixMilli(),
			"count":              0,
		})
	}
	if c.Query("symbol") != "" {
		if len(out) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: "symbol not found"})
		}
		return okJSON(c, out[0])
	}
	return okJSON(c, out)
}

func (s *Server) binanceDepth(c fiber.Ctx) error {
	market, err := s.resolveMarketSymbol(c, c.Query("symbol"))
	if err != nil {
		return err
	}
	depth, err := queryInt(c, "limit", 100)
	if err != nil {
		return err
	}
	book, err := s.orders.Book(c.Context(), market, depth)
	if err != nil {
		return orderError(c, err)
	}
	return okJSON(c, fiber.Map{
		"lastUpdateId": time.Now().UTC().UnixMilli(),
		"bids":         priceLevelsToArrays(book.Bids),
		"asks":         priceLevelsToArrays(book.Asks),
	})
}

func (s *Server) binanceTrades(c fiber.Ctx) error {
	market, err := s.resolveMarketSymbol(c, c.Query("symbol"))
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
	for i, item := range items {
		out = append(out, fiber.Map{
			"id":           i,
			"price":        item.Price,
			"qty":          item.Quantity,
			"quoteQty":     item.QuoteQuantity,
			"time":         item.CreatedAt.UTC().UnixMilli(),
			"isBuyerMaker": item.TakerSide == "sell",
			"isBestMatch":  true,
		})
	}
	return okJSON(c, out)
}

func (s *Server) binanceKlines(c fiber.Ctx) error {
	market, err := s.resolveMarketSymbol(c, c.Query("symbol"))
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 500)
	if err != nil {
		return err
	}
	candles, err := s.orders.Candles(c.Context(), apporders.MarketHistoryRequest{
		Market:   market,
		Interval: c.Query("interval", "1m"),
		Limit:    limit,
	})
	if err != nil {
		return orderError(c, err)
	}
	out := make([][]any, 0, len(candles))
	for _, item := range candles {
		out = append(out, []any{
			item.OpenTime.UTC().UnixMilli(),
			item.Open,
			item.High,
			item.Low,
			item.Close,
			item.VolumeBase,
			item.CloseTime.UTC().UnixMilli(),
			item.VolumeQuote,
			item.TradeCount,
			"0",
			"0",
			"0",
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

func binanceSymbol(market string) string {
	return strings.ReplaceAll(externalPairID(market), "_", "")
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

func firstLevelQuantity(levels []orderbook.PriceLevel) string {
	if len(levels) == 0 {
		return "0"
	}
	return levels[0].Quantity
}

func quoteVolume(baseVolume string, lastPrice string) string {
	return decimal.Mul(baseVolume, lastPrice)
}

func openPriceFromChange(lastPrice string, changePercent string) string {
	return lastPrice
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
