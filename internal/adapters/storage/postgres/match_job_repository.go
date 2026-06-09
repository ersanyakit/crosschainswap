package postgres

import (
	"context"
	"strings"
	"time"

	"exchange/internal/core/order"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MatchJobStatusPending     = "pending"
	MatchJobStatusProcessing  = "processing"
	MatchJobStatusCompleted   = "completed"
	MatchJobStatusFailed      = "failed"
	MatchJobStatusQuarantined = "quarantined"
)

type MatchJob struct {
	ID       string
	OrderID  order.ID
	Market   string
	Attempts int
}

func (r *ExchangeRepository) CreateMatchJob(ctx context.Context, orderID order.ID, market string) error {
	if strings.TrimSpace(string(orderID)) == "" {
		return nil
	}
	now := time.Now()
	model := ExchangeMatchJob{
		ID:          idgen.New("mjob"),
		OrderID:     string(orderID),
		Market:      strings.ToUpper(strings.TrimSpace(market)),
		Status:      MatchJobStatusPending,
		AvailableAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "order_id"}},
		DoNothing: true,
	}).Create(&model).Error
}

func (r *ExchangeRepository) ClaimMatchJobs(ctx context.Context, workerID string, limit int, lockTTL time.Duration) ([]MatchJob, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "matcher"
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if lockTTL <= 0 {
		lockTTL = 5 * time.Minute
	}

	now := time.Now()
	staleBefore := now.Add(-lockTTL)
	claimed := make([]MatchJob, 0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []ExchangeMatchJob
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status = ? AND available_at <= ?) OR (status = ? AND locked_at <= ?)", MatchJobStatusPending, now, MatchJobStatusProcessing, staleBefore).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}}}}).
			Limit(limit).
			Find(&models).Error
		if err != nil {
			return err
		}

		for _, model := range models {
			model.Status = MatchJobStatusProcessing
			model.Attempts++
			model.LockedBy = workerID
			model.LockedAt = now
			model.UpdatedAt = now
			if err := tx.Save(&model).Error; err != nil {
				return err
			}
			claimed = append(claimed, modelToMatchJob(model))
		}
		return nil
	})
	return claimed, err
}

func (r *ExchangeRepository) CompleteMatchJob(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&ExchangeMatchJob{}).
		Where(&ExchangeMatchJob{ID: id}).
		Updates(map[string]any{
			"status":     MatchJobStatusCompleted,
			"locked_by":  "",
			"locked_at":  time.Time{},
			"last_error": "",
			"updated_at": now,
		}).Error
}

func (r *ExchangeRepository) FailMatchJob(ctx context.Context, id string, message string, maxAttempts int, retryAfter time.Duration) error {
	id = strings.TrimSpace(id)
	if id == "" {
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
		var model ExchangeMatchJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "id = ?", id).Error; err != nil {
			return err
		}

		nextStatus := MatchJobStatusPending
		availableAt := now.Add(retryAfter)
		if model.Attempts >= maxAttempts {
			nextStatus = MatchJobStatusQuarantined
			availableAt = now
		}
		model.Status = nextStatus
		model.AvailableAt = availableAt
		model.LockedBy = ""
		model.LockedAt = time.Time{}
		model.LastError = strings.TrimSpace(message)
		model.UpdatedAt = now
		return tx.Save(&model).Error
	})
}

func modelToMatchJob(model ExchangeMatchJob) MatchJob {
	return MatchJob{
		ID:       model.ID,
		OrderID:  order.ID(model.OrderID),
		Market:   model.Market,
		Attempts: model.Attempts,
	}
}
