package pricing

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	"exchange/internal/core/venue"
)

type memoryPoolStore struct {
	pools []venue.Pool
}

func (s memoryPoolStore) ListPoolsByAssetIDs(_ context.Context, _ []venue.AssetID) ([]venue.Pool, error) {
	return s.pools, nil
}

func TestPricesUsesOnlyRegisteredAssets(t *testing.T) {
	assets := asset.NewRegistry([]asset.Asset{
		{
			Symbol:   "PEPPER",
			Name:     "PEPPER",
			Type:     "token",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67", Decimals: 18},
			},
		},
		{
			Symbol:   "CHZ",
			Name:     "Wrapped Chiliz",
			Type:     "native",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0x677f7e16c7dd57be1d4c8ad1244883214953dc47", Decimals: 18},
			},
		},
	})
	service := NewService(assets, memoryPoolStore{pools: []venue.Pool{
		{
			ID:       "registered-pool",
			ChainKey: chain.ChainKeyChiliz,
			VenueKey: venue.VenueKeyKewlSwap,
			Kind:     venue.PoolKindV2,
			Token0:   "0x60F397ACBCFB8F4E3234C659A3E10867E6FA6B67",
			Token1:   "0x677f7e16c7dd57be1d4c8ad1244883214953dc47",
			Reserve0: scaled(100, 18),
			Reserve1: scaled(250, 18),
			Enabled:  true,
		},
		{
			ID:       "unknown-quote-pool",
			ChainKey: chain.ChainKeyChiliz,
			VenueKey: venue.VenueKeyKewlSwap,
			Kind:     venue.PoolKindV2,
			Token0:   "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67",
			Token1:   "0x0000000000000000000000000000000000000001",
			Reserve0: scaled(100, 18),
			Reserve1: scaled(100, 18),
			Enabled:  true,
		},
	}})

	result, err := service.Prices(context.Background(), "pepper")
	if err != nil {
		t.Fatal(err)
	}

	if result.Symbol != "PEPPER" {
		t.Fatalf("unexpected symbol: %s", result.Symbol)
	}
	if len(result.Prices) != 1 {
		t.Fatalf("expected only registered quote pool, got %d", len(result.Prices))
	}
	price := result.Prices[0]
	if price.Price != "2.5" || price.QuoteSymbol != "CHZ" {
		t.Fatalf("unexpected price: %#v", price)
	}
}

func TestPricesRejectsUnknownAsset(t *testing.T) {
	service := NewService(asset.NewRegistry(nil), memoryPoolStore{})

	_, err := service.Prices(context.Background(), "NOPE")
	if !errors.Is(err, ErrUnknownAsset) {
		t.Fatalf("expected ErrUnknownAsset, got %v", err)
	}
}

func TestPricesIncludesAssetInfoAndDerivedUSDC(t *testing.T) {
	assets := asset.NewRegistry([]asset.Asset{
		{
			Symbol:   "PEPPER",
			Name:     "PEPPER",
			Type:     "token",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0xpepper", Decimals: 18},
			},
		},
		{
			Symbol:   "CHZ",
			Name:     "Chiliz",
			Type:     "native",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0xchz", Symbol: "wCHZ", Name: "Wrapped Chiliz", Decimals: 18},
			},
		},
		{
			Symbol:   "USDC",
			Name:     "USD Coin",
			Type:     "token",
			Decimals: 6,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0xusdc", Decimals: 6},
			},
		},
	})
	service := NewService(assets, memoryPoolStore{pools: []venue.Pool{
		{
			ID:       "pepper-chz",
			ChainKey: chain.ChainKeyChiliz,
			VenueKey: venue.VenueKeyKayenSwap,
			Kind:     venue.PoolKindV2,
			Token0:   "0xpepper",
			Token1:   "0xchz",
			Reserve0: scaled(100, 18),
			Reserve1: scaled(250, 18),
			Enabled:  true,
		},
		{
			ID:       "chz-usdc",
			ChainKey: chain.ChainKeyChiliz,
			VenueKey: venue.VenueKeyKayenSwap,
			Kind:     venue.PoolKindV2,
			Token0:   "0xchz",
			Token1:   "0xusdc",
			Reserve0: scaled(2, 18),
			Reserve1: scaled(1, 6),
			Enabled:  true,
		},
	}})

	result, err := service.Prices(context.Background(), "PEPPER")
	if err != nil {
		t.Fatal(err)
	}
	if result.Asset.Name != "PEPPER" || result.Asset.Decimals != 18 || len(result.Asset.Deployments) != 1 {
		t.Fatalf("unexpected asset metadata: %#v", result.Asset)
	}
	if len(result.Prices) != 1 {
		t.Fatalf("expected one PEPPER price, got %d", len(result.Prices))
	}
	price := result.Prices[0]
	if price.Price != "2.5" || price.BaseUSDC != "1.25" || price.QuoteUSDC != "0.5" || price.PriceUSDC != "1.25" {
		t.Fatalf("unexpected derived price: %#v", price)
	}
	if price.QuoteAsset.Symbol != "wCHZ" || price.QuoteAsset.Decimals != 18 {
		t.Fatalf("unexpected quote metadata: %#v", price.QuoteAsset)
	}
}

