package config

import "testing"

func TestDefaultRegistriesCreateUSDMarketsForEveryAsset(t *testing.T) {
	registries := LoadDefaultRegistries()
	assets := registries.Assets.All()
	if len(assets) == 0 {
		t.Fatal("expected default assets")
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
