package aerodrome

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"exchange/internal/adapters/venues/evm/multicall"
	"exchange/internal/adapters/venues/evm/rpc"
	"exchange/internal/core/chain"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/sync/errgroup"
)

const defaultConcurrency = 3

type Scanner struct {
	client   *ethclient.Client
	rpc      *rpc.Pool
	mc       *multicall.Client
	factory  common.Address
	chainKey chain.ChainKey
	venueKey venue.VenueKey

	factoryABI  abi.ABI
	pairABI     abi.ABI
	concurrency int
	batchSize   int
}

func NewScanner(
	client *ethclient.Client,
	factory common.Address,
	chainKey chain.ChainKey,
	venueKey venue.VenueKey,
) *Scanner {
	parsedFactory, _ := abi.JSON(strings.NewReader(factoryABI))
	parsedPair, _ := abi.JSON(strings.NewReader(pairABI))
	return &Scanner{
		client:      client,
		factory:     factory,
		chainKey:    chainKey,
		venueKey:    venueKey,
		factoryABI:  parsedFactory,
		pairABI:     parsedPair,
		concurrency: defaultConcurrency,
		batchSize:   300,
	}
}

func NewScannerWithRPCPool(
	rpcPool *rpc.Pool,
	multicallClient *multicall.Client,
	factory common.Address,
	chainKey chain.ChainKey,
	venueKey venue.VenueKey,
) (*Scanner, error) {
	parsedFactory, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return nil, err
	}
	parsedPair, err := abi.JSON(strings.NewReader(pairABI))
	if err != nil {
		return nil, err
	}
	return &Scanner{
		rpc:         rpcPool,
		mc:          multicallClient,
		factory:     factory,
		chainKey:    chainKey,
		venueKey:    venueKey,
		factoryABI:  parsedFactory,
		pairABI:     parsedPair,
		concurrency: defaultConcurrency,
		batchSize:   300,
	}, nil
}

func (s *Scanner) AllPoolsLength(ctx context.Context) (*big.Int, error) {
	if s.rpc != nil {
		data, err := s.factoryABI.Pack("allPoolsLength")
		if err != nil {
			return nil, err
		}
		out, err := s.rpc.CallContract(ctx, ethereum.CallMsg{To: &s.factory, Data: data})
		if err != nil {
			return nil, err
		}
		values, err := s.factoryABI.Unpack("allPoolsLength", out)
		if err != nil {
			return nil, err
		}
		return values[0].(*big.Int), nil
	}
	return callPoolsLength(ctx, s.client, s.factory)
}

func (s *Scanner) PoolAt(ctx context.Context, index *big.Int) (common.Address, error) {
	return callPoolAt(ctx, s.client, s.factory, index)
}

func (s *Scanner) LoadPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	poolAddress := common.HexToAddress(string(id))
	if s.mc != nil {
		pools, err := s.loadPoolsMulticall(ctx, []common.Address{poolAddress})
		if err != nil {
			return nil, err
		}
		if len(pools) == 0 {
			return nil, fmt.Errorf("pool %s not found", id)
		}
		return &pools[0], nil
	}
	return s.loadPool(ctx, poolAddress)
}

func (s *Scanner) loadPool(ctx context.Context, poolAddress common.Address) (*venue.Pool, error) {
	token0, err := callAddress(ctx, s.client, poolAddress, "token0")
	if err != nil {
		return nil, fmt.Errorf("token0: %w", err)
	}

	token1, err := callAddress(ctx, s.client, poolAddress, "token1")
	if err != nil {
		return nil, fmt.Errorf("token1: %w", err)
	}

	reserve0, reserve1, err := callReserves(ctx, s.client, poolAddress)
	if err != nil {
		return nil, fmt.Errorf("getReserves: %w", err)
	}
	stable, err := callBool(ctx, s.client, poolAddress, "stable")
	if err != nil {
		return nil, fmt.Errorf("stable: %w", err)
	}
	poolKind := venue.PoolKindV2
	if stable {
		poolKind = venue.PoolKindStable
	}

	return &venue.Pool{
		ID:       venue.PoolID(poolAddress.Hex()),
		Address:  poolAddress.Hex(),
		ChainKey: s.chainKey,
		VenueKey: s.venueKey,
		Kind:     poolKind,

		Token0: venue.AssetID(token0.Hex()),
		Token1: venue.AssetID(token1.Hex()),

		Reserve0: reserve0,
		Reserve1: reserve1,

		Enabled: true,
	}, nil
}

