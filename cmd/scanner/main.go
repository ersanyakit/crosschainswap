package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync/atomic"
	"time"

	"exchange/internal/adapters/eventbus"
	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/adapters/venues/evm/aerodrome"
	"exchange/internal/adapters/venues/evm/multicall"
	evmrpc "exchange/internal/adapters/venues/evm/rpc"
	"exchange/internal/adapters/venues/evm/uniswapv2"
	solanaonchain "exchange/internal/adapters/venues/solana/onchain"
	"exchange/internal/config"
	"exchange/internal/core/chain"
	"exchange/internal/core/event"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum/common"
)

func main() {
	fmt.Println("Starting pool scanner with GORM storage...")

	// 1. Load environment variables
	if err := postgres.LoadEnv("."); err != nil {
		log.Printf("Warning: failed to load .env file: %v", err)
	}

	// 2. Connect to database and sync GORM models
	db, err := postgres.Connect()
	if err != nil {
		log.Fatalf("Fatal: failed to initialize database: %v", err)
	}
	repo := postgres.NewPoolRepository(db)
	bus := eventbus.NewInMemory()
	bus.Subscribe(event.PoolBatchScanned, func(ctx context.Context, evt event.Event) error {
		payload, ok := evt.Payload.(event.PoolBatchScannedPayload)
		if !ok {
			return fmt.Errorf("invalid payload for %s", evt.Type)
		}
		return repo.SavePools(ctx, payload.Pools)
	})

	ctx := context.Background()
	regs := config.LoadDefaultRegistries()

	venuesList := regs.Venues.All()
	fmt.Printf("Loaded %d venues from default registry\n", len(venuesList))

	for _, v := range venuesList {
		if !v.Enabled {
			fmt.Printf("Skipping disabled venue: %s (%s)\n", v.Name, v.Key)
			continue
		}

		if v.Kind != venue.VenueKindUniswapV2 &&
			v.Kind != venue.VenueKindAerodrome &&
			v.Kind != venue.VenueKindRaydium &&
			v.Kind != venue.VenueKindOrca &&
			v.Kind != venue.VenueKindMeteora {
			fmt.Printf("Skipping unsupported venue kind: %s (%s) of kind %s\n", v.Name, v.Key, v.Kind)
			continue
		}

		fmt.Printf("\n--- Scanning Venue: %s (%s) ---\n", v.Name, v.Key)

		// Get chain configuration
		chainCfg, ok := regs.Chains.Get(string(v.ChainKey))
		if !ok {
			log.Printf("Error: chain configuration not found for chain key: %s", v.ChainKey)
			continue
		}

		if len(chainCfg.RPCURLs) == 0 {
			log.Printf("Error: no RPC URLs configured for chain: %s", v.ChainKey)
			continue
		}

		if chainCfg.Kind == chain.KindSolana {
			scanner, err := solanaonchain.NewScanner([]string(chainCfg.RPCURLs), v.ChainKey, v.Key, v.Config)
			if err != nil {
				log.Printf("Error: failed to initialize Solana on-chain scanner for %s: %v", v.Key, err)
				continue
			}
			scanAndSave(ctx, bus, scanner, v.Key)
			continue
		}

		if chainCfg.Kind != chain.KindEVM {
			log.Printf("Skipping %s: unsupported chain kind %s", v.Key, chainCfg.Kind)
			continue
		}

		if chainCfg.Multicall3Address == "" {
			log.Printf("Error: no Multicall3 address configured for chain: %s", v.ChainKey)
			continue
		}

		fmt.Printf("Connecting to %d RPC URLs with round-robin failover\n", len(chainCfg.RPCURLs))
		rpcPool, err := evmrpc.New([]string(chainCfg.RPCURLs))
		if err != nil {
			log.Printf("Error: failed to initialize RPC pool for %s: %v", v.ChainKey, err)
			continue
		}
		multicallClient, err := multicall.NewClient(rpcPool, common.HexToAddress(chainCfg.Multicall3Address))
		if err != nil {
			log.Printf("Error: failed to initialize multicall for %s: %v", v.ChainKey, err)
			rpcPool.Close()
			continue
		}

		// Parse the config and instantiate the correct scanner
		var scanner venue.PoolScanner

		switch v.Kind {
		case venue.VenueKindUniswapV2:
			cfg, ok := v.Config.(venue.UniswapV2Config)
			if !ok {
				log.Printf("Error: invalid config type for UniswapV2 venue %s", v.Key)
				rpcPool.Close()
				continue
			}
			factoryAddr := common.HexToAddress(cfg.FactoryAddress)
			fmt.Printf("Factory Address: %s\n", factoryAddr.Hex())
			scanner, err = uniswapv2.NewScanner(rpcPool, multicallClient, factoryAddr, v.ChainKey, v.Key)
		case venue.VenueKindAerodrome:
			cfg, ok := v.Config.(venue.AerodromeClassicConfig)
			if !ok {
				log.Printf("Error: invalid config type for Aerodrome venue %s", v.Key)
				rpcPool.Close()
				continue
			}
			factoryAddr := common.HexToAddress(cfg.PoolFactoryAddress)
			fmt.Printf("Factory Address: %s\n", factoryAddr.Hex())
			scanner, err = aerodrome.NewScannerWithRPCPool(rpcPool, multicallClient, factoryAddr, v.ChainKey, v.Key)
		}
		if err != nil {
			log.Printf("Error: failed to create scanner for %s: %v", v.Key, err)
			rpcPool.Close()
			continue
		}

		// Retrieve total count of pairs
		var total *big.Int
		switch s := scanner.(type) {
		case *uniswapv2.Scanner:
			total, err = s.AllPairsLength(ctx)
		case *aerodrome.Scanner:
			total, err = s.AllPoolsLength(ctx)
		}

		if err != nil {
			log.Printf("Error: failed to fetch all pairs length for %s: %v", v.Key, err)
			rpcPool.Close()
			continue
		}

		fmt.Printf("Total pairs found in factory: %s\n", total.String())
		scanAndSave(ctx, bus, scanner, v.Key)
		rpcPool.Close()
	}
}

