package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/app/apiruntime"
	appmarketdata "exchange/internal/app/marketdata"
	appmatcher "exchange/internal/app/matcher"
	"exchange/internal/app/poolscanner"
	appworker "exchange/internal/app/worker"
	"exchange/internal/config"
)

type Service struct {
	Name     string
	Interval time.Duration
	RunOnce  func(context.Context, config.Registries) error
	Run      func(context.Context) error
}

type Options struct {
	FrontendMode           string
	FrontendDir            string
	FrontendHost           string
	FrontendPort           string
	FrontendPackageManager string
}

func RunAll(processName string) error {
	return RunAllWithOptions(processName, DefaultOptions(processName))
}

func RunPlaceholder(processName string, interval time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}
	registries := config.LoadRegistries(ctx)
	printRegistrySummary(processName, registries)
	return runService(ctx, registries, Service{
		Name:     processName,
		Interval: interval,
		RunOnce:  heartbeat(processName),
	})
}

func RunAllWithOptions(processName string, opts Options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}

	registries := config.LoadRegistries(ctx)
	printRegistrySummary(processName, registries)

	services := []Service{
		{Name: "api", Run: apiruntime.Run},
		{Name: "indexer", Interval: 10 * time.Second, RunOnce: heartbeat("indexer")},
		{Name: "matcher", Run: appmatcher.Run},
		{Name: "marketdata", Run: appmarketdata.Run},
		{Name: "executor", Interval: 3 * time.Second, RunOnce: heartbeat("executor")},
		{Name: "settler", Interval: 5 * time.Second, RunOnce: heartbeat("settler")},
		{Name: "scheduler", Interval: 15 * time.Second, RunOnce: heartbeat("scheduler")},
		{Name: "worker", Run: appworker.Run},
		{Name: "scanner", Run: runPoolScanner},
	}
	if frontendEnabled(opts.FrontendMode) {
		frontendOpts := opts
		services = append(services, Service{Name: "frontend", Run: func(ctx context.Context) error {
			return runFrontend(ctx, frontendOpts)
		}})
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

func DefaultOptions(processName string) Options {
	mode := envOrDefault("FRONTEND_MODE", "off")
	if strings.EqualFold(processName, "executor") && os.Getenv("FRONTEND_MODE") == "" {
		mode = "dev"
	}
	return Options{
		FrontendMode:           mode,
		FrontendDir:            envOrDefault("FRONTEND_DIR", "frontend"),
		FrontendHost:           envOrDefault("FRONTEND_HOST", "0.0.0.0"),
		FrontendPort:           envOrDefault("FRONTEND_PORT", "3002"),
		FrontendPackageManager: envOrDefault("FRONTEND_PACKAGE_MANAGER", "npm"),
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
		if err := os.Setenv("SCANNER_INTERVAL", "1s"); err != nil {
			return err
		}
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
