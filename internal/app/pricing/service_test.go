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

func scaled(value int64, decimals int) *big.Int {
	out := big.NewInt(value)
	out.Mul(out, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	return out
}
