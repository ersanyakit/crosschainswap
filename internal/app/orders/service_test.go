package orders

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"exchange/internal/core/asset"
	"exchange/internal/core/balance"
	"exchange/internal/core/chain"
	"exchange/internal/core/decimal"
	"exchange/internal/core/market"
	"exchange/internal/core/matching"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
	"exchange/internal/core/trade"
)

func TestBuildOrderValidatesAndNormalizes(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "PEPPER/USDC", BaseAsset: "PEPPER", QuoteAsset: "USDC", Enabled: true},
	}), nil)

	item, err := service.buildOrder(PlaceRequest{
		ClientOrderID: "client-1",
		UserID:        "u1",
		Market:        "pepper/usdc",
		Side:          "BUY",
		Type:          "limit",
		Price:         "0.25",
		Quantity:      "10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Market != "PEPPER/USDC" || item.Side != order.SideBuy || item.Status != order.StatusOpen {
		t.Fatalf("unexpected order: %#v", item)
	}
	if item.Price != "0.25" || item.Quantity != "10" || item.RemainingQuantity != "10" {
		t.Fatalf("unexpected decimal normalization: %#v", item)
	}
}

func TestMarketSummariesReturnUSDDefaultsWithoutRepository(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "PEPPER/USD", BaseAsset: "PEPPER", QuoteAsset: "USD", Enabled: true},
	}), nil)

	summaries, err := service.MarketSummaries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].Symbol != "PEPPER/USD" || summaries[0].QuoteAsset != "USD" || summaries[0].LastPrice != "0.000000001" {
		t.Fatalf("unexpected summary: %#v", summaries[0])
	}
}

func TestCandlesReturnFallbackWithoutRepository(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "PEPPER/USD", BaseAsset: "PEPPER", QuoteAsset: "USD", Enabled: true},
	}), nil)

	candles, err := service.Candles(t.Context(), MarketHistoryRequest{
		Market:   "PEPPER/USD",
		Interval: "1m",
		Limit:    5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 5 {
		t.Fatalf("expected 5 fallback candles, got %d", len(candles))
	}
	last := candles[len(candles)-1]
	if last.Market != "PEPPER/USD" || last.Interval != "1m" || last.Close != "0.000000001" {
		t.Fatalf("unexpected fallback candle: %#v", last)
	}
}

func TestGatewayDepositAmountFromRaw(t *testing.T) {
	got, err := gatewayDepositAmount(GatewayDepositCallback{AmountRaw: "100250000", Decimals: 6})
	if err != nil {
		t.Fatal(err)
	}
	if got != "100.25" {
		t.Fatalf("gatewayDepositAmount raw = %q, want 100.25", got)
	}
}

func TestGatewayDepositSymbolUsesDeploymentSymbolForSelectedChain(t *testing.T) {
	service := NewService(market.Registry{}, nil)
	service.SetAssetRegistry(asset.NewRegistry([]asset.Asset{
		{
			Symbol: "BTC",
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKey("bitcoin"), Symbol: "BTC", Native: true, Enabled: true},
				{ChainKey: chain.ChainKeyEthereum, Symbol: "WBTC", Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Enabled: true},
			},
		},
	}))

	if got := service.gatewayDepositSymbol("BTC", "ethereum"); got != "WBTC" {
		t.Fatalf("gatewayDepositSymbol(BTC, ethereum) = %q, want WBTC", got)
	}
	if got := service.gatewayDepositSymbol("BTC", "bitcoin"); got != "BTC" {
		t.Fatalf("gatewayDepositSymbol(BTC, bitcoin) = %q, want BTC", got)
	}
}

func TestGatewayDepositAssetCanonicalizesDeploymentSymbol(t *testing.T) {
	service := NewService(market.Registry{}, nil)
	service.SetAssetRegistry(asset.NewRegistry([]asset.Asset{
		{
			Symbol: "BTC",
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyEthereum, Symbol: "WBTC", Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Enabled: true},
			},
		},
	}))

	got := service.gatewayDepositAsset(GatewayDepositCallback{Asset: "WBTC"})
	if got != "BTC" {
		t.Fatalf("gatewayDepositAsset(WBTC) = %q, want BTC", got)
	}
}

func TestValidGatewayAssetSymbolAllowsUnlistedDepositAssets(t *testing.T) {
	for _, asset := range []string{"BTC", "USDT", "USDC.E", "WBTC-OLD", "TOKEN_1"} {
		if !validGatewayAssetSymbol(asset) {
			t.Fatalf("expected gateway asset %q to be accepted", asset)
		}
	}
}