func TestPricesDerivesUSDCThroughSolanaSOLPair(t *testing.T) {
	assets := asset.NewRegistry([]asset.Asset{
		{
			Symbol:   "PEPPER",
			Name:     "PEPPER",
			Type:     "token",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "pepper-mint", Decimals: 3},
			},
		},
		{
			Symbol:   "SOL",
			Name:     "Solana",
			Type:     "native",
			Decimals: 9,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "sol-mint", Symbol: "WSOL", Decimals: 9},
			},
		},
		{
			Symbol:   "USDC",
			Name:     "USD Coin",
			Type:     "token",
			Decimals: 6,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "usdc-mint", Decimals: 6},
			},
		},
	})
	service := NewService(assets, memoryPoolStore{pools: []venue.Pool{
		{
			ID:       "pepper-sol",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindV2,
			Token0:   "pepper-mint",
			Token1:   "sol-mint",
			Reserve0: scaled(1_000_000, 3),
			Reserve1: scaled(10, 9),
			Enabled:  true,
		},
		{
			ID:       "sol-usdc-small",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindV2,
			Token0:   "sol-mint",
			Token1:   "usdc-mint",
			Reserve0: scaled(1, 9),
			Reserve1: scaled(100, 6),
			Enabled:  true,
		},
		{
			ID:       "sol-usdc-large",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindV2,
			Token0:   "sol-mint",
			Token1:   "usdc-mint",
			Reserve0: scaled(10, 9),
			Reserve1: scaled(900, 6),
			Enabled:  true,
		},
	}})

	result, err := service.Prices(context.Background(), "PEPPER")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Prices) != 1 {
		t.Fatalf("expected one PEPPER price, got %d", len(result.Prices))
	}
	price := result.Prices[0]
	if price.Price != "0.00001" || price.BaseUSDC != "0.0009" || price.QuoteUSDC != "90" || price.PriceUSDC != "0.0009" {
		t.Fatalf("unexpected SOL-derived USDC price: %#v", price)
	}
	if price.USDCRoute == nil || price.USDCRoute.PoolID != "sol-usdc-large" || price.USDCRoute.FromSymbol != "SOL" {
		t.Fatalf("unexpected USDC route: %#v", price.USDCRoute)
	}
}

func TestPricesDoesNotDeriveUSDCFromCLMMVaultReservesWithoutSpotPrice(t *testing.T) {
	assets := asset.NewRegistry([]asset.Asset{
		{
			Symbol:   "PEPPER",
			Name:     "PEPPER",
			Type:     "token",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "pepper-mint", Decimals: 3},
			},
		},
		{
			Symbol:   "SOL",
			Name:     "Solana",
			Type:     "native",
			Decimals: 9,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "sol-mint", Symbol: "WSOL", Decimals: 9},
			},
		},
		{
			Symbol:   "USDC",
			Name:     "USD Coin",
			Type:     "token",
			Decimals: 6,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "usdc-mint", Decimals: 6},
			},
		},
	})
	service := NewService(assets, memoryPoolStore{pools: []venue.Pool{
		{
			ID:       "pepper-sol",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindV2,
			Token0:   "pepper-mint",
			Token1:   "sol-mint",
			Reserve0: scaled(1_000_000, 3),
			Reserve1: scaled(10, 9),
			Enabled:  true,
		},
		{
			ID:       "bad-clmm-sol-usdc",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindCLMM,
			Token0:   "sol-mint",
			Token1:   "usdc-mint",
			Reserve0: scaled(10, 9),
			Reserve1: scaled(145, 6),
			Enabled:  true,
		},
	}})

	result, err := service.Prices(context.Background(), "PEPPER")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Prices) != 1 {
		t.Fatalf("expected one PEPPER price, got %d", len(result.Prices))
	}
	price := result.Prices[0]
	if price.PriceUSDC != "" || price.QuoteUSDC != "" || price.USDCRoute != nil {
		t.Fatalf("expected no derived USDC from CLMM vault reserves, got %#v", price)
	}
}

