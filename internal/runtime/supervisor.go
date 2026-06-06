package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/app/apiruntime"
	"exchange/internal/app/poolscanner"
	"exchange/internal/config"
)

type Service struct {
	Name     string
	Interval time.Duration
	RunOnce  func(context.Context, config.Registries) error
	Run      func(context.Context) error
}

func RunAll(processName string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}

	registries := config.LoadDefaultRegistries()
	printRegistrySummary(processName, registries)

	services := []Service{
		{Name: "api", Run: apiruntime.Run},
		{Name: "indexer", Interval: 10 * time.Second, RunOnce: heartbeat("indexer")},
		{Name: "matcher", Interval: 2 * time.Second, RunOnce: heartbeat("matcher")},
		{Name: "executor", Interval: 3 * time.Second, RunOnce: heartbeat("executor")},
		{Name: "settler", Interval: 5 * time.Second, RunOnce: heartbeat("settler")},
		{Name: "scheduler", Interval: 15 * time.Second, RunOnce: heartbeat("scheduler")},
		{Name: "worker", Interval: 20 * time.Second, RunOnce: heartbeat("worker")},
		{Name: "scanner", Run: runPoolScanner},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(services))
	for _, svc := range services {
		svc := svc
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runService(ctx, registries, svc); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("%s: %w", svc.Name, err)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-ctx.Done():
		<-done
		return nil
	case err := <-errCh:
		stop()
		<-done
		return err
	}
}

func runService(ctx context.Context, registries config.Registries, svc Service) error {
	slog.Info("service started", "service", svc.Name)
	defer slog.Info("service stopped", "service", svc.Name)

	if svc.Run != nil {
		return svc.Run(ctx)
	}

	if err := svc.RunOnce(ctx, registries); err != nil {
		return err
	}
	ticker := time.NewTicker(svc.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := svc.RunOnce(ctx, registries); err != nil {
				return err
			}
		}
	}
}

func runPoolScanner(ctx context.Context) error {
	if os.Getenv("SCANNER_INTERVAL") == "" {
		os.Setenv("SCANNER_INTERVAL", "1s")
	}
	return poolscanner.Run(ctx)
}

func heartbeat(name string) func(context.Context, config.Registries) error {
	return func(ctx context.Context, registries config.Registries) error {
		slog.Info("heartbeat", "service", name, "chains", len(registries.Chains.All()), "assets", len(registries.Assets.All()), "markets", len(registries.Markets.All()))
		return nil
	}
}

func printRegistrySummary(processName string, registries config.Registries) {
	slog.Info("exchange runtime booted", "process", processName)
	for _, c := range registries.Chains.All() {
		chainID := ""
		if c.ChainID != nil {
			chainID = fmt.Sprintf("%d", *c.ChainID)
		}
		slog.Info("chain registered", "key", c.Key, "name", c.Name, "kind", c.Kind, "chain_id", chainID, "network", c.Network)
	}
	if pepper, ok := registries.Assets.Get("PEPPER"); ok {
		for _, d := range pepper.Deployments {
			slog.Info("PEPPER deployment registered", "chain", d.ChainKey, "address", d.Address, "mint", d.Mint, "decimals", d.Decimals, "enabled", d.Enabled)
		}
	}
}
