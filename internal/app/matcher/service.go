package matcher

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
	"exchange/internal/app/orders"
	"exchange/internal/config"
)

func Run(ctx context.Context) error {
	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}

	db, err := postgres.Connect()
	if err != nil {
		return err
	}
	registries := config.LoadDefaultRegistries()
	repo := postgres.NewExchangeRepository(db)
	outboxRepo := postgres.NewOutboxRepository(db)
	orderService := orders.NewService(registries.Markets, repo)
	orderService.SetPublisher(func(payload []byte) {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := outboxRepo.Create(notifyCtx, orders.UpdatesChannel, "", payload); err != nil {
			slog.Error("failed to enqueue exchange update", "error", err)
		}
	})

	workerID := matcherWorkerID()
	pollInterval := durationEnv("MATCHER_POLL_INTERVAL", 500*time.Millisecond)
	retryDelay := durationEnv("MATCHER_RETRY_DELAY", 2*time.Second)
	lockTTL := durationEnv("MATCHER_JOB_LOCK_TTL", 5*time.Minute)
	batchSize := intEnv("MATCHER_BATCH_SIZE", 50)
	maxAttempts := intEnv("MATCHER_MAX_ATTEMPTS", 5)

	slog.Info("matcher service started", "worker_id", workerID, "batch_size", batchSize, "poll_interval", pollInterval, "job_lock_ttl", lockTTL)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		processed, err := runOnce(ctx, repo, orderService, workerID, batchSize, maxAttempts, retryDelay, lockTTL)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("matcher cycle failed", "error", err)
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

func runOnce(
	ctx context.Context,
	repo *postgres.ExchangeRepository,
	orderService *orders.Service,
	workerID string,
	batchSize int,
	maxAttempts int,
	retryDelay time.Duration,
	lockTTL time.Duration,
) (int, error) {
	jobs, err := repo.ClaimMatchJobs(ctx, workerID, batchSize, lockTTL)
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		if _, err := orderService.MatchOrder(ctx, job.OrderID); err != nil {
			if failErr := repo.FailMatchJob(ctx, job.ID, err.Error(), maxAttempts, retryDelay); failErr != nil {
				return len(jobs), fmt.Errorf("match job %s failed and could not be marked failed: %w", job.ID, failErr)
			}
			slog.Error("match job failed", "job_id", job.ID, "order_id", job.OrderID, "attempts", job.Attempts, "error", err)
			continue
		}
		if err := repo.CompleteMatchJob(ctx, job.ID); err != nil {
			return len(jobs), err
		}
		slog.Info("match job completed", "job_id", job.ID, "order_id", job.OrderID, "market", job.Market)
	}
	return len(jobs), nil
}

func matcherWorkerID() string {
	if value := strings.TrimSpace(os.Getenv("MATCHER_WORKER_ID")); value != "" {
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
