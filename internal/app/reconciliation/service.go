package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/config"
	"exchange/internal/core/market"
)

type Report struct {
	GeneratedAt         time.Time                           `json:"generated_at"`
	OrderBookDriftCount int                                 `json:"orderbook_drift_count"`
	OrderBookDrift      []postgres.OrderBookProjectionDrift `json:"orderbook_drift"`
}

func Run(ctx context.Context, out io.Writer) error {
	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}
	db, err := postgres.Connect()
	if err != nil {
		return err
	}
	repo := postgres.NewExchangeRepository(db)
	report, err := Check(ctx, repo, config.LoadRegistries(ctx).Markets)
	if err != nil {
		return err
	}
	if out == nil {
		out = os.Stdout
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	if failOnDrift() && report.OrderBookDriftCount > 0 {
		return fmt.Errorf("reconciliation found %d orderbook projection drift rows", report.OrderBookDriftCount)
	}
	return nil
}

func Check(ctx context.Context, repo *postgres.ExchangeRepository, markets market.Registry) (Report, error) {
	report := Report{GeneratedAt: time.Now().UTC()}
	if repo == nil {
		return report, nil
	}
	for _, item := range markets.All() {
		if item.Symbol == "" || !item.Enabled {
			continue
		}
		drift, err := repo.ReconcileOrderBookProjection(ctx, item.Symbol)
		if err != nil {
			return report, err
		}
		report.OrderBookDrift = append(report.OrderBookDrift, drift...)
	}
	report.OrderBookDriftCount = len(report.OrderBookDrift)
	return report, nil
}

func failOnDrift() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("RECONCILE_FAIL_ON_DRIFT")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}