func TestValidGatewayAssetSymbolRejectsUnsafeValues(t *testing.T) {
	for _, asset := range []string{"", "btc", "BTC/USD", "BTC ETH", "BTC;DROP", strings.Repeat("A", 33)} {
		if validGatewayAssetSymbol(asset) {
			t.Fatalf("expected gateway asset %q to be rejected", asset)
		}
	}
}

func TestGatewayStatusNormalization(t *testing.T) {
	cases := map[string]string{
		"awaiting_payment": "pending",
		"paid":             "settled",
		"completed":        "settled",
		"failed":           "canceled",
	}
	for input, want := range cases {
		if got := normalizeGatewayStatus(input); got != want {
			t.Fatalf("normalizeGatewayStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGatewayBalanceEventIDIsDeterministicAndShort(t *testing.T) {
	first := gatewayBalanceEventID("gwdep_s", "payment-1")
	second := gatewayBalanceEventID("gwdep_s", "payment-1")
	if first != second {
		t.Fatalf("gatewayBalanceEventID is not deterministic: %q != %q", first, second)
	}
	if len(first) > 64 {
		t.Fatalf("gatewayBalanceEventID length = %d, want <= 64", len(first))
	}
}

func TestKnownChainAllowsGatewayWalletsWhenMarketsHaveNoChainMetadata(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "SOL/USD", BaseAsset: "SOL", QuoteAsset: "USD", Enabled: true},
	}), nil)

	if !service.knownChain("solana") {
		t.Fatal("knownChain rejected a gateway wallet chain when markets have no chain metadata")
	}
}

func TestKnownChainUsesConfiguredChainMetadataWhenPresent(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "SOL/USD", BaseAsset: "SOL", QuoteAsset: "USD", ChainKeys: []string{"solana"}, Enabled: true},
	}), nil)

	if !service.knownChain("solana") {
		t.Fatal("knownChain rejected configured chain")
	}
	if service.knownChain("base") {
		t.Fatal("knownChain accepted unconfigured chain")
	}
}

func TestBuildMarketOrderDefaultsToIOC(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "PEPPER/USDC", BaseAsset: "PEPPER", QuoteAsset: "USDC", Enabled: true},
	}), nil)

	item, err := service.buildOrder(PlaceRequest{
		ClientOrderID: "client-market-1",
		UserID:        "u1",
		Market:        "PEPPER/USDC",
		Side:          "buy",
		Type:          "market",
		Price:         "0.3",
		Quantity:      "10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Type != order.TypeMarket || item.TimeInForce != order.TimeInForceIOC || item.Status != order.StatusOpen {
		t.Fatalf("unexpected market order: %#v", item)
	}
}

func TestBuildMarketOrderRejectsGTC(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "PEPPER/USDC", BaseAsset: "PEPPER", QuoteAsset: "USDC", Enabled: true},
	}), nil)

	_, err := service.buildOrder(PlaceRequest{
		ClientOrderID: "client-market-gtc",
		UserID:        "u1",
		Market:        "PEPPER/USDC",
		Side:          "buy",
		Type:          "market",
		TimeInForce:   "gtc",
		Price:         "0.3",
		Quantity:      "10",
	})
	if err == nil {
		t.Fatalf("expected market gtc to be rejected")
	}
}

func TestEffectiveMatchTakerSweepsMarketSellBook(t *testing.T) {
	item := order.Order{
		ID:     "sell-market",
		Side:   order.SideSell,
		Type:   order.TypeMarket,
		Price:  "0.990421",
		Market: "USDC/USD",
	}

	got := effectiveMatchTaker(item)
	if got.Price != minPositiveMarketPrice {
		t.Fatalf("market sell effective price = %s, want %s", got.Price, minPositiveMarketPrice)
	}
	if item.Price != "0.990421" {
		t.Fatalf("effectiveMatchTaker mutated original order price: %s", item.Price)
	}
}

func TestEffectiveMatchTakerKeepsMarketBuyProtectionPrice(t *testing.T) {
	item := order.Order{
		ID:     "buy-market",
		Side:   order.SideBuy,
		Type:   order.TypeMarket,
		Price:  "1.010009",
		Market: "USDC/USD",
	}

	got := effectiveMatchTaker(item)
	if got.Price != item.Price {
		t.Fatalf("market buy effective price = %s, want submitted protection %s", got.Price, item.Price)
	}
}

