package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"exchange/internal/adapters/eventstream"
	"exchange/internal/adapters/storage/postgres"
)

func Run(ctx context.Context) error {
	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}

	db, err := postgres.Connect()
	if err != nil {
		return err
	}
	outbox := postgres.NewOutboxRepository(db)
	bus, err := eventstream.NewFromEnv(db)
	if err != nil {
		return err
	}
	defer func() {
		if err := bus.Close(); err != nil {
			slog.Error("event backend close failed", "error", err)
		}
	}()

	workerID := workerID()
	pollInterval := durationEnv("OUTBOX_POLL_INTERVAL", 500*time.Millisecond)
	retryDelay := durationEnv("OUTBOX_RETRY_DELAY", 2*time.Second)
	lockTTL := durationEnv("OUTBOX_LOCK_TTL", 5*time.Minute)
	batchSize := intEnv("OUTBOX_BATCH_SIZE", 100)
	maxAttempts := intEnv("OUTBOX_MAX_ATTEMPTS", 10)

	slog.Info("outbox worker started", "worker_id", workerID, "batch_size", batchSize, "poll_interval", pollInterval, "lock_ttl", lockTTL)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		processed, err := runOnce(ctx, outbox, bus, workerID, batchSize, maxAttempts, retryDelay, lockTTL)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("outbox worker cycle failed", "error", err)
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
	outbox *postgres.OutboxRepository,
	bus eventstream.Bus,
	workerID string,
	batchSize int,
	maxAttempts int,
	retryDelay time.Duration,
	lockTTL time.Duration,
) (int, error) {
	events, err := outbox.Claim(ctx, workerID, batchSize, lockTTL)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		if err := bus.Publish(ctx, eventstream.Message{Topic: event.Topic, Key: event.EventKey, Payload: event.Payload}); err != nil {
			if failErr := outbox.MarkFailed(ctx, event.ID, err.Error(), maxAttempts, retryDelay); failErr != nil {
				return len(events), fmt.Errorf("outbox event %s failed and could not be marked failed: %w", event.ID, failErr)
			}
			slog.Error("outbox event publish failed", "event_id", event.ID, "topic", event.Topic, "attempts", event.Attempts, "error", err)
			continue
		}
		if err := outbox.MarkPublished(ctx, event.ID); err != nil {
			return len(events), err
		}
		slog.Info("outbox event published", "event_id", event.ID, "topic", event.Topic, "key", event.EventKey)
	}
	return len(events), nil
}

func workerID() string {
	if value := strings.TrimSpace(os.Getenv("OUTBOX_WORKER_ID")); value != "" {
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