func (s *Scanner) ScanPools(ctx context.Context) ([]venue.Pool, error) {
	if s.mc != nil {
		var pools []venue.Pool
		_, err := s.ScanPoolsStream(ctx, func(_ context.Context, batch []venue.Pool) error {
			pools = append(pools, batch...)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return pools, nil
	}

	total, err := s.AllPoolsLength(ctx)
	if err != nil {
		return nil, fmt.Errorf("allPoolsLength: %w", err)
	}

	totalInt := total.Int64()
	if totalInt == 0 {
		return []venue.Pool{}, nil
	}

	poolAddresses := make([]common.Address, totalInt)

	if s.mc != nil {
		addresses, err := s.loadPoolAddressesMulticall(ctx, totalInt)
		if err != nil {
			return nil, err
		}
		return s.loadPoolsMulticall(ctx, addresses)
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency)

	for i := int64(0); i < totalInt; i++ {
		idx := i

		g.Go(func() error {
			addr, err := s.PoolAt(gCtx, big.NewInt(idx))
			if err != nil {
				return fmt.Errorf("allPools(%d): %w", idx, err)
			}

			poolAddresses[idx] = addr
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	pools := make([]venue.Pool, totalInt)

	g, gCtx = errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency)

	for i := int64(0); i < totalInt; i++ {
		idx := i

		g.Go(func() error {
			addr := poolAddresses[idx]

			pool, err := s.loadPool(gCtx, addr)
			if err != nil {
				return fmt.Errorf("load pool %s: %w", addr.Hex(), err)
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

func (s *Scanner) ScanPoolsStream(ctx context.Context, handle venue.PoolBatchHandler) (int, error) {
	total, err := s.AllPoolsLength(ctx)
	if err != nil {
		return 0, fmt.Errorf("allPoolsLength: %w", err)
	}

	totalInt := total.Int64()
	if totalInt == 0 {
		return 0, nil
	}
	if s.mc == nil {
		pools, err := s.ScanPools(ctx)
		if err != nil {
			return 0, err
		}
		if handle != nil {
			if err := handle(ctx, pools); err != nil {
				return 0, err
			}
		}
		return len(pools), nil
	}

	totalScanned := 0
	for start := int64(0); start < totalInt; start += int64(s.batchSize) {
		end := min(start+int64(s.batchSize), totalInt)
		addresses, err := s.loadPoolAddressBatchMulticall(ctx, start, end)
		if err != nil {
			return totalScanned, err
		}
		pools, err := s.loadPoolBatchMulticall(ctx, addresses)
		if err != nil {
			return totalScanned, err
		}
		if handle != nil {
			if err := handle(ctx, pools); err != nil {
				return totalScanned, err
			}
		}
		totalScanned += len(pools)
	}

	return totalScanned, nil
}

func (s *Scanner) loadPoolAddressesMulticall(ctx context.Context, total int64) ([]common.Address, error) {
	addresses := make([]common.Address, 0, total)

	for start := int64(0); start < total; start += int64(s.batchSize) {
		end := min(start+int64(s.batchSize), total)
		batch, err := s.loadPoolAddressBatchMulticall(ctx, start, end)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, batch...)
	}

	return addresses, nil
}

func (s *Scanner) loadPoolAddressBatchMulticall(ctx context.Context, start, end int64) ([]common.Address, error) {
	calls := make([]multicall.Call3, 0, end-start)

	for i := start; i < end; i++ {
		data, err := s.factoryABI.Pack("allPools", big.NewInt(i))
		if err != nil {
			return nil, err
		}
		calls = append(calls, multicall.Call3{Target: s.factory, AllowFailure: false, CallData: data})
	}

	results, err := s.mc.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall allPools %d-%d: %w", start, end-1, err)
	}

	addresses := make([]common.Address, 0, len(results))
	for offset, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("allPools(%d) failed", start+int64(offset))
		}
		values, err := s.factoryABI.Unpack("allPools", result.ReturnData)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, values[0].(common.Address))
	}

	return addresses, nil
}

func (s *Scanner) loadPoolsMulticall(ctx context.Context, addresses []common.Address) ([]venue.Pool, error) {
	pools := make([]venue.Pool, 0, len(addresses))

	for start := 0; start < len(addresses); start += s.batchSize {
		end := min(start+s.batchSize, len(addresses))
		batch, err := s.loadPoolBatchMulticall(ctx, addresses[start:end])
		if err != nil {
			return nil, err
		}
		pools = append(pools, batch...)
	}

	return pools, nil
}

func (s *Scanner) loadPoolBatchMulticall(ctx context.Context, addresses []common.Address) ([]venue.Pool, error) {
	pools := make([]venue.Pool, 0, len(addresses))
	calls := make([]multicall.Call3, 0, len(addresses)*4)

	for _, pool := range addresses {
		for _, method := range []string{"token0", "token1", "stable", "getReserves"} {
			data, err := s.pairABI.Pack(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, multicall.Call3{Target: pool, AllowFailure: false, CallData: data})
		}
	}

	results, err := s.mc.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall pool details: %w", err)
	}

	for i, pool := range addresses {
		base := i * 4
		if !results[base].Success || !results[base+1].Success || !results[base+2].Success || !results[base+3].Success {
			return nil, fmt.Errorf("pool detail call failed for %s", pool.Hex())
		}

		token0Values, err := s.pairABI.Unpack("token0", results[base].ReturnData)
		if err != nil {
			return nil, err
		}
		token1Values, err := s.pairABI.Unpack("token1", results[base+1].ReturnData)
		if err != nil {
			return nil, err
		}
		stableValues, err := s.pairABI.Unpack("stable", results[base+2].ReturnData)
		if err != nil {
			return nil, err
		}
		reserveValues, err := s.pairABI.Unpack("getReserves", results[base+3].ReturnData)
		if err != nil {
			return nil, err
		}
		poolKind := venue.PoolKindV2
		if stableValues[0].(bool) {
			poolKind = venue.PoolKindStable
		}

		pools = append(pools, venue.Pool{
			ID:       venue.PoolID(pool.Hex()),
			Address:  pool.Hex(),
			ChainKey: s.chainKey,
			VenueKey: s.venueKey,
			Kind:     poolKind,
			Token0:   venue.AssetID(token0Values[0].(common.Address).Hex()),
			Token1:   venue.AssetID(token1Values[0].(common.Address).Hex()),
			Reserve0: reserveValues[0].(*big.Int),
			Reserve1: reserveValues[1].(*big.Int),
			Enabled:  true,
		})
	}

	return pools, nil
}