func TestEffectiveMarketSellMatchesBidsBelowSubmittedProtection(t *testing.T) {
	item := order.Order{
		ID:                "sell-market",
		UserID:            "seller",
		Market:            "USDC/USD",
		BaseAsset:         "USDC",
		QuoteAsset:        "USD",
		Side:              order.SideSell,
		Type:              order.TypeMarket,
		Status:            order.StatusOpen,
		Price:             "0.990421",
		Quantity:          "10",
		FilledQuantity:    "0",
		RemainingQuantity: "10",
	}
	makers := []order.Order{
		{
			ID:                "bid-below-protection",
			UserID:            "buyer",
			Market:            "USDC/USD",
			BaseAsset:         "USDC",
			QuoteAsset:        "USD",
			Side:              order.SideBuy,
			Status:            order.StatusOpen,
			Price:             "0.5",
			Quantity:          "10",
			FilledQuantity:    "0",
			RemainingQuantity: "10",
		},
	}

	result, err := matching.MatchLimit(effectiveMatchTaker(item), makers, func() trade.ID { return "trd" }, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 1 || result.Trades[0].Price != "0.5" || result.Trades[0].Quantity != "10" {
		t.Fatalf("market sell should sweep bid below submitted protection, got %#v", result.Trades)
	}
	if result.Taker.Status != order.StatusFilled || result.Makers[0].Status != order.StatusFilled {
		t.Fatalf("unexpected statuses: taker=%#v maker=%#v", result.Taker, result.Makers[0])
	}
}

func TestBuildOrderRejectsBelowSupportedNotional(t *testing.T) {
	service := NewService(market.NewRegistry([]market.Market{
		{Symbol: "PEPPER/USDC", BaseAsset: "PEPPER", QuoteAsset: "USDC", Enabled: true},
	}), nil)

	_, err := service.buildOrder(PlaceRequest{
		ClientOrderID: "client-dust-1",
		UserID:        "u1",
		Market:        "PEPPER/USDC",
		Side:          "buy",
		Type:          "limit",
		TimeInForce:   "gtc",
		Price:         "0.000000000000000001",
		Quantity:      "0.000000000000000001",
	})
	if err == nil {
		t.Fatalf("expected below precision notional to be rejected")
	}
}

func TestPublishOrderUpdateEmitsFilledAndTrades(t *testing.T) {
	var payloads [][]byte
	service := NewService(market.Registry{}, nil)
	service.SetPublisher(func(payload []byte) {
		payloads = append(payloads, payload)
	})

	service.publishOrderUpdate("exchange.order_accepted", order.Order{
		ID:     "ord_1",
		UserID: "u1",
		Market: "PEPPER/USDC",
		Status: order.StatusFilled,
	}, nil)

	if len(payloads) != 2 {
		t.Fatalf("expected order and book events, got %d", len(payloads))
	}
	var event map[string]any
	if err := json.Unmarshal(payloads[0], &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "exchange.order_filled" {
		t.Fatalf("unexpected event type: %#v", event["type"])
	}
}

func TestMatchEventsIncludeFilledMakerTerminalEvent(t *testing.T) {
	taker := order.Order{ID: "taker", UserID: "buyer", Market: "PEPPER/USDC", Status: order.StatusFilled}
	maker := order.Order{ID: "maker", UserID: "seller", Market: "PEPPER/USDC", Status: order.StatusFilled}
	trades := []trade.Trade{{
		ID:           "trd-1",
		Market:       "PEPPER/USDC",
		MakerOrderID: maker.ID,
		TakerOrderID: taker.ID,
		MakerUserID:  maker.UserID,
		TakerUserID:  taker.UserID,
		CreatedAt:    time.Unix(1, 0).UTC(),
	}}

	events := matchEvents(taker, []order.Order{maker}, trades)

	var makerFilled bool
	var takerFilled bool
	for _, event := range events {
		if event.OrderID == maker.ID && event.Type == order.EventOrderFilled {
			makerFilled = true
		}
		if event.OrderID == taker.ID && event.Type == order.EventOrderFilled {
			takerFilled = true
		}
	}
	if !makerFilled || !takerFilled {
		t.Fatalf("missing filled events: maker=%t taker=%t events=%#v", makerFilled, takerFilled, events)
	}
}

func TestPublishOrderUpdateEmitsExpired(t *testing.T) {
	var payloads [][]byte
	service := NewService(market.Registry{}, nil)
	service.SetPublisher(func(payload []byte) {
		payloads = append(payloads, payload)
	})

	service.publishOrderUpdate("exchange.order_accepted", order.Order{
		ID:     "ord_1",
		UserID: "u1",
		Market: "PEPPER/USDC",
		Status: order.StatusExpired,
	}, nil)

	if len(payloads) != 2 {
		t.Fatalf("expected order and book events, got %d", len(payloads))
	}
	var event map[string]any
	if err := json.Unmarshal(payloads[0], &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "exchange.order_expired" {
		t.Fatalf("unexpected event type: %#v", event["type"])
	}
}

func TestPublishOrderBookDeltaEmitsVersionedLevels(t *testing.T) {
	var payloads [][]byte
	service := NewService(market.Registry{}, nil)
	service.SetPublisher(func(payload []byte) {
		payloads = append(payloads, payload)
	})

	service.publishOrderBookDelta(bookDelta{
		Market:  "PEPPER/USDC",
		Version: 42,
		Levels: []orderbook.PriceLevel{
			{Market: "PEPPER/USDC", Side: order.SideBuy, Price: "1", Quantity: "5"},
			{Market: "PEPPER/USDC", Side: order.SideSell, Price: "2", Quantity: "0"},
		},
	})

	if len(payloads) != 1 {
		t.Fatalf("expected one event, got %d", len(payloads))
	}
	var event map[string]any
	if err := json.Unmarshal(payloads[0], &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "exchange.orderbook_delta" || event["version"].(float64) != 42 {
		t.Fatalf("unexpected delta event: %#v", event)
	}
	if len(event["bids"].([]any)) != 1 || len(event["asks"].([]any)) != 1 {
		t.Fatalf("unexpected bid/ask delta payload: %#v", event)
	}
}

func TestPublishBalanceUpdateEmitsDepositEvent(t *testing.T) {
	var payloads [][]byte
	service := NewService(market.Registry{}, nil)
	service.SetPublisher(func(payload []byte) {
		payloads = append(payloads, payload)
	})

	service.publishBalanceUpdate("exchange.deposit_settled", &balance.Balance{
		UserID:    "u1",
		Asset:     "USDC",
		Available: "100",
	})

	if len(payloads) != 1 {
		t.Fatalf("expected one event, got %d", len(payloads))
	}
	var event map[string]any
	if err := json.Unmarshal(payloads[0], &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "exchange.deposit_settled" {
		t.Fatalf("unexpected event type: %#v", event["type"])
	}
}

func TestStopTriggered(t *testing.T) {
	buy := order.Order{Side: order.SideBuy, StopPrice: "10"}
	if !stopTriggered(buy, "10") || !stopTriggered(buy, "10.1") || stopTriggered(buy, "9.9") {
		t.Fatalf("unexpected buy stop behavior")
	}

	sell := order.Order{Side: order.SideSell, StopPrice: "10"}
	if !stopTriggered(sell, "10") || !stopTriggered(sell, "9.9") || stopTriggered(sell, "10.1") {
		t.Fatalf("unexpected sell stop behavior")
	}
}

func TestStopPriceValidForLastPriceMatchesSpotStopLossLimitDirection(t *testing.T) {
	buy := order.Order{Type: order.TypeStopLimit, Side: order.SideBuy, StopPrice: "10"}
	if !stopPriceValidForLastPrice(buy, "9.99") {
		t.Fatalf("buy stop-limit should be valid above last price")
	}
	if stopPriceValidForLastPrice(buy, "10") || stopPriceValidForLastPrice(buy, "10.01") {
		t.Fatalf("buy stop-limit should reject stop prices at or below the current last price")
	}

	sell := order.Order{Type: order.TypeStopLimit, Side: order.SideSell, StopPrice: "10"}
	if !stopPriceValidForLastPrice(sell, "10.01") {
		t.Fatalf("sell stop-limit should be valid below last price")
	}
	if stopPriceValidForLastPrice(sell, "10") || stopPriceValidForLastPrice(sell, "9.99") {
		t.Fatalf("sell stop-limit should reject stop prices at or above the current last price")
	}
}

func TestDecimalMath(t *testing.T) {
	if decimal.Add("1.25", "2.75") != "4" {
		t.Fatalf("bad add")
	}
	if decimal.SubFloorZero("1", "2") != "0" {
		t.Fatalf("bad bounded sub")
	}
	if decimal.Mul("0.5", "3") != "1.5" {
		t.Fatalf("bad multiply")
	}
	if decimal.Cmp("1.01", "1.001") <= 0 {
		t.Fatalf("bad compare")
	}
}

func TestParsePositiveDecimalRejectsUnsupportedPrecision(t *testing.T) {
	if _, err := parsePositiveDecimal("0.0000000000000000001", "price"); err == nil {
		t.Fatalf("expected unsupported precision error")
	}
	if _, err := parsePositiveDecimal("1.0000000000000000001", "price"); err == nil {
		t.Fatalf("expected too many decimals error")
	}
}
