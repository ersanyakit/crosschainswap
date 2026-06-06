package uniswapv3

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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const factoryABI = `[
  {"inputs":[{"internalType":"address","name":"tokenA","type":"address"},{"internalType":"address","name":"tokenB","type":"address"},{"internalType":"uint24","name":"fee","type":"uint24"}],"name":"getPool","outputs":[{"internalType":"address","name":"pool","type":"address"}],"stateMutability":"view","type":"function"}
]`

const slipstreamFactoryABI = `[
  {"inputs":[{"internalType":"address","name":"tokenA","type":"address"},{"internalType":"address","name":"tokenB","type":"address"},{"internalType":"int24","name":"tickSpacing","type":"int24"}],"name":"getPool","outputs":[{"internalType":"address","name":"pool","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"tickSpacings","outputs":[{"internalType":"int24[]","name":"","type":"int24[]"}],"stateMutability":"view","type":"function"}
]`

const poolABI = `[
  {"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"fee","outputs":[{"internalType":"uint24","name":"","type":"uint24"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"liquidity","outputs":[{"internalType":"uint128","name":"","type":"uint128"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"slot0","outputs":[
    {"internalType":"uint160","name":"sqrtPriceX96","type":"uint160"},
    {"internalType":"int24","name":"tick","type":"int24"},
    {"internalType":"uint16","name":"observationIndex","type":"uint16"},
    {"internalType":"uint16","name":"observationCardinality","type":"uint16"},
    {"internalType":"uint16","name":"observationCardinalityNext","type":"uint16"},
    {"internalType":"uint8","name":"feeProtocol","type":"uint8"},
    {"internalType":"bool","name":"unlocked","type":"bool"}
  ],"stateMutability":"view","type":"function"}
]`

const slipstreamPoolABI = `[
  {"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"fee","outputs":[{"internalType":"uint24","name":"","type":"uint24"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"tickSpacing","outputs":[{"internalType":"int24","name":"","type":"int24"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"liquidity","outputs":[{"internalType":"uint128","name":"","type":"uint128"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"slot0","outputs":[
    {"internalType":"uint160","name":"sqrtPriceX96","type":"uint160"},
    {"internalType":"int24","name":"tick","type":"int24"},
    {"internalType":"uint16","name":"observationIndex","type":"uint16"},
    {"internalType":"uint16","name":"observationCardinality","type":"uint16"},
    {"internalType":"uint16","name":"observationCardinalityNext","type":"uint16"},
    {"internalType":"bool","name":"unlocked","type":"bool"}
  ],"stateMutability":"view","type":"function"}
]`

var defaultPoolKeys = []poolKey{
	{fee: 100},
	{fee: 500},
	{fee: 3000},
	{fee: 10000},
}

var defaultSlipstreamTickSpacings = []int32{1, 50, 100, 200, 2000}

type poolKey struct {
	fee         uint32
	tickSpacing int32
}

type Scanner struct {
	rpc             *rpc.Pool
	multicall       *multicall.Client
	factory         common.Address
	chainKey        chain.ChainKey
	venueKey        venue.VenueKey
	factoryABI      abi.ABI
	poolABI         abi.ABI
	poolKeys        []poolKey
	usesTickSpacing bool
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
	parsedPool, err := abi.JSON(strings.NewReader(poolABI))
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
		poolABI:    parsedPool,
		poolKeys:   defaultPoolKeys,
	}, nil
}

func NewSlipstreamScanner(
	rpcPool *rpc.Pool,
	multicallClient *multicall.Client,
	factory common.Address,
	chainKey chain.ChainKey,
	venueKey venue.VenueKey,
) (*Scanner, error) {
	parsedFactory, err := abi.JSON(strings.NewReader(slipstreamFactoryABI))
	if err != nil {
		return nil, err
	}
	parsedPool, err := abi.JSON(strings.NewReader(slipstreamPoolABI))
	if err != nil {
		return nil, err
	}

	return &Scanner{
		rpc:             rpcPool,
		multicall:       multicallClient,
		factory:         factory,
		chainKey:        chainKey,
		venueKey:        venueKey,
		factoryABI:      parsedFactory,
		poolABI:         parsedPool,
		poolKeys:        slipstreamPoolKeys(defaultSlipstreamTickSpacings),
		usesTickSpacing: true,
	}, nil
}

func (s *Scanner) ScanPools(ctx context.Context) ([]venue.Pool, error) {
	return nil, fmt.Errorf("uniswap v3 scanner requires tracked asset ids")
}

