package uniswapv1

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	evmassets "exchange/internal/adapters/venues/evm/assets"
	"exchange/internal/adapters/venues/evm/rpc"
	"exchange/internal/core/chain"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const factoryABI = `[
  {"inputs":[{"internalType":"address","name":"token","type":"address"}],"name":"getExchange","outputs":[{"internalType":"address","name":"exchange","type":"address"}],"stateMutability":"view","type":"function"}
]`

const exchangeABI = `[
  {"inputs":[],"name":"tokenAddress","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}
]`

const erc20ABI = `[
  {"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

type Scanner struct {
	rpc         *rpc.Pool
	factory     common.Address
	weth        common.Address
	chainKey    chain.ChainKey
	venueKey    venue.VenueKey
	factoryABI  abi.ABI
	exchangeABI abi.ABI
	erc20ABI    abi.ABI
}

func NewScanner(
	rpcPool *rpc.Pool,
	factory common.Address,
	weth common.Address,
	chainKey chain.ChainKey,
	venueKey venue.VenueKey,
) (*Scanner, error) {
	parsedFactory, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return nil, err
	}
	parsedExchange, err := abi.JSON(strings.NewReader(exchangeABI))
	if err != nil {
		return nil, err
	}
	parsedERC20, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}

	return &Scanner{
		rpc:         rpcPool,
		factory:     factory,
		weth:        weth,
		chainKey:    chainKey,
		venueKey:    venueKey,
		factoryABI:  parsedFactory,
		exchangeABI: parsedExchange,
		erc20ABI:    parsedERC20,
	}, nil
}

func (s *Scanner) ScanPools(ctx context.Context) ([]venue.Pool, error) {
	return nil, fmt.Errorf("uniswap v1 scanner requires tracked asset ids")
}

func (s *Scanner) LoadPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	pool, err := s.loadExchange(ctx, common.HexToAddress(string(id)))
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *Scanner) ScanPoolsForAssets(ctx context.Context, assetIDs []venue.AssetID) ([]venue.Pool, error) {
	exchanges, err := s.findExchanges(ctx, assetIDs)
	if err != nil {
		return nil, err
	}
	pools := make([]venue.Pool, 0, len(exchanges))
	for _, exchange := range exchanges {
		pool, err := s.loadExchange(ctx, exchange)
		if err != nil {
			continue
		}
		pools = append(pools, *pool)
	}
	return pools, nil
}

func (s *Scanner) findExchanges(ctx context.Context, assetIDs []venue.AssetID) ([]common.Address, error) {
	tokens := evmassets.Addresses(assetIDs)
	seen := make(map[common.Address]struct{})
	exchanges := make([]common.Address, 0, len(tokens))
	for _, token := range tokens {
		if token == s.weth {
			continue
		}
		data, err := s.factoryABI.Pack("getExchange", token)
		if err != nil {
			return nil, err
		}
		out, err := s.rpc.CallContract(ctx, ethereum.CallMsg{To: &s.factory, Data: data})
		if err != nil {
			continue
		}
		values, err := s.factoryABI.Unpack("getExchange", out)
		if err != nil {
			continue
		}
		exchange := values[0].(common.Address)
		if exchange == (common.Address{}) {
			continue
		}
		if _, ok := seen[exchange]; ok {
			continue
		}
		seen[exchange] = struct{}{}
		exchanges = append(exchanges, exchange)
	}
	return exchanges, nil
}

func (s *Scanner) loadExchange(ctx context.Context, exchange common.Address) (*venue.Pool, error) {
	token, err := s.exchangeToken(ctx, exchange)
	if err != nil {
		return nil, err
	}
	tokenReserve, err := s.tokenBalance(ctx, token, exchange)
	if err != nil {
		return nil, err
	}
	ethReserve, err := s.rpc.BalanceAt(ctx, exchange)
	if err != nil {
		return nil, err
	}

	return &venue.Pool{
		ID:       venue.PoolID(exchange.Hex()),
		Address:  exchange.Hex(),
		ChainKey: s.chainKey,
		VenueKey: s.venueKey,
		Kind:     venue.PoolKindV2,
		Token0:   venue.AssetID(token.Hex()),
		Token1:   venue.AssetID(s.weth.Hex()),
		Reserve0: tokenReserve,
		Reserve1: ethReserve,
		Enabled:  true,
	}, nil
}

func (s *Scanner) exchangeToken(ctx context.Context, exchange common.Address) (common.Address, error) {
	data, err := s.exchangeABI.Pack("tokenAddress")
	if err != nil {
		return common.Address{}, err
	}
	out, err := s.rpc.CallContract(ctx, ethereum.CallMsg{To: &exchange, Data: data})
	if err != nil {
		return common.Address{}, err
	}
	values, err := s.exchangeABI.Unpack("tokenAddress", out)
	if err != nil {
		return common.Address{}, err
	}
	return values[0].(common.Address), nil
}

func (s *Scanner) tokenBalance(ctx context.Context, token common.Address, account common.Address) (*big.Int, error) {
	data, err := s.erc20ABI.Pack("balanceOf", account)
	if err != nil {
		return nil, err
	}
	out, err := s.rpc.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data})
	if err != nil {
		return nil, err
	}
	values, err := s.erc20ABI.Unpack("balanceOf", out)
	if err != nil {
		return nil, err
	}
	return values[0].(*big.Int), nil
}
