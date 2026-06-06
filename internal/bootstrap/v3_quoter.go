package bootstrap

import (
	"fmt"

	evmrpc "exchange/internal/adapters/venues/evm/rpc"
	evmswap "exchange/internal/adapters/venues/evm/swap"
	"exchange/internal/core/chain"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum/common"
)

func NewUniswapV3Quoter(
	chains chain.Registry,
	venues venue.Registry,
) (*evmswap.UniswapV3Quoter, func(), error) {
	rpcByChain := make(map[chain.ChainKey]*evmrpc.Pool)
	quoterByVenue := make(map[venue.VenueKey]common.Address)
	slipstreamByVenue := make(map[venue.VenueKey]bool)

	closeAll := func() {
		for _, rpcPool := range rpcByChain {
			rpcPool.Close()
		}
	}

	for _, v := range venues.All() {
		if !v.Enabled || v.Kind != venue.VenueKindUniswapV3 {
			continue
		}

		cfg, ok := v.Config.(venue.UniswapV3Config)
		if !ok {
			closeAll()
			return nil, nil, fmt.Errorf("invalid uniswap v3 config for venue %s", v.Key)
		}
		if cfg.QuoterAddress == "" {
			continue
		}

		chainCfg, ok := chains.Get(string(v.ChainKey))
		if !ok || !chainCfg.Enabled || chainCfg.Kind != chain.KindEVM || len(chainCfg.RPCURLs) == 0 {
			continue
		}

		if _, ok := rpcByChain[v.ChainKey]; !ok {
			rpcPool, err := evmrpc.New([]string(chainCfg.RPCURLs))
			if err != nil {
				closeAll()
				return nil, nil, fmt.Errorf("failed to initialize uniswap v3 quoter rpc for %s: %w", v.ChainKey, err)
			}
			rpcByChain[v.ChainKey] = rpcPool
		}
		quoterByVenue[v.Key] = common.HexToAddress(cfg.QuoterAddress)
		if v.Key == venue.VenueKeyAerodromeSlipstream {
			slipstreamByVenue[v.Key] = true
		}
	}

	if len(quoterByVenue) == 0 {
		return nil, func() {}, nil
	}

	quoter, err := evmswap.NewUniswapV3Quoter(rpcByChain, quoterByVenue, slipstreamByVenue)
	if err != nil {
		closeAll()
		return nil, nil, err
	}
	return quoter, closeAll, nil
}
