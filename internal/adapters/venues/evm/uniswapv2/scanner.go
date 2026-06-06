package uniswapv2

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	evmassets "exchange/internal/adapters/venues/evm/assets"
	"exchange/internal/adapters/venues/evm/multicall"
	"exchange/internal/adapters/venues/evm/rpc"
	"exchange/internal/core/chain"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const factoryABI = `[
  {"inputs":[],"name":"allPairsLength","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"uint256","name":"","type":"uint256"}],"name":"allPairs","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"address","name":"","type":"address"},{"internalType":"address","name":"","type":"address"}],"name":"getPair","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}
]`

const pairABI = `[
  {"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"getReserves","outputs":[
    {"internalType":"uint112","name":"_reserve0","type":"uint112"},
    {"internalType":"uint112","name":"_reserve1","type":"uint112"},
    {"internalType":"uint32","name":"_blockTimestampLast","type":"uint32"}
  ],"stateMutability":"view","type":"function"}
]`

const defaultBatchSize = 300

type Scanner struct {
	rpc        *rpc.Pool
	multicall  *multicall.Client
	factory    common.Address
	chainKey   chain.ChainKey
	venueKey   venue.VenueKey
	factoryABI abi.ABI
	pairABI    abi.ABI
	batchSize  int
}

func NewScanner(
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
		rpc:        rpcPool,
		multicall:  multicallClient,
		factory:    factory,
		chainKey:   chainKey,
		venueKey:   venueKey,
		factoryABI: parsedFactory,
		pairABI:    parsedPair,
		batchSize:  defaultBatchSize,
	}, nil
}

func (s *Scanner) AllPairsLength(ctx context.Context) (*big.Int, error) {
	data, err := s.factoryABI.Pack("allPairsLength")
	if err != nil {
		return nil, err
	}

	out, err := s.rpc.CallContract(ctx, ethereum.CallMsg{To: &s.factory, Data: data})
	if err != nil {
		return nil, err
	}

	values, err := s.factoryABI.Unpack("allPairsLength", out)
	if err != nil {
		return nil, err
	}

	return values[0].(*big.Int), nil
}

func (s *Scanner) LoadPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	pools, err := s.loadPairs(ctx, []common.Address{common.HexToAddress(string(id))})
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("pool %s not found", id)
	}
	return &pools[0], nil
}

func (s *Scanner) ScanPools(ctx context.Context) ([]venue.Pool, error) {
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

func (s *Scanner) ScanPoolsForAssets(ctx context.Context, assetIDs []venue.AssetID) ([]venue.Pool, error) {
	addresses, err := s.findPairAddresses(ctx, assetIDs)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, nil
	}
	return s.loadPairs(ctx, addresses)
}

func (s *Scanner) ScanPoolsStream(ctx context.Context, handle venue.PoolBatchHandler) (int, error) {
	total, err := s.AllPairsLength(ctx)
	if err != nil {
		return 0, fmt.Errorf("allPairsLength: %w", err)
	}

	totalInt := total.Int64()
	if totalInt == 0 {
		return 0, nil
	}

	totalScanned := 0
	for start := int64(0); start < totalInt; start += int64(s.batchSize) {
		end := min(start+int64(s.batchSize), totalInt)
		addresses, err := s.loadPairAddressBatch(ctx, start, end)
		if err != nil {
			return totalScanned, err
		}
		pools, err := s.loadPairBatch(ctx, addresses)
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

func (s *Scanner) findPairAddresses(ctx context.Context, assetIDs []venue.AssetID) ([]common.Address, error) {
	tokens := evmassets.Addresses(assetIDs)
	if len(tokens) < 2 {
		return nil, nil
	}

	calls := make([]multicall.Call3, 0, (len(tokens)*(len(tokens)-1))/2)
	for i := 0; i < len(tokens); i++ {
		for j := i + 1; j < len(tokens); j++ {
			data, err := s.factoryABI.Pack("getPair", tokens[i], tokens[j])
			if err != nil {
				return nil, err
			}
			calls = append(calls, multicall.Call3{Target: s.factory, AllowFailure: true, CallData: data})
		}
	}

	results, err := s.multicall.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall getPair: %w", err)
	}

	seen := make(map[common.Address]struct{})
	addresses := make([]common.Address, 0, len(results))
	for _, result := range results {
		if !result.Success {
			continue
		}
		values, err := s.factoryABI.Unpack("getPair", result.ReturnData)
		if err != nil {
			continue
		}
		pair := values[0].(common.Address)
		if pair == (common.Address{}) {
			continue
		}
		if _, ok := seen[pair]; ok {
			continue
		}
		seen[pair] = struct{}{}
		addresses = append(addresses, pair)
	}

	return addresses, nil
}

func (s *Scanner) loadPairAddresses(ctx context.Context, total int64) ([]common.Address, error) {
	addresses := make([]common.Address, 0, total)

	for start := int64(0); start < total; start += int64(s.batchSize) {
		end := min(start+int64(s.batchSize), total)
		batch, err := s.loadPairAddressBatch(ctx, start, end)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, batch...)
	}

	return addresses, nil
}

