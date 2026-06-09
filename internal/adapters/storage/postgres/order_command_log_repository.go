package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"exchange/pkg/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OrderCommandLogStatusPending     = "pending"
	OrderCommandLogStatusProcessing  = "processing"
	OrderCommandLogStatusApplied     = "applied"
	OrderCommandLogStatusRejected    = "rejected"
	OrderCommandLogStatusQuarantined = "quarantined"
)

type OrderCommandLog struct {
	ID             string
	CommandID      string
	SequenceID     uint64
	Market         string
	Type           string
	Key            string
	PayloadHash    string
	Payload        string
	Status         string
	Attempts       int
	AvailableAt    time.Time
	LockedBy       string
	LockedAt       time.Time
	LastError      string
	AppliedOrderID string
	AppliedAt      time.Time
	QuarantinedAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (r *ExchangeRepository) AppendOrderCommandLog(ctx context.Context, item OrderCommandLog) (*OrderCommandLog, error) {
	item.CommandID = strings.TrimSpace(item.CommandID)
	item.Market = strings.ToUpper(strings.TrimSpace(item.Market))
	item.Type = strings.TrimSpace(item.Type)
	item.Key = strings.TrimSpace(item.Key)
	item.PayloadHash = strings.TrimSpace(item.PayloadHash)
	item.Payload = strings.TrimSpace(item.Payload)
	if item.ID == "" {
		item.ID = idgen.New("ocl")
	}
	if item.Type == "" {
		item.Type = OrderCommandTypeNewOrder
	}
	if item.Status == "" {
		item.Status = OrderCommandLogStatusPending
	}
	if item.CommandID == "" || item.Market == "" || item.PayloadHash == "" || item.Payload == "" {
		return nil, fmt.Errorf("order command log requires command id, market, payload hash and payload")
	}

	var out OrderCommandLog
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &ExchangeRepository{db: tx}
		var existing ExchangeOrderCommandLog
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&existing, "command_id = ?", item.CommandID).Error
		if err == nil {
			if existing.PayloadHash != item.PayloadHash {
				return fmt.Errorf("order command log payload conflict for %s", item.CommandID)
			}
			out = modelToOrderCommandLog(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		seq, err := txRepo.nextOrderCommandSequence(ctx, item.Market)
		if err != nil {
			return err
		}
		now := time.Now()
		model := ExchangeOrderCommandLog{
			ID:          strings.TrimSpace(item.ID),
			CommandID:   item.CommandID,
			SequenceID:  seq,
			Market:      item.Market,
			Type:        item.Type,
			Key:         item.Key,
			PayloadHash: item.PayloadHash,
			Payload:     item.Payload,
			Status:      item.Status,
			Attempts:    item.Attempts,
			AvailableAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if !item.AvailableAt.IsZero() {
			model.AvailableAt = item.AvailableAt
		}
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		out = modelToOrderCommandLog(model)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *ExchangeRepository) ClaimOrderCommandLogs(ctx context.Context, market string, workerID string, limit int, lockTTL time.Duration) ([]OrderCommandLog, error) {
	return r.ClaimOrderCommandLogsAfter(ctx, market, workerID, limit, lockTTL, 0)
}

func (r *ExchangeRepository) ClaimOrderCommandLogsAfter(ctx context.Context, market string, workerID string, limit int, lockTTL time.Duration, afterSequence uint64) ([]OrderCommandLog, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return nil, fmt.Errorf("market is required for command log claim")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "matcher"
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if lockTTL <= 0 {
		lockTTL = 5 * time.Minute
	}

	now := time.Now()
	staleBefore := now.Add(-lockTTL)
	claimed := make([]OrderCommandLog, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []ExchangeOrderCommandLog
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(&ExchangeOrderCommandLog{Market: market}).
			Where("(status = ? AND available_at <= ?) OR (status = ? AND locked_at <= ?)", OrderCommandLogStatusPending, now, OrderCommandLogStatusProcessing, staleBefore).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}}}}).
			Limit(limit)
		if afterSequence > 0 {
			query = query.Where("sequence_id > ?", afterSequence)
		}
		if err := query.Find(&models).Error; err != nil {
			return err
		}
		for _, model := range models {
			model.Status = OrderCommandLogStatusProcessing
			model.Attempts++
			model.LockedBy = workerID
			model.LockedAt = now
			model.UpdatedAt = now
			if err := tx.Save(&model).Error; err != nil {
				return err
			}
			claimed = append(claimed, modelToOrderCommandLog(model))
		}
		return nil
	})
	return claimed, err
}

func (r *ExchangeRepository) ListOrderCommandLogsAfter(ctx context.Context, market string, afterSequence uint64, limit int) ([]OrderCommandLog, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	var models []ExchangeOrderCommandLog
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrderCommandLog{Market: market}).
		Where("sequence_id > ?", afterSequence).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]OrderCommandLog, 0, len(models))
	for _, model := range models {
		out = append(out, modelToOrderCommandLog(model))
	}
	return out, nil
}

func (r *ExchangeRepository) ListUnappliedOrderCommandLogsThrough(ctx context.Context, market string, throughSequence uint64, limit int) ([]OrderCommandLog, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" || throughSequence == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	var models []ExchangeOrderCommandLog
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrderCommandLog{Market: market}).
		Where("sequence_id <= ?", throughSequence).
		Where("status IN ?", []string{OrderCommandLogStatusPending, OrderCommandLogStatusProcessing}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]OrderCommandLog, 0, len(models))
	for _, model := range models {
		out = append(out, modelToOrderCommandLog(model))
	}
	return out, nil
}

