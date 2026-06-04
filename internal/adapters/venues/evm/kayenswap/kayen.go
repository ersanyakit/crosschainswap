package kayenswap

import (
	"context"
	"fmt"
	"math/big"

	"exchange/internal/core/chain"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/sync/errgroup"
)

type Scanner struct {
	client   *ethclient.Client
	factory  common.Address
	chainKey chain.ChainKey
	venueKey venue.VenueKey
}

func NewScanner(
	client *ethclient.Client,
	factory common.Address,
	chainKey chain.ChainKey,
	venueKey venue.VenueKey,
) *Scanner {
	return &Scanner{
		client:   client,
		factory:  factory,
		chainKey: chainKey,
		venueKey: venueKey,
	}
}

func (s *Scanner) AllPairsLength(ctx context.Context) (*big.Int, error) {
	return callUint256(ctx, s.client, s.factory, "allPairsLength()")
}

func (s *Scanner) PairAt(ctx context.Context, index *big.Int) (common.Address, error) {
	return callAddressWithUint256(ctx, s.client, s.factory, "allPairs(uint256)", index)
}

func (s *Scanner) LoadPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	pairAddress := common.HexToAddress(string(id))
	return s.loadPair(ctx, pairAddress)
}

func (s *Scanner) loadPair(ctx context.Context, pairAddress common.Address) (*venue.Pool, error) {
	token0, err := callAddress(ctx, s.client, pairAddress, "token0()")
	if err != nil {
		return nil, fmt.Errorf("token0: %w", err)
	}

	token1, err := callAddress(ctx, s.client, pairAddress, "token1()")
	if err != nil {
		return nil, fmt.Errorf("token1: %w", err)
	}

	reserve0, reserve1, err := callReserves(ctx, s.client, pairAddress)
	if err != nil {
		return nil, fmt.Errorf("getReserves: %w", err)
	}

	return &venue.Pool{
		ID:       venue.PoolID(pairAddress.Hex()),
		ChainKey: s.chainKey,
		VenueKey: s.venueKey,
		Kind:     venue.PoolKindV2,

		Token0: venue.AssetID(token0.Hex()),
		Token1: venue.AssetID(token1.Hex()),

		Reserve0: reserve0,
		Reserve1: reserve1,

		Enabled: true,
	}, nil
}

func (s *Scanner) ScanPools(ctx context.Context) ([]venue.Pool, error) {
	total, err := s.AllPairsLength(ctx)
	if err != nil {
		return nil, fmt.Errorf("allPairsLength: %w", err)
	}

	totalInt := total.Int64()
	pairAddresses := make([]common.Address, totalInt)

	// Fetch pair addresses concurrently
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(20) // Limit to 20 concurrent RPC calls

	for i := int64(0); i < totalInt; i++ {
		idx := i
		g.Go(func() error {
			addr, err := s.PairAt(gCtx, big.NewInt(idx))
			if err != nil {
				return fmt.Errorf("allPairs(%d): %w", idx, err)
			}
			pairAddresses[idx] = addr
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Fetch pool details (token0, token1, reserves) concurrently
	pools := make([]venue.Pool, totalInt)
	g, gCtx = errgroup.WithContext(ctx)
	g.SetLimit(20) // Limit to 20 concurrent pool loaders

	for i := int64(0); i < totalInt; i++ {
		idx := i
		g.Go(func() error {
			addr := pairAddresses[idx]
			pool, err := s.loadPair(gCtx, addr)
			if err != nil {
				return fmt.Errorf("load pair %s: %w", addr.Hex(), err)
			}
			pools[idx] = *pool
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return pools, nil
}