func scanAndSave(
	ctx context.Context,
	bus event.Bus,
	scanner venue.PoolScanner,
	venueKey venue.VenueKey,
) {
	fmt.Println("Scanning all pools...")
	startTime := time.Now()

	if streaming, ok := scanner.(venue.StreamingPoolScanner); ok {
		var totalSaved atomic.Int64
		totalScanned, err := streaming.ScanPoolsStream(ctx, func(ctx context.Context, pools []venue.Pool) error {
			if len(pools) == 0 {
				return nil
			}

			if err := bus.Publish(ctx, event.Event{
				Type: event.PoolBatchScanned,
				Payload: event.PoolBatchScannedPayload{
					VenueKey: venueKey,
					Pools:    pools,
				},
			}); err != nil {
				return err
			}

			saved := totalSaved.Add(int64(len(pools)))
			fmt.Printf("Saved %d pools for %s (total saved: %d)\n", len(pools), venueKey, saved)
			return nil
		})
		if err != nil {
			log.Printf("Error: failed to scan pools for %s: %v", venueKey, err)
			return
		}

		duration := time.Since(startTime)
		if totalScanned > 0 {
			fmt.Printf("Scanned and saved %d pools in %v (Average: %v per pool)\n", totalScanned, duration, duration/time.Duration(totalScanned))
		} else {
			fmt.Printf("Scanned and saved 0 pools in %v\n", duration)
		}
		return
	}

	pools, err := scanner.ScanPools(ctx)
	if err != nil {
		log.Printf("Error: failed to scan pools for %s: %v", venueKey, err)
		return
	}
	duration := time.Since(startTime)
	if len(pools) > 0 {
		fmt.Printf("Scanned %d pools in %v (Average: %v per pool)\n", len(pools), duration, duration/time.Duration(len(pools)))
	} else {
		fmt.Printf("Scanned 0 pools in %v\n", duration)
	}

	fmt.Printf("Saving %d pools to Postgres...\n", len(pools))
	dbStartTime := time.Now()
	if err := bus.Publish(ctx, event.Event{
		Type: event.PoolBatchScanned,
		Payload: event.PoolBatchScannedPayload{
			VenueKey: venueKey,
			Pools:    pools,
		},
	}); err != nil {
		log.Printf("Error: failed to save pools to database: %v", err)
	} else {
		fmt.Printf("Successfully saved %d pools in %v\n", len(pools), time.Since(dbStartTime))
	}
}