func (s *Scanner) LoadPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	pools, err := s.loadPools(ctx, []common.Address{common.HexToAddress(string(id))})
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("pool %s not found", id)
	}
	return &pools[0], nil
}

func (s *Scanner) ScanPoolsForAssets(ctx context.Context, assetIDs []venue.AssetID) ([]venue.Pool, error) {
	addresses, err := s.findPoolAddresses(ctx, assetIDs)
	if err != nil {
		return nil, err
	}
	return s.loadPools(ctx, addresses)
}

func (s *Scanner) findPoolAddresses(ctx context.Context, assetIDs []venue.AssetID) ([]common.Address, error) {
	tokens := evmassets.Addresses(assetIDs)
	if len(tokens) < 2 {
		return nil, nil
	}
	poolKeys := s.scanPoolKeys(ctx)

	calls := make([]multicall.Call3, 0, len(tokens)*(len(tokens)-1)*len(poolKeys)/2)
	for i := 0; i < len(tokens); i++ {
		for j := i + 1; j < len(tokens); j++ {
			for _, key := range poolKeys {
				data, err := s.packGetPool(tokens[i], tokens[j], key)
				if err != nil {
					return nil, err
				}
				calls = append(calls, multicall.Call3{Target: s.factory, AllowFailure: true, CallData: data})
			}
		}
	}

	results, err := s.multicall.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall getPool: %w", err)
	}

	seen := make(map[common.Address]struct{})
	addresses := make([]common.Address, 0, len(results))
	for _, result := range results {
		if !result.Success {
			continue
		}
		values, err := s.factoryABI.Unpack("getPool", result.ReturnData)
		if err != nil {
			continue
		}
		pool := values[0].(common.Address)
		if pool == (common.Address{}) {
			continue
		}
		if _, ok := seen[pool]; ok {
			continue
		}
		seen[pool] = struct{}{}
		addresses = append(addresses, pool)
	}
	return addresses, nil
}

func (s *Scanner) scanPoolKeys(ctx context.Context) []poolKey {
	if !s.usesTickSpacing {
		return s.poolKeys
	}

	data, err := s.factoryABI.Pack("tickSpacings")
	if err != nil {
		return s.poolKeys
	}
	results, err := s.multicall.Aggregate3(ctx, []multicall.Call3{{
		Target:       s.factory,
		AllowFailure: true,
		CallData:     data,
	}})
	if err != nil || len(results) == 0 || !results[0].Success {
		return s.poolKeys
	}
	values, err := s.factoryABI.Unpack("tickSpacings", results[0].ReturnData)
	if err != nil || len(values) == 0 {
		return s.poolKeys
	}
	spacings := abiInt32Slice(values[0])
	if len(spacings) == 0 {
		return s.poolKeys
	}
	return slipstreamPoolKeys(spacings)
}

func (s *Scanner) packGetPool(tokenA, tokenB common.Address, key poolKey) ([]byte, error) {
	if s.usesTickSpacing {
		return s.factoryABI.Pack("getPool", tokenA, tokenB, big.NewInt(int64(key.tickSpacing)))
	}
	return s.factoryABI.Pack("getPool", tokenA, tokenB, new(big.Int).SetUint64(uint64(key.fee)))
}

