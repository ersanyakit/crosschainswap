package swap

import (
	"context"
	"math/big"
	"testing"

	coreswap "exchange/internal/core/swap"
	"exchange/internal/core/venue"
)

type fakePoolProvider struct {
	pools map[venue.PoolID]*venue.Pool
}

func (p fakePoolProvider) GetPool(_ context.Context, id venue.PoolID) (*venue.Pool, error) {
	return p.pools[id], nil
}

func TestConstantProductOut(t *testing.T) {
	out := constantProductOut(
		big.NewInt(1_000),
		big.NewInt(10_000),
		big.NewInt(20_000),
		30,
	)
	if out.String() != "1813" {
		t.Fatalf("unexpected amount out: %s", out)
	}
}

func TestConstantProductOutRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		amountIn   *big.Int
		reserveIn  *big.Int
		reserveOut *big.Int
		feeBps     uint32
	}{
		{name: "nil amount", amountIn: nil, reserveIn: big.NewInt(1), reserveOut: big.NewInt(1), feeBps: 30},
		{name: "zero reserve", amountIn: big.NewInt(1), reserveIn: big.NewInt(0), reserveOut: big.NewInt(1), feeBps: 30},
		{name: "fee too high", amountIn: big.NewInt(1), reserveIn: big.NewInt(1), reserveOut: big.NewInt(1), feeBps: 10_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := constantProductOut(tt.amountIn, tt.reserveIn, tt.reserveOut, tt.feeBps)
			if out.Sign() != 0 {
				t.Fatalf("expected zero amount out, got %s", out)
			}
		})
	}
}

func TestQuoteUsesPoolAddressFallbackAndVenueFee(t *testing.T) {
	poolID := venue.PoolID("0x00000000000000000000000000000000000000aa")
	executor, err := NewUniswapV2Executor("", 30, fakePoolProvider{
		pools: map[venue.PoolID]*venue.Pool{
			poolID: {
				ID:       poolID,
				Kind:     venue.PoolKindV2,
				Token0:   "0x0000000000000000000000000000000000000001",
				Token1:   "0x0000000000000000000000000000000000000002",
				Reserve0: big.NewInt(10_000),
				Reserve1: big.NewInt(20_000),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	executor.FeeBpsByVenue = map[venue.VenueKey]uint32{
		venue.VenueKeyKewlSwap: 100,
	}

	quote, err := executor.Quote(context.Background(), coreswap.Request{
		VenueKey:    venue.VenueKeyKewlSwap,
		PoolAddress: string(poolID),
		TokenIn:     "0x0000000000000000000000000000000000000001",
		TokenOut:    "0x0000000000000000000000000000000000000002",
		AmountIn:    big.NewInt(1_000),
	})
	if err != nil {
		t.Fatal(err)
	}

	if quote.FeeBps != 100 {
		t.Fatalf("expected venue fee 100, got %d", quote.FeeBps)
	}
	if quote.AmountOut.String() != "1801" {
		t.Fatalf("unexpected amount out: %s", quote.AmountOut)
	}
}

func TestQuoteRejectsStablePool(t *testing.T) {
	poolID := venue.PoolID("0x00000000000000000000000000000000000000bb")
	executor, err := NewUniswapV2Executor("", 30, fakePoolProvider{
		pools: map[venue.PoolID]*venue.Pool{
			poolID: {
				ID:       poolID,
				Kind:     venue.PoolKindStable,
				Token0:   "0x0000000000000000000000000000000000000001",
				Token1:   "0x0000000000000000000000000000000000000002",
				Reserve0: big.NewInt(10_000),
				Reserve1: big.NewInt(20_000),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = executor.Quote(context.Background(), coreswap.Request{
		PoolID:   poolID,
		TokenIn:  "0x0000000000000000000000000000000000000001",
		TokenOut: "0x0000000000000000000000000000000000000002",
		AmountIn: big.NewInt(1_000),
	})
	if err == nil {
		t.Fatal("expected stable pool rejection")
	}
}
