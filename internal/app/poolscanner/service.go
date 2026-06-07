package poolscanner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"exchange/internal/adapters/eventbus"
	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/adapters/venues/evm/aerodrome"
	"exchange/internal/adapters/venues/evm/multicall"
	evmrpc "exchange/internal/adapters/venues/evm/rpc"
	"exchange/internal/adapters/venues/evm/uniswapv1"
	"exchange/internal/adapters/venues/evm/uniswapv2"
	"exchange/internal/adapters/venues/evm/uniswapv3"
	solanaonchain "exchange/internal/adapters/venues/solana/onchain"
	"exchange/internal/app/pricing"
	"exchange/internal/config"
	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	"exchange/internal/core/event"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum/common"
)

func Run(ctx context.Context) error {
	fmt.Println("Starting pool scanner with GORM storage...")

	// 1. Load environment variables
	if err := postgres.LoadEnv("."); err != nil {
		log.Printf("Warning: failed to load .env file: %v", err)
	}

	// 2. Connect to database and sync GORM models
	db, err := postgres.Connect()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	regs := config.LoadDefaultRegistries()
	repo := postgres.NewPoolRepository(db)
	outboxRepo := postgres.NewOutboxRepository(db)
	leaseRepo := postgres.NewLeaseRepository(db)
	leaseOwner := scannerLeaseOwner()
	priceService := pricing.NewService(regs.Assets, repo)
	bus := eventbus.NewInMemory()
	bus.Subscribe(event.PoolBatchScanned, func(ctx context.Context, evt event.Event) error {
		payload, ok := evt.Payload.(event.PoolBatchScannedPayload)
		if !ok {
			return fmt.Errorf("invalid payload for %s", evt.Type)
		}
		if err := repo.SavePools(ctx, payload.Pools); err != nil {
			return err
		}
		return publishPriceUpdates(ctx, outboxRepo, priceService, payload.Pools)
	})

	venuesList := regs.Venues.All()
	fmt.Printf("Loaded %d venues from default registry\n", len(venuesList))

	evmConnections := make(map[chain.ChainKey]*evmConnection)
	defer closeEVMConnections(evmConnections)

	for {
		for _, v := range venuesList {
			if !v.Enabled {
				fmt.Printf("Skipping disabled venue: %s (%s)\n", v.Name, v.Key)
				continue
			}

			if v.Kind != venue.VenueKindUniswapV1 &&
				v.Kind != venue.VenueKindUniswapV2 &&
				v.Kind != venue.VenueKindUniswapV3 &&
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
				if trackedScanEnabled() {
					runVenueScan(ctx, leaseRepo, v, leaseOwner, func() {
						scanTrackedAndSave(ctx, bus, scanner, v.Key, trackedAssetIDs(regs.Assets, v.ChainKey))
					})
					continue
				}
				runVenueScan(ctx, leaseRepo, v, leaseOwner, func() {
					scanAndSave(ctx, bus, scanner, v.Key)
				})
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

			conn, err := getEVMConnection(evmConnections, chainCfg)
			if err != nil {
				log.Printf("Error: failed to initialize EVM connection for %s: %v", v.ChainKey, err)
				continue
			}
			rpcPool := conn.rpcPool
			multicallClient := conn.multicallClient

			// Parse the config and instantiate the correct scanner
			var scanner venue.PoolScanner

			switch v.Kind {
			case venue.VenueKindUniswapV1:
				cfg, ok := v.Config.(venue.UniswapV1Config)
				if !ok {
					log.Printf("Error: invalid config type for UniswapV1 venue %s", v.Key)
					continue
				}
				factoryAddr := common.HexToAddress(cfg.FactoryAddress)
				wethAddr := common.HexToAddress(cfg.WETHAddress)
				fmt.Printf("Factory Address: %s\n", factoryAddr.Hex())
				scanner, err = uniswapv1.NewScanner(rpcPool, factoryAddr, wethAddr, v.ChainKey, v.Key)
			case venue.VenueKindUniswapV2:
				cfg, ok := v.Config.(venue.UniswapV2Config)
				if !ok {
					log.Printf("Error: invalid config type for UniswapV2 venue %s", v.Key)
					continue
				}
				factoryAddr := common.HexToAddress(cfg.FactoryAddress)
				fmt.Printf("Factory Address: %s\n", factoryAddr.Hex())
				scanner, err = uniswapv2.NewScanner(rpcPool, multicallClient, factoryAddr, v.ChainKey, v.Key)
			case venue.VenueKindUniswapV3:
				cfg, ok := v.Config.(venue.UniswapV3Config)
				if !ok {
					log.Printf("Error: invalid config type for UniswapV3 venue %s", v.Key)
					continue
				}
				factoryAddr := common.HexToAddress(cfg.FactoryAddress)
				fmt.Printf("Factory Address: %s\n", factoryAddr.Hex())
				if v.Key == venue.VenueKeyAerodromeSlipstream {
					scanner, err = uniswapv3.NewSlipstreamScanner(rpcPool, multicallClient, factoryAddr, v.ChainKey, v.Key)
				} else {
					scanner, err = uniswapv3.NewScanner(rpcPool, multicallClient, factoryAddr, v.ChainKey, v.Key)
				}
			case venue.VenueKindAerodrome:
				cfg, ok := v.Config.(venue.AerodromeClassicConfig)
				if !ok {
					log.Printf("Error: invalid config type for Aerodrome venue %s", v.Key)
					continue
				}
				factoryAddr := common.HexToAddress(cfg.PoolFactoryAddress)
				fmt.Printf("Factory Address: %s\n", factoryAddr.Hex())
				scanner, err = aerodrome.NewScannerWithRPCPool(rpcPool, multicallClient, factoryAddr, v.ChainKey, v.Key)
			}
			if err != nil {
				log.Printf("Error: failed to create scanner for %s: %v", v.Key, err)
				continue
			}

			if trackedScanEnabled() {
				runVenueScan(ctx, leaseRepo, v, leaseOwner, func() {
					scanTrackedAndSave(ctx, bus, scanner, v.Key, trackedAssetIDs(regs.Assets, v.ChainKey))
				})
				continue
			}

			// Retrieve total count of pairs
			var total *big.Int
			switch s := scanner.(type) {
			case *uniswapv2.Scanner:
				total, err = s.AllPairsLength(ctx)
			case *aerodrome.Scanner:
				total, err = s.AllPoolsLength(ctx)
			default:
				log.Printf("Skipping full scan for %s: scanner only supports tracked registry scans", v.Key)
				continue
			}

			if err != nil {
				log.Printf("Error: failed to fetch all pairs length for %s: %v", v.Key, err)
				continue
			}

			fmt.Printf("Total pairs found in factory: %s\n", total.String())
			runVenueScan(ctx, leaseRepo, v, leaseOwner, func() {
				scanAndSave(ctx, bus, scanner, v.Key)
			})
		}

		interval := scannerInterval()
		if interval <= 0 {
			break
		}
		fmt.Printf("Scanner cycle completed; sleeping %v\n", interval)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
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

type evmTrackedScanner interface {
	ScanPoolsForAssets(ctx context.Context, assetIDs []venue.AssetID) ([]venue.Pool, error)
}

type evmConnection struct {
	rpcPool         *evmrpc.Pool
	multicallClient *multicall.Client
}

func getEVMConnection(connections map[chain.ChainKey]*evmConnection, chainCfg chain.Chain) (*evmConnection, error) {
	if conn, ok := connections[chainCfg.Key]; ok {
		return conn, nil
	}

	fmt.Printf("Connecting to %d RPC URLs for %s with round-robin failover\n", len(chainCfg.RPCURLs), chainCfg.Key)
	rpcPool, err := evmrpc.New([]string(chainCfg.RPCURLs))
	if err != nil {
		return nil, fmt.Errorf("rpc pool: %w", err)
	}
	multicallClient, err := multicall.NewClient(rpcPool, common.HexToAddress(chainCfg.Multicall3Address))
	if err != nil {
		rpcPool.Close()
		return nil, fmt.Errorf("multicall: %w", err)
	}

	conn := &evmConnection{rpcPool: rpcPool, multicallClient: multicallClient}
	connections[chainCfg.Key] = conn
	return conn, nil
}

func closeEVMConnections(connections map[chain.ChainKey]*evmConnection) {
	for _, conn := range connections {
		conn.rpcPool.Close()
	}
}

type solanaTrackedScanner interface {
	ScanPoolsForAssets(ctx context.Context, assetIDs []venue.AssetID, handle venue.PoolBatchHandler) (int, error)
}

func scanTrackedAndSave(
	ctx context.Context,
	bus event.Bus,
	scanner venue.PoolScanner,
	venueKey venue.VenueKey,
	assetIDs []venue.AssetID,
) {
	if len(assetIDs) == 0 {
		fmt.Printf("Skipping tracked scan for %s: no registry assets on chain\n", venueKey)
		return
	}

	fmt.Printf("Scanning tracked registry pools for %s (%d asset deployments)...\n", venueKey, len(assetIDs))
	startTime := time.Now()

	if streaming, ok := scanner.(solanaTrackedScanner); ok {
		var totalSaved atomic.Int64
		totalScanned, err := streaming.ScanPoolsForAssets(ctx, assetIDs, func(ctx context.Context, pools []venue.Pool) error {
			if len(pools) == 0 {
				return nil
			}
			if err := publishPools(ctx, bus, venueKey, pools); err != nil {
				return err
			}
			saved := totalSaved.Add(int64(len(pools)))
			fmt.Printf("Saved %d tracked pools for %s (total saved: %d)\n", len(pools), venueKey, saved)
			return nil
		})
		if err != nil {
			log.Printf("Error: failed tracked scan for %s: %v", venueKey, err)
			return
		}
		fmt.Printf("Tracked scan saved %d pools for %s in %v\n", totalScanned, venueKey, time.Since(startTime))
		return
	}

	tracked, ok := scanner.(evmTrackedScanner)
	if !ok {
		log.Printf("Error: scanner for %s does not support tracked scans", venueKey)
		return
	}

	pools, err := tracked.ScanPoolsForAssets(ctx, assetIDs)
	if err != nil {
		log.Printf("Error: failed tracked scan for %s: %v", venueKey, err)
		return
	}
	if err := publishPools(ctx, bus, venueKey, pools); err != nil {
		log.Printf("Error: failed to save tracked pools for %s: %v", venueKey, err)
		return
	}
	fmt.Printf("Tracked scan saved %d pools for %s in %v\n", len(pools), venueKey, time.Since(startTime))
}

func publishPools(ctx context.Context, bus event.Bus, venueKey venue.VenueKey, pools []venue.Pool) error {
	if len(pools) == 0 {
		return nil
	}
	return bus.Publish(ctx, event.Event{
		Type: event.PoolBatchScanned,
		Payload: event.PoolBatchScannedPayload{
			VenueKey: venueKey,
			Pools:    pools,
		},
	})
}

func publishPriceUpdates(
	ctx context.Context,
	outbox *postgres.OutboxRepository,
	priceService *pricing.Service,
	pools []venue.Pool,
) error {
	for _, symbol := range priceService.SymbolsForPools(pools) {
		payload, err := json.Marshal(pricing.NewSymbolUpdateEvent(symbol))
		if err != nil {
			return err
		}
		if err := outbox.Create(ctx, pricing.UpdatesChannel, symbol, payload); err != nil {
			return err
		}
	}
	return nil
}

func trackedAssetIDs(assets asset.Registry, chainKey chain.ChainKey) []venue.AssetID {
	seen := make(map[venue.AssetID]struct{})
	out := make([]venue.AssetID, 0)
	for _, item := range assets.All() {
		for _, deployment := range item.Deployments {
			if deployment.ChainKey != chainKey {
				continue
			}
			id := venue.AssetID(deployment.AssetID())
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func trackedScanEnabled() bool {
	return os.Getenv("SCANNER_MODE") != "full"
}

func scannerInterval() time.Duration {
	raw := os.Getenv("SCANNER_INTERVAL")
	if raw == "" {
		return 0
	}
	interval, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("Warning: invalid SCANNER_INTERVAL %q: %v", raw, err)
		return 0
	}
	return interval
}

func runVenueScan(ctx context.Context, leases *postgres.LeaseRepository, v venue.Venue, owner string, scan func()) {
	if scan == nil {
		return
	}
	if !scannerLeasesEnabled() || leases == nil {
		scan()
		return
	}

	name := scannerLeaseName(v)
	ttl := scannerLeaseTTL()
	acquired, err := leases.TryAcquire(ctx, name, owner, ttl)
	if err != nil {
		log.Printf("Error: failed to acquire scanner lease %s: %v", name, err)
		return
	}
	if !acquired {
		fmt.Printf("Skipping %s: scanner lease %s is held by another instance\n", v.Key, name)
		return
	}

	stopRenewal := startLeaseRenewal(ctx, leases, name, owner, ttl)
	defer func() {
		stopRenewal()
		if err := leases.Release(context.Background(), name, owner); err != nil {
			log.Printf("Warning: failed to release scanner lease %s: %v", name, err)
		}
	}()
	scan()
}

func startLeaseRenewal(ctx context.Context, leases *postgres.LeaseRepository, name string, owner string, ttl time.Duration) func() {
	interval := ttl / 3
	if interval <= 0 {
		interval = time.Minute
	}
	renewCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				acquired, err := leases.TryAcquire(renewCtx, name, owner, ttl)
				if err != nil {
					log.Printf("Warning: failed to renew scanner lease %s: %v", name, err)
					continue
				}
				if !acquired {
					log.Printf("Warning: scanner lease %s was lost by %s", name, owner)
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func scannerLeaseName(v venue.Venue) string {
	return fmt.Sprintf("scanner:%s:%s", v.ChainKey, v.Key)
}

func scannerLeaseOwner() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

func scannerLeasesEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("SCANNER_LEASES")), "false")
}

func scannerLeaseTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SCANNER_LEASE_TTL"))
	if raw == "" {
		return 5 * time.Minute
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		log.Printf("Warning: invalid SCANNER_LEASE_TTL %q: %v", raw, err)
		return 5 * time.Minute
	}
	return ttl
}
