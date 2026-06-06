package bootstrap

import (
	"fmt"

	evmswap "exchange/internal/adapters/venues/evm/swap"
	solanaswap "exchange/internal/adapters/venues/solana/swap"
	appswap "exchange/internal/app/swap"
	"exchange/internal/core/venue"
)

type SwapEngineOptions struct {
	PoolProvider   evmswap.PoolProvider
	V3Quoter       evmswap.V3Quoter
	SolanaBuilders map[venue.VenueKind]solanaswap.InstructionBuilder
}

func NewSwapEngine(venues venue.Registry, opts SwapEngineOptions) (*appswap.Engine, error) {
	engine := appswap.NewEngine()

	uniswapV2Routers := make(map[venue.VenueKey]string)
	aerodromeRouters := make(map[venue.VenueKey]string)
	uniswapV3Routers := make(map[venue.VenueKey]string)
	slipstreamVenues := make(map[venue.VenueKey]bool)

	for _, v := range venues.All() {
		if !v.Enabled {
			continue
		}

		switch v.Kind {
		case venue.VenueKindUniswapV2:
			cfg, ok := v.Config.(venue.UniswapV2Config)
			if !ok {
				return nil, fmt.Errorf("invalid uniswap v2 config for venue %s", v.Key)
			}
			if cfg.RouterAddress != "" {
				uniswapV2Routers[v.Key] = cfg.RouterAddress
			}
		case venue.VenueKindAerodrome:
			cfg, ok := v.Config.(venue.AerodromeClassicConfig)
			if !ok {
				return nil, fmt.Errorf("invalid aerodrome config for venue %s", v.Key)
			}
			if cfg.RouterAddress != "" {
				aerodromeRouters[v.Key] = cfg.RouterAddress
			}
		case venue.VenueKindUniswapV3:
			cfg, ok := v.Config.(venue.UniswapV3Config)
			if !ok {
				return nil, fmt.Errorf("invalid uniswap v3 config for venue %s", v.Key)
			}
			if cfg.RouterAddress != "" {
				uniswapV3Routers[v.Key] = cfg.RouterAddress
			}
			if v.Key == venue.VenueKeyAerodromeSlipstream {
				slipstreamVenues[v.Key] = true
			}
		}
	}

	if len(uniswapV2Routers) > 0 {
		executor, err := evmswap.NewUniswapV2Executor("", 30, opts.PoolProvider)
		if err != nil {
			return nil, err
		}
		executor.RouterByVenue = uniswapV2Routers
		engine.Register(venue.VenueKindUniswapV2, executor)
	}

	if len(aerodromeRouters) > 0 {
		executor, err := evmswap.NewUniswapV2Executor("", 30, opts.PoolProvider)
		if err != nil {
			return nil, err
		}
		executor.RouterByVenue = aerodromeRouters
		engine.Register(venue.VenueKindAerodrome, executor)
	}

	if len(uniswapV3Routers) > 0 {
		executor, err := evmswap.NewUniswapV3Executor("", 3000, opts.V3Quoter)
		if err != nil {
			return nil, err
		}
		executor.RouterByVenue = uniswapV3Routers
		executor.SlipstreamByVenue = slipstreamVenues
		executor.Pools = opts.PoolProvider
		engine.Register(venue.VenueKindUniswapV3, executor)
	}

	for kind, builder := range opts.SolanaBuilders {
		if builder == nil {
			continue
		}
		engine.Register(kind, solanaswap.NewPoolExecutor(builder))
	}

	return engine, nil
}