func (s *Scanner) loadPools(ctx context.Context, addresses []common.Address) ([]venue.Pool, error) {
	if len(addresses) == 0 {
		return nil, nil
	}
	methods := []string{"token0", "token1", "fee", "liquidity", "slot0"}
	if s.usesTickSpacing {
		methods = []string{"token0", "token1", "fee", "tickSpacing", "liquidity", "slot0"}
	}

	calls := make([]multicall.Call3, 0, len(addresses)*len(methods))
	for _, pool := range addresses {
		for _, method := range methods {
			data, err := s.poolABI.Pack(method)
			if err != nil {
				return nil, err
			}
			calls = append(calls, multicall.Call3{Target: pool, AllowFailure: true, CallData: data})
		}
	}

	results, err := s.multicall.Aggregate3(ctx, calls)
	if err != nil {
		return nil, fmt.Errorf("multicall pool details: %w", err)
	}

	pools := make([]venue.Pool, 0, len(addresses))
	for i, pool := range addresses {
		base := i * len(methods)
		if !allSuccessful(results[base : base+len(methods)]) {
			continue
		}
		token0Values, err := s.poolABI.Unpack("token0", results[base].ReturnData)
		if err != nil {
			continue
		}
		token1Values, err := s.poolABI.Unpack("token1", results[base+1].ReturnData)
		if err != nil {
			continue
		}
		feeValues, err := s.poolABI.Unpack("fee", results[base+2].ReturnData)
		if err != nil {
			continue
		}

		next := base + 3
		var tickSpacing int32
		if s.usesTickSpacing {
			tickSpacingValues, err := s.poolABI.Unpack("tickSpacing", results[next].ReturnData)
			if err != nil {
				continue
			}
			parsedTickSpacing, ok := abiInt32(tickSpacingValues[0])
			if !ok {
				continue
			}
			tickSpacing = parsedTickSpacing
			next++
		}

		liquidityValues, err := s.poolABI.Unpack("liquidity", results[next].ReturnData)
		if err != nil {
			continue
		}
		next++
		slot0Values, err := s.poolABI.Unpack("slot0", results[next].ReturnData)
		if err != nil {
			continue
		}

		sqrtPrice, ok := abiBigInt(slot0Values[0])
		if !ok || sqrtPrice.Sign() <= 0 {
			continue
		}
		liquidity, ok := abiBigInt(liquidityValues[0])
		if !ok {
			continue
		}
		tick, ok := abiInt64(slot0Values[1])
		if !ok {
			continue
		}
		fee, ok := abiUint32(feeValues[0])
		if !ok {
			continue
		}

		pools = append(pools, venue.Pool{
			ID:           venue.PoolID(pool.Hex()),
			Address:      pool.Hex(),
			ChainKey:     s.chainKey,
			VenueKey:     s.venueKey,
			Kind:         venue.PoolKindV3,
			Token0:       venue.AssetID(token0Values[0].(common.Address).Hex()),
			Token1:       venue.AssetID(token1Values[0].(common.Address).Hex()),
			SqrtPriceX96: sqrtPrice,
			Liquidity:    liquidity,
			Tick:         tick,
			Fee:          fee,
			TickSpacing:  tickSpacing,
			Enabled:      true,
		})
	}
	return pools, nil
}

func allSuccessful(results []multicall.Result) bool {
	for _, result := range results {
		if !result.Success {
			return false
		}
	}
	return true
}

func slipstreamPoolKeys(spacings []int32) []poolKey {
	keys := make([]poolKey, 0, len(spacings))
	seen := make(map[int32]struct{}, len(spacings))
	for _, spacing := range spacings {
		if spacing == 0 {
			continue
		}
		if _, ok := seen[spacing]; ok {
			continue
		}
		seen[spacing] = struct{}{}
		keys = append(keys, poolKey{tickSpacing: spacing})
	}
	return keys
}

func abiBigInt(value any) (*big.Int, bool) {
	switch v := value.(type) {
	case *big.Int:
		return v, true
	case uint64:
		return new(big.Int).SetUint64(v), true
	case uint32:
		return new(big.Int).SetUint64(uint64(v)), true
	case uint16:
		return new(big.Int).SetUint64(uint64(v)), true
	case uint8:
		return new(big.Int).SetUint64(uint64(v)), true
	case int64:
		return big.NewInt(v), true
	case int32:
		return big.NewInt(int64(v)), true
	case int16:
		return big.NewInt(int64(v)), true
	case int8:
		return big.NewInt(int64(v)), true
	default:
		return nil, false
	}
}

func abiInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case *big.Int:
		return v.Int64(), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case int16:
		return int64(v), true
	case int8:
		return int64(v), true
	default:
		return 0, false
	}
}

func abiInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case *big.Int:
		return int32(v.Int64()), true
	case int64:
		return int32(v), true
	case int32:
		return v, true
	case int16:
		return int32(v), true
	case int8:
		return int32(v), true
	default:
		return 0, false
	}
}

func abiInt32Slice(value any) []int32 {
	switch values := value.(type) {
	case []int32:
		return values
	case []*big.Int:
		out := make([]int32, 0, len(values))
		for _, value := range values {
			if value == nil {
				continue
			}
			out = append(out, int32(value.Int64()))
		}
		return out
	case []any:
		out := make([]int32, 0, len(values))
		for _, value := range values {
			parsed, ok := abiInt32(value)
			if ok {
				out = append(out, parsed)
			}
		}
		return out
	default:
		return nil
	}
}

func abiUint32(value any) (uint32, bool) {
	switch v := value.(type) {
	case *big.Int:
		return uint32(v.Uint64()), true
	case uint64:
		return uint32(v), true
	case uint32:
		return v, true
	case uint16:
		return uint32(v), true
	case uint8:
		return uint32(v), true
	default:
		return 0, false
	}
}
