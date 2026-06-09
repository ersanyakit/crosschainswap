package config

import "testing"

func TestDefaultRegistriesCreateUSDMarketsForEveryAsset(t *testing.T) {
	registries := LoadDefaultRegistries()
	assets := registries.Assets.All()
	if len(assets) != 13 {
		t.Fatalf("expected 13 default assets, got %d", len(assets))
	}

	for _, asset := range assets {
		market, ok := registries.Markets.Get(asset.Symbol + "/USD")
		if !ok {
			t.Fatalf("missing USD market for asset %s", asset.Symbol)
		}
		if market.BaseAsset != asset.Symbol || market.QuoteAsset != "USD" || !market.Enabled {
			t.Fatalf("unexpected market for asset %s: %#v", asset.Symbol, market)
		}
		if len(market.ChainKeys) != 0 {
			t.Fatalf("internal limit market must not carry DEX chain metadata: %#v", market)
		}
	}
}

func TestDefaultRegistriesIncludeGatewayFallbackAssets(t *testing.T) {
	registries := LoadDefaultRegistries()
	expected := []string{
		"AVAX", "BNB", "BTC", "CHZ", "CHZINU", "ETH", "LGBT", "PEPPER", "SOL", "TBT", "TRX", "USDC", "USDT",
	}

	for _, symbol := range expected {
		item, ok := registries.Assets.Get(symbol)
		if !ok {
			t.Fatalf("missing default asset %s", symbol)
		}
		if _, ok := registries.Markets.Get(symbol + "/USD"); !ok {
			t.Fatalf("missing default market %s/USD", symbol)
		}
		if len(item.Deployments) == 0 {
			t.Fatalf("asset %s has no deployments", symbol)
		}
		for _, deployment := range item.Deployments {
			if !deployment.Enabled {
				t.Fatalf("asset %s has disabled fallback deployment: %#v", symbol, deployment)
			}
		}
	}
}
