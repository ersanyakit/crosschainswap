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
	"exchange/internal/core/market"
	"exchange/internal/core/order"

	"gorm.io/gorm"
)

func Run(ctx context.Context) error {
	if err := postgres.LoadEnv("."); err != nil {
		slog.Warn("failed to load .env file", "error", err)
	}

	db, err := postgres.Connect()
	if err != nil {
		return err
	}
	registries := config.LoadRegistries(ctx)
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

	if commandLogMatchingEnabled() {
		return runCommandLogLoop(ctx, repo, orderService, registries.Markets, workerID, batchSize, maxAttempts, retryDelay, lockTTL, pollInterval)
	}

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

func runCommandLogLoop(
	ctx context.Context,
	repo *postgres.ExchangeRepository,
	orderService *orders.Service,
	markets market.Registry,
	workerID string,
	batchSize int,
	maxAttempts int,
	retryDelay time.Duration,
	lockTTL time.Duration,
	pollInterval time.Duration,
) error {
	marketSymbols := enabledMarketSymbols(markets)
	runtimes := map[string]*marketRuntime{}
	if residentRuntimeEnabled() && bookMatchingEnabled() {
		for _, item := range markets.All() {
			if item.Symbol == "" || !item.Enabled {
				continue
			}
			runtimes[item.Symbol] = newMarketRuntime(repo, orderService, item)
		}
	}
	slog.Info("command-log matcher service started", "worker_id", workerID, "markets", marketSymbols, "batch_size", batchSize, "poll_interval", pollInterval, "job_lock_ttl", lockTTL)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		processed := 0
		for _, marketSymbol := range marketSymbols {
			count, err := runCommandLogOnceWithRuntime(ctx, repo, orderService, marketSymbol, workerID, batchSize, maxAttempts, retryDelay, lockTTL, runtimes[marketSymbol])
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("command-log matcher cycle failed", "market", marketSymbol, "error", err)
			}
			if errors.Is(err, context.Canceled) {
				return err
			}
			processed += count
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

func runCommandLogOnce(
	ctx context.Context,
	repo *postgres.ExchangeRepository,
	orderService *orders.Service,
	marketSymbol string,
	workerID string,
	batchSize int,
	maxAttempts int,
	retryDelay time.Duration,
	lockTTL time.Duration,
) (int, error) {
	return runCommandLogOnceWithRuntime(ctx, repo, orderService, marketSymbol, workerID, batchSize, maxAttempts, retryDelay, lockTTL, nil)
}

func runCommandLogOnceWithRuntime(
	ctx context.Context,
	repo *postgres.ExchangeRepository,
	orderService *orders.Service,
	marketSymbol string,
	workerID string,
	batchSize int,
	maxAttempts int,
	retryDelay time.Duration,
	lockTTL time.Duration,
	runtime *marketRuntime,
) (int, error) {
	var afterSequence uint64
	recovered := 0
	if runtime != nil {
		if err := runtime.ensureLoaded(ctx); err != nil {
			return 0, err
		}
		afterSequence = runtime.lastSequence
		count, err := completeRecoveredCommandLogs(ctx, repo, marketSymbol, runtime, afterSequence)
		if err != nil {
			return 0, err
		}
		recovered = count
		if recovered > 0 {
			afterSequence = runtime.lastSequence
		}
	}
	commands, err := repo.ClaimOrderCommandLogsAfter(ctx, marketSymbol, workerID, batchSize, lockTTL, afterSequence)
	if err != nil {
		return 0, err
	}
	for _, command := range commands {
		if err := matchCommandLogSafelyWithRuntime(ctx, repo, orderService, command, runtime); err != nil {
			if failErr := repo.FailOrderCommandLog(ctx, command.CommandID, err.Error(), maxAttempts, retryDelay); failErr != nil {
				return len(commands), fmt.Errorf("order command log %s failed and could not be marked failed: %w", command.CommandID, failErr)
			}
			slog.Error("order command log failed", "command_id", command.CommandID, "sequence_id", command.SequenceID, "market", command.Market, "attempts", command.Attempts, "error", err)
			continue
		}
		slog.Info("order command log applied", "command_id", command.CommandID, "sequence_id", command.SequenceID, "market", command.Market)
	}
	return recovered + len(commands), nil
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
		if err := matchOrderSafely(ctx, orderService, job.OrderID); err != nil {
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

func matchCommandLogSafely(ctx context.Context, repo *postgres.ExchangeRepository, orderService *orders.Service, command postgres.OrderCommandLog) (err error) {
	return matchCommandLogSafelyWithRuntime(ctx, repo, orderService, command, nil)
}

func matchCommandLogSafelyWithRuntime(ctx context.Context, repo *postgres.ExchangeRepository, orderService *orders.Service, command postgres.OrderCommandLog, runtime *marketRuntime) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("matcher panic while processing command %s: %v", command.CommandID, recovered)
		}
	}()
	return matchCommandLogWithRuntime(ctx, repo, orderService, command, runtime)
}

func matchCommandLog(ctx context.Context, repo *postgres.ExchangeRepository, orderService *orders.Service, command postgres.OrderCommandLog) error {
	return matchCommandLogWithRuntime(ctx, repo, orderService, command, nil)
}

func matchCommandLogWithRuntime(ctx context.Context, repo *postgres.ExchangeRepository, orderService *orders.Service, command postgres.OrderCommandLog, runtime *marketRuntime) error {
	if command.Type != "" && command.Type != postgres.OrderCommandTypeNewOrder {
		if err := repo.RejectOrderCommandLog(ctx, command.CommandID, "unsupported command type"); err != nil {
			return err
		}
		return nil
	}
	accepted, err := repo.GetOrderCommand(ctx, command.CommandID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(accepted.OrderID) == "" {
		return fmt.Errorf("command %s has no accepted order id", command.CommandID)
	}
	if completed, err := completeCommandFromExistingMatchEvent(ctx, repo, command, *accepted); err != nil {
		return err
	} else if completed {
		if runtime != nil && command.SequenceID > runtime.lastSequence {
			runtime.lastSequence = command.SequenceID
		}
		return nil
	}
	var result *orders.MatchResult
	if runtime != nil {
		result, err = runtime.ProcessCommand(ctx, *accepted, command, snapshotEnabled(command.SequenceID))
	} else if bookMatchingEnabled() {
		result, err = orderService.MatchOrderWithBookAtSequence(ctx, order.ID(accepted.OrderID), command.SequenceID, snapshotEnabled(command.SequenceID))
	} else {
		result, err = orderService.MatchOrder(ctx, order.ID(accepted.OrderID))
	}
	if err != nil {
		return err
	}
	if result == nil || result.Order.ID == "" {
		return fmt.Errorf("command %s produced no match result", command.CommandID)
	}
	if err := repo.CompleteOrderCommand(ctx, command.CommandID, string(result.Order.ID), string(result.Order.Status)); err != nil {
		return err
	}
	if err := repo.ApplyOrderCommandLog(ctx, command.CommandID, string(result.Order.ID)); err != nil {
		return err
	}
	if runtime != nil && command.SequenceID > runtime.lastSequence {
		runtime.lastSequence = command.SequenceID
	}
	return nil
}

func completeRecoveredCommandLogs(ctx context.Context, repo *postgres.ExchangeRepository, marketSymbol string, runtime *marketRuntime, throughSequence uint64) (int, error) {
	if runtime == nil || throughSequence == 0 {
		return 0, nil
	}
	recovered := 0
	for {
		commands, err := repo.ListUnappliedOrderCommandLogsThrough(ctx, marketSymbol, throughSequence, 1000)
		if err != nil {
			return recovered, err
		}
		if len(commands) == 0 {
			return recovered, nil
		}
		for _, command := range commands {
			accepted, err := repo.GetOrderCommand(ctx, command.CommandID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return recovered, err
			}
			completed, err := completeCommandFromExistingMatchEvent(ctx, repo, command, *accepted)
			if err != nil {
				return recovered, err
			}
			if completed {
				recovered++
			}
		}
		if len(commands) < 1000 {
			return recovered, nil
		}
	}
}

func completeCommandFromExistingMatchEvent(ctx context.Context, repo *postgres.ExchangeRepository, command postgres.OrderCommandLog, accepted postgres.OrderCommand) (bool, error) {
	if command.SequenceID == 0 {
		return false, nil
	}
	if _, err := repo.GetMatchEventLog(ctx, command.Market, command.SequenceID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(accepted.OrderID) == "" {
		return false, fmt.Errorf("command %s has match event but no accepted order id", command.CommandID)
	}
	current, err := repo.GetOrder(ctx, order.ID(accepted.OrderID))
	if err != nil {
		return false, err
	}
	if err := repo.CompleteOrderCommand(ctx, command.CommandID, string(current.ID), string(current.Status)); err != nil {
		return false, err
	}
	if err := repo.ApplyOrderCommandLog(ctx, command.CommandID, string(current.ID)); err != nil {
		return false, err
	}
	return true, nil
}

func matchOrderSafely(ctx context.Context, orderService *orders.Service, orderID order.ID) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("matcher panic while processing order %s: %v", orderID, recovered)
		}
	}()
	_, err = orderService.MatchOrder(ctx, orderID)
	return err
}

func commandLogMatchingEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MATCHING_MODE")), "command_log")
}

func bookMatchingEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MATCHING_ENGINE")), "book")
}

func residentRuntimeEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MATCHING_RUNTIME")), "resident")
}

func snapshotEnabled(sequence uint64) bool {
	every := intEnv("MATCHER_SNAPSHOT_EVERY", 0)
	return every > 0 && sequence > 0 && sequence%uint64(every) == 0
}

func enabledMarketSymbols(markets market.Registry) []string {
	items := markets.All()
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Symbol == "" || !item.Enabled {
			continue
		}
		out = append(out, item.Symbol)
	}
	return out
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