func TestPricesDerivesQuoteUSDCFromDirectBaseUSDC(t *testing.T) {
	assets := asset.NewRegistry([]asset.Asset{
		{
			Symbol:   "PEPPER",
			Name:     "PEPPER",
			Type:     "token",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "pepper-mint", Decimals: 3},
			},
		},
		{
			Symbol:   "SOL",
			Name:     "Solana",
			Type:     "native",
			Decimals: 9,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "sol-mint", Symbol: "WSOL", Decimals: 9},
			},
		},
		{
			Symbol:   "USDC",
			Name:     "USD Coin",
			Type:     "token",
			Decimals: 6,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "usdc-mint", Decimals: 6},
			},
		},
	})
	service := NewService(assets, memoryPoolStore{pools: []venue.Pool{
		{
			ID:       "pepper-sol",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindV2,
			Token0:   "pepper-mint",
			Token1:   "sol-mint",
			Reserve0: scaled(1_000_000, 3),
			Reserve1: scaled(10, 9),
			Enabled:  true,
		},
		{
			ID:       "pepper-usdc",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindV2,
			Token0:   "pepper-mint",
			Token1:   "usdc-mint",
			Reserve0: scaled(1_000_000, 3),
			Reserve1: scaled(620, 6),
			Enabled:  true,
		},
	}})

	result, err := service.Prices(context.Background(), "PEPPER")
	if err != nil {
		t.Fatal(err)
	}
	var solPrice *PoolPrice
	for i := range result.Prices {
		if result.Prices[i].QuoteSymbol == "SOL" {
			solPrice = &result.Prices[i]
			break
		}
	}
	if solPrice == nil {
		t.Fatalf("expected PEPPER/SOL price, got %#v", result.Prices)
	}
	if solPrice.Price != "0.00001" || solPrice.BaseUSDC != "0.00062" || solPrice.QuoteUSDC != "62" || solPrice.PriceUSDC != "0.00062" {
		t.Fatalf("unexpected quote-derived USDC fields: %#v", solPrice)
	}
}

func TestPricesDoesNotUseDirectCLMMVaultReservesWithoutSpotPrice(t *testing.T) {
	assets := asset.NewRegistry([]asset.Asset{
		{
			Symbol:   "PEPPER",
			Name:     "PEPPER",
			Type:     "token",
			Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "pepper-mint", Decimals: 3},
			},
		},
		{
			Symbol:   "USDC",
			Name:     "USD Coin",
			Type:     "token",
			Decimals: 6,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "usdc-mint", Decimals: 6},
			},
		},
	})
	service := NewService(assets, memoryPoolStore{pools: []venue.Pool{
		{
			ID:       "bad-clmm-pepper-usdc",
			ChainKey: chain.ChainKeySolana,
			VenueKey: venue.VenueKeyMeteora,
			Kind:     venue.PoolKindCLMM,
			Token0:   "pepper-mint",
			Token1:   "usdc-mint",
			Reserve0: scaled(1_000_000, 3),
			Reserve1: scaled(100, 6),
			Enabled:  true,
		},
	}})

	result, err := service.Prices(context.Background(), "PEPPER")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Prices) != 0 {
		t.Fatalf("expected no direct CLMM reserve-derived price, got %#v", result.Prices)
	}
}

func scaled(value int64, decimals int) *big.Int {
	out := big.NewInt(value)
	out.Mul(out, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	return out
}