func (s *Scanner) loadPairAddressBatch(ctx context.Context, start, end int64) ([]common.Address, error) {
	calls := make([]multicall.Call3, 0, end-start)

	for i := start; i < end; i++ {
		data, err := s.factoryABI.Pack("allPairs", big.NewInt(i))
		if err != nil {
			return nil, err
		}
		calls = append(calls, multicall.Call3{Target: s.factory, AllowFailure: false, CallData: data})
	}

	results, err := s.multicall.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall allPairs %d-%d: %w", start, end-1, err)
	}

	addresses := make([]common.Address, 0, len(results))
	for offset, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("allPairs(%d) failed", start+int64(offset))
		}
		values, err := s.factoryABI.Unpack("allPairs", result.ReturnData)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, values[0].(common.Address))
	}

	return addresses, nil
}

func (s *Scanner) loadPairs(ctx context.Context, addresses []common.Address) ([]venue.Pool, error) {
	pools := make([]venue.Pool, 0, len(addresses))

	for start := 0; start < len(addresses); start += s.batchSize {
		end := min(start+s.batchSize, len(addresses))
		batch, err := s.loadPairBatch(ctx, addresses[start:end])
		if err != nil {
			return nil, err
		}
		pools = append(pools, batch...)
	}

	return pools, nil
}

func (s *Scanner) loadPairBatch(ctx context.Context, addresses []common.Address) ([]venue.Pool, error) {
	pools := make([]venue.Pool, 0, len(addresses))
	calls := make([]multicall.Call3, 0, len(addresses)*3)

	for _, pair := range addresses {
		for _, method := range []string{"token0", "token1", "getReserves"} {
			data, err := s.pairABI.Pack(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, multicall.Call3{Target: pair, AllowFailure: true, CallData: data})
		}
	}

	results, err := s.multicall.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall pair details: %w", err)
	}

	for i, pair := range addresses {
		base := i * 3
		if !results[base].Success || !results[base+1].Success || !results[base+2].Success {
			continue
		}

		token0Values, err := s.pairABI.Unpack("token0", results[base].ReturnData)
		if err != nil {
			continue
		}
		token1Values, err := s.pairABI.Unpack("token1", results[base+1].ReturnData)
		if err != nil {
			continue
		}
		reserveValues, err := s.pairABI.Unpack("getReserves", results[base+2].ReturnData)
		if err != nil {
			continue
		}

		pools = append(pools, venue.Pool{
			ID:       venue.PoolID(pair.Hex()),
			Address:  pair.Hex(),
			ChainKey: s.chainKey,
			VenueKey: s.venueKey,
			Kind:     venue.PoolKindV2,
			Token0:   venue.AssetID(token0Values[0].(common.Address).Hex()),
			Token1:   venue.AssetID(token1Values[0].(common.Address).Hex()),
			Reserve0: reserveValues[0].(*big.Int),
			Reserve1: reserveValues[1].(*big.Int),
			Enabled:  true,
		})
	}

	return pools, nil
}
