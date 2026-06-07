package postgres

import (
	"context"
	"strings"
	"time"

	"exchange/pkg/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusPublished  = "published"
	OutboxStatusFailed     = "failed"
)

type OutboxEvent struct {
	ID       string
	Topic    string
	EventKey string
	Payload  []byte
	Attempts int
}

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) Create(ctx context.Context, topic string, eventKey string, payload []byte) error {
	if strings.TrimSpace(topic) == "" || len(payload) == 0 {
		return nil
	}
	now := time.Now()
	model := ExchangeOutboxEvent{
		ID:          idgen.New("out"),
		Topic:       strings.TrimSpace(topic),
		EventKey:    strings.TrimSpace(eventKey),
		Payload:     string(payload),
		Status:      OutboxStatusPending,
		AvailableAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r *OutboxRepository) Claim(ctx context.Context, workerID string, limit int, lockTTL time.Duration) ([]OutboxEvent, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "outbox"
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if lockTTL <= 0 {
		lockTTL = 5 * time.Minute
	}

	now := time.Now()
	staleBefore := now.Add(-lockTTL)
	claimed := make([]OutboxEvent, 0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []ExchangeOutboxEvent
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status = ? AND available_at <= ?) OR (status = ? AND locked_at <= ?)", OutboxStatusPending, now, OutboxStatusProcessing, staleBefore).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "created_at"}}}}).
			Limit(limit).
			Find(&models).Error
		if err != nil {
			return err
		}

		for _, model := range models {
			model.Status = OutboxStatusProcessing
			model.Attempts++
			model.LockedBy = workerID
			model.LockedAt = now
			model.UpdatedAt = now
			if err := tx.Save(&model).Error; err != nil {
				return err
			}
			claimed = append(claimed, modelToOutboxEvent(model))
		}
		return nil
	})
	return claimed, err
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&ExchangeOutboxEvent{}).
		Where(&ExchangeOutboxEvent{ID: id}).
		Updates(map[string]any{
			"status":       OutboxStatusPublished,
			"locked_by":    "",
			"locked_at":    time.Time{},
			"last_error":   "",
			"published_at": now,
			"updated_at":   now,
		}).Error
}

func (r *OutboxRepository) MarkFailed(ctx context.Context, id string, message string, maxAttempts int, retryAfter time.Duration) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model ExchangeOutboxEvent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "id = ?", id).Error; err != nil {
			return err
		}

		nextStatus := OutboxStatusPending
		availableAt := now.Add(retryAfter)
		if model.Attempts >= maxAttempts {
			nextStatus = OutboxStatusFailed
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

func (r *OutboxRepository) Notify(ctx context.Context, event OutboxEvent) error {
	if strings.TrimSpace(event.Topic) == "" || len(event.Payload) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", event.Topic, string(event.Payload)).Error; err != nil {
		return err
	}
	return nil
}

func modelToOutboxEvent(model ExchangeOutboxEvent) OutboxEvent {
	return OutboxEvent{
		ID:       model.ID,
		Topic:    model.Topic,
		EventKey: model.EventKey,
		Payload:  []byte(model.Payload),
		Attempts: model.Attempts,
	}
}