func (r *ExchangeRepository) OrderCommandSequenceBounds(ctx context.Context, market string) (uint64, uint64, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return 0, 0, nil
	}
	type row struct {
		Oldest uint64
		Latest uint64
	}
	var out row
	err := r.db.WithContext(ctx).
		Model(&ExchangeOrderCommandLog{}).
		Select("COALESCE(MIN(sequence_id), 0) AS oldest, COALESCE(MAX(sequence_id), 0) AS latest").
		Where(&ExchangeOrderCommandLog{Market: market}).
		Scan(&out).Error
	if err != nil {
		return 0, 0, err
	}
	return out.Oldest, out.Latest, nil
}

func (r *ExchangeRepository) LatestAppliedOrderCommandSequence(ctx context.Context, market string) (uint64, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return 0, nil
	}
	var model ExchangeOrderCommandLog
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrderCommandLog{Market: market, Status: OrderCommandLogStatusApplied}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}, Desc: true}}}).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return model.SequenceID, nil
}

func (r *ExchangeRepository) ApplyOrderCommandLog(ctx context.Context, commandID string, orderID string) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&ExchangeOrderCommandLog{}).
		Where(&ExchangeOrderCommandLog{CommandID: commandID}).
		Updates(map[string]any{
			"status":           OrderCommandLogStatusApplied,
			"applied_order_id": strings.TrimSpace(orderID),
			"applied_at":       now,
			"locked_by":        "",
			"locked_at":        time.Time{},
			"last_error":       "",
			"updated_at":       now,
		}).Error
}

func (r *ExchangeRepository) RejectOrderCommandLog(ctx context.Context, commandID string, message string) error {
	return r.updateOrderCommandLogTerminal(ctx, commandID, OrderCommandLogStatusRejected, "", message, time.Time{})
}

func (r *ExchangeRepository) QuarantineOrderCommandLog(ctx context.Context, commandID string, message string) error {
	return r.updateOrderCommandLogTerminal(ctx, commandID, OrderCommandLogStatusQuarantined, "quarantined_at", message, time.Now())
}

func (r *ExchangeRepository) FailOrderCommandLog(ctx context.Context, commandID string, message string, maxAttempts int, retryAfter time.Duration) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return nil
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model ExchangeOrderCommandLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "command_id = ?", commandID).Error; err != nil {
			return err
		}
		model.Status = OrderCommandLogStatusPending
		model.AvailableAt = now.Add(retryAfter)
		if model.Attempts >= maxAttempts {
			model.Status = OrderCommandLogStatusQuarantined
			model.QuarantinedAt = now
			model.AvailableAt = now
		}
		model.LockedBy = ""
		model.LockedAt = time.Time{}
		model.LastError = strings.TrimSpace(message)
		model.UpdatedAt = now
		return tx.Save(&model).Error
	})
}

func (r *ExchangeRepository) updateOrderCommandLogTerminal(ctx context.Context, commandID string, status string, timeField string, message string, when time.Time) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return nil
	}
	now := time.Now()
	values := map[string]any{
		"status":     status,
		"locked_by":  "",
		"locked_at":  time.Time{},
		"last_error": strings.TrimSpace(message),
		"updated_at": now,
	}
	if timeField != "" {
		values[timeField] = when
	}
	return r.db.WithContext(ctx).
		Model(&ExchangeOrderCommandLog{}).
		Where(&ExchangeOrderCommandLog{CommandID: commandID}).
		Updates(values).Error
}

func (r *ExchangeRepository) nextOrderCommandSequence(ctx context.Context, market string) (uint64, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return 0, fmt.Errorf("market is required for command sequence")
	}
	for attempt := 0; attempt < 2; attempt++ {
		var model ExchangeOrderCommandSequence
		err := r.db.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(&ExchangeOrderCommandSequence{Market: market}).
			First(&model).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, err
			}
			maxSeq, err := r.maxOrderCommandSequence(ctx, market)
			if err != nil {
				return 0, err
			}
			seq := maxSeq + 1
			if seq == 0 {
				seq = 1
			}
			model = ExchangeOrderCommandSequence{Market: market, NextSequence: seq + 1, UpdatedAt: time.Now()}
			if createErr := r.db.WithContext(ctx).Create(&model).Error; createErr != nil {
				if attempt == 0 {
					continue
				}
				return 0, createErr
			}
			return seq, nil
		}

		seq := model.NextSequence
		if seq == 0 {
			seq = 1
		}
		model.NextSequence = seq + 1
		model.UpdatedAt = time.Now()
		if err := r.db.WithContext(ctx).Save(&model).Error; err != nil {
			return 0, err
		}
		return seq, nil
	}
	return 0, fmt.Errorf("failed to allocate order command sequence for %s", market)
}

func (r *ExchangeRepository) maxOrderCommandSequence(ctx context.Context, market string) (uint64, error) {
	var model ExchangeOrderCommandLog
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrderCommandLog{Market: market}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}, Desc: true}}}).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return model.SequenceID, nil
}

func modelToOrderCommandLog(model ExchangeOrderCommandLog) OrderCommandLog {
	return OrderCommandLog{
		ID:             model.ID,
		CommandID:      model.CommandID,
		SequenceID:     model.SequenceID,
		Market:         model.Market,
		Type:           model.Type,
		Key:            model.Key,
		PayloadHash:    model.PayloadHash,
		Payload:        model.Payload,
		Status:         model.Status,
		Attempts:       model.Attempts,
		AvailableAt:    model.AvailableAt,
		LockedBy:       model.LockedBy,
		LockedAt:       model.LockedAt,
		LastError:      model.LastError,
		AppliedOrderID: model.AppliedOrderID,
		AppliedAt:      model.AppliedAt,
		QuarantinedAt:  model.QuarantinedAt,
		CreatedAt:      model.CreatedAt,
		UpdatedAt:      model.UpdatedAt,
	}
}
