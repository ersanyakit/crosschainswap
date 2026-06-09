package matcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/app/orders"
	"exchange/internal/core/market"
	"exchange/internal/core/matching"
	"exchange/internal/core/order"
	"exchange/internal/core/orderbook"
	"exchange/internal/core/trade"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
)

type marketRuntime struct {
	market       market.Market
	repo         *postgres.ExchangeRepository
	orderService *orders.Service
	book         *matching.MarketBook
	lastSequence uint64
}

type replayMatchEventPayload struct {
	Taker  order.Order            `json:"taker"`
	Makers []order.Order          `json:"makers"`
	Trades []trade.Trade          `json:"trades"`
	Levels []orderbook.PriceLevel `json:"levels,omitempty"`
}

func newMarketRuntime(repo *postgres.ExchangeRepository, orderService *orders.Service, item market.Market) *marketRuntime {
	return &marketRuntime{repo: repo, orderService: orderService, market: item}
}

func (r *marketRuntime) ProcessCommand(ctx context.Context, accepted postgres.OrderCommand, command postgres.OrderCommandLog, saveSnapshot bool) (*orders.MatchResult, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	if accepted.OrderID == "" {
		return nil, fmt.Errorf("command %s has no accepted order id", command.CommandID)
	}
	current, err := r.repo.GetOrder(ctx, order.ID(accepted.OrderID))
	if err != nil {
		return nil, err
	}
	if !runtimeMatchable(current.Status) {
		return &orders.MatchResult{Order: *current}, nil
	}
	if _, ok := r.book.ActiveOrder(current.ID); ok {
		return &orders.MatchResult{Order: *current}, nil
	}

	taker := *current
	if taker.Status == order.StatusPendingMatch {
		taker.Status = order.StatusOpen
		taker.UpdatedAt = time.Now()
	}

	before := r.book.CaptureState(r.lastSequence, time.Now())
	result, err := r.book.Apply(taker, func() trade.ID {
		return trade.ID(idgen.New("trd"))
	}, time.Now())
	if err != nil {
		_ = r.restore(before)
		return nil, err
	}
	result.Taker.Price = taker.Price

	persisted, err := r.orderService.PersistBookMatchResult(ctx, result, command.SequenceID, saveSnapshot)
	if err != nil {
		_ = r.restore(before)
		return nil, err
	}
	if persisted.ReloadBook {
		if err := r.reload(ctx); err != nil {
			return nil, err
		}
	}
	return &persisted.Result, nil
}

func (r *marketRuntime) ensureLoaded(ctx context.Context) error {
	if r.book != nil {
		return nil
	}
	return r.reload(ctx)
}

func (r *marketRuntime) reload(ctx context.Context) error {
	snapshot, err := r.repo.LatestMatcherSnapshot(ctx, r.market.Symbol)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return r.loadActiveProjection(ctx, 0)
	}
	if err := r.validateRecoveryWindow(ctx); err != nil {
		return err
	}

	latestApplied, err := r.repo.LatestAppliedOrderCommandSequence(ctx, r.market.Symbol)
	if err != nil {
		return err
	}
	latestMatchEvent, err := r.repo.LatestMatchEventSequence(ctx, r.market.Symbol)
	if err != nil {
		return err
	}
	if latestApplied > snapshot.LastAppliedSequence && latestMatchEvent >= latestApplied {
		return r.replayMatchEventsFromSnapshot(ctx, *snapshot)
	}
	if latestApplied > snapshot.LastAppliedSequence {
		return r.loadActiveProjection(ctx, latestApplied)
	}
	if latestMatchEvent > snapshot.LastAppliedSequence {
		return r.replayMatchEventsFromSnapshot(ctx, *snapshot)
	}

	return r.restore(*snapshot)
}

func (r *marketRuntime) replayMatchEventsFromSnapshot(ctx context.Context, snapshot matching.BookStateSnapshot) error {
	if err := r.restore(snapshot); err != nil {
		return err
	}
	for {
		events, err := r.repo.ListMatchEventLogsAfter(ctx, r.market.Symbol, r.lastSequence, 1000)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		for _, event := range events {
			var payload replayMatchEventPayload
			if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
				return err
			}
			if err := r.book.ApplyResult(matching.Result{Taker: payload.Taker, Makers: payload.Makers, Trades: payload.Trades}); err != nil {
				return err
			}
			r.lastSequence = event.SequenceID
		}
		if len(events) < 1000 {
			return nil
		}
	}
}

func (r *marketRuntime) loadActiveProjection(ctx context.Context, lastSequence uint64) error {
	active, err := r.repo.ListActiveOrdersForUpdate(ctx, r.market.Symbol, "", 0)
	if err != nil {
		return err
	}
	book := matching.NewMarketBook(r.market.Symbol, r.market.BaseAsset, r.market.QuoteAsset)
	if err := book.Load(active); err != nil {
		return err
	}
	if lastSequence == 0 {
		latestApplied, err := r.repo.LatestAppliedOrderCommandSequence(ctx, r.market.Symbol)
		if err != nil {
			return err
		}
		lastSequence = latestApplied
	}
	r.book = book
	r.lastSequence = lastSequence
	return nil
}

func (r *marketRuntime) validateRecoveryWindow(ctx context.Context) error {
	snapshot, err := r.repo.LatestMatcherSnapshot(ctx, r.market.Symbol)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	oldest, _, err := r.repo.OrderCommandSequenceBounds(ctx, r.market.Symbol)
	if err != nil {
		return err
	}
	if oldest == 0 {
		return nil
	}
	return matching.CheckSnapshotRetention(matching.SnapshotRetentionGuard{
		LatestSnapshotSequence: snapshot.LastAppliedSequence,
		OldestRetainedSequence: oldest,
		SnapshotCreatedAt:      snapshot.CreatedAt,
		Now:                    time.Now(),
		EventRetention:         durationEnv("MATCHER_EVENT_RETENTION", 0),
		MaxSnapshotAge:         durationEnv("MATCHER_MAX_SNAPSHOT_AGE", 0),
	})
}

func (r *marketRuntime) ReplayFromSnapshot(ctx context.Context, limit int) ([]postgres.OrderCommandLog, error) {
	snapshot, err := r.repo.LatestMatcherSnapshot(ctx, r.market.Symbol)
	if err != nil {
		return nil, err
	}
	if err := r.validateRecoveryWindow(ctx); err != nil {
		return nil, err
	}
	book, err := matching.RestoreMarketBook(*snapshot)
	if err != nil {
		return nil, err
	}
	r.book = book
	r.lastSequence = snapshot.LastAppliedSequence
	return r.repo.ListOrderCommandLogsAfter(ctx, r.market.Symbol, snapshot.LastAppliedSequence, limit)
}

func (r *marketRuntime) restore(snapshot matching.BookStateSnapshot) error {
	book, err := matching.RestoreMarketBook(snapshot)
	if err != nil {
		return err
	}
	r.book = book
	r.lastSequence = snapshot.LastAppliedSequence
	return nil
}

func runtimeMatchable(status order.Status) bool {
	switch status {
	case order.StatusPendingMatch, order.StatusOpen, order.StatusPartiallyFilled:
		return true
	default:
		return false
	}
}

func isSnapshotRetentionUnsafe(err error) bool {
	return errors.Is(err, matching.ErrSnapshotRetentionUnsafe)
}
