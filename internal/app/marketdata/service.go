package marketdata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/config"
	"exchange/internal/core/market"

	"gorm.io/gorm"
)

const candleProjectionName = "candles"

func Run(ctx context.Context) error {
	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}

	db, err := postgres.Connect()
	if err != nil {
		return err
	}
	repo := postgres.NewExchangeRepository(db)
	markets := config.LoadRegistries(ctx).Markets

	workerID := marketDataWorkerID()
	pollInterval := durationEnv("MARKETDATA_POLL_INTERVAL", 500*time.Millisecond)
	batchSize := intEnv("MARKETDATA_BATCH_SIZE", 1000)

	slog.Info("marketdata candle worker started", "worker_id", workerID, "batch_size", batchSize, "poll_interval", pollInterval)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		processed, err := RunOnce(ctx, repo, markets, batchSize)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("marketdata candle worker cycle failed", "error", err)
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		if processed > 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func RunOnce(ctx context.Context, repo *postgres.ExchangeRepository, markets market.Registry, batchSize int) (int, error) {
	if repo == nil {
		return 0, nil
	}
	if batchSize <= 0 || batchSize > 10000 {
		batchSize = 1000
	}
	processed := 0
	for _, item := range markets.All() {
		if item.Symbol == "" || !item.Enabled {
			continue
		}
		count, err := processMarketCandles(ctx, repo, item.Symbol, batchSize)
		if err != nil {
			return processed, err
		}
		processed += count
	}
	return processed, nil
}

func processMarketCandles(ctx context.Context, repo *postgres.ExchangeRepository, marketSymbol string, batchSize int) (int, error) {
	offset, err := repo.GetProjectionOffset(ctx, candleProjectionName, marketSymbol)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
		offset = &postgres.ProjectionOffset{Projection: candleProjectionName, Market: marketSymbol, Stream: "exchange_trades"}
	}
	trades, err := repo.ListTradesAfter(ctx, marketSymbol, offset.LastEventAt, offset.LastEventID, batchSize)
	if err != nil {
		return 0, err
	}
	if len(trades) == 0 {
		return 0, nil
	}

	err = repo.Transaction(ctx, func(tx *postgres.ExchangeRepository) error {
		if err := tx.UpdateCandles(ctx, trades); err != nil {
			return err
		}
		last := trades[len(trades)-1]
		_, err := tx.AdvanceProjectionOffset(ctx, postgres.ProjectionOffset{
			Projection:   candleProjectionName,
			Market:       marketSymbol,
			Stream:       "exchange_trades",
			LastSequence: offset.LastSequence + uint64(len(trades)),
			LastEventID:  string(last.ID),
			LastEventAt:  last.CreatedAt,
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	return len(trades), nil
}

func marketDataWorkerID() string {
	if value := strings.TrimSpace(os.Getenv("MARKETDATA_WORKER_ID")); value != "" {
		return value
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func intEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
