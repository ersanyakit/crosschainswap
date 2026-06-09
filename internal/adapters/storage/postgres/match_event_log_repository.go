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

const MatchEventTypeResult = "match_result"

type MatchEventLog struct {
	ID          string
	Market      string
	SequenceID  uint64
	Type        string
	PayloadHash string
	Payload     string
	CreatedAt   time.Time
}

func (r *ExchangeRepository) AppendMatchEventLog(ctx context.Context, item MatchEventLog) (*MatchEventLog, error) {
	item.Market = strings.ToUpper(strings.TrimSpace(item.Market))
	item.Type = strings.TrimSpace(item.Type)
	item.PayloadHash = strings.TrimSpace(item.PayloadHash)
	item.Payload = strings.TrimSpace(item.Payload)
	if item.ID == "" {
		item.ID = idgen.New("mel")
	}
	if item.Type == "" {
		item.Type = MatchEventTypeResult
	}
	if item.Market == "" || item.SequenceID == 0 || item.PayloadHash == "" || item.Payload == "" {
		return nil, fmt.Errorf("match event log requires market, sequence id, payload hash and payload")
	}

	var out MatchEventLog
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing ExchangeMatchEventLog
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&existing, "market = ? AND sequence_id = ? AND type = ?", item.Market, item.SequenceID, item.Type).Error
		if err == nil {
			if existing.PayloadHash != item.PayloadHash {
				return fmt.Errorf("match event log payload conflict for %s/%d/%s", item.Market, item.SequenceID, item.Type)
			}
			out = modelToMatchEventLog(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := time.Now()
		model := ExchangeMatchEventLog{
			ID:          item.ID,
			Market:      item.Market,
			SequenceID:  item.SequenceID,
			Type:        item.Type,
			PayloadHash: item.PayloadHash,
			Payload:     item.Payload,
			CreatedAt:   now,
		}
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		out = modelToMatchEventLog(model)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *ExchangeRepository) ListMatchEventLogsAfter(ctx context.Context, market string, afterSequence uint64, limit int) ([]MatchEventLog, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	var models []ExchangeMatchEventLog
	err := r.db.WithContext(ctx).
		Where(&ExchangeMatchEventLog{Market: market, Type: MatchEventTypeResult}).
		Where("sequence_id > ?", afterSequence).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}}}}).
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]MatchEventLog, 0, len(models))
	for _, model := range models {
		out = append(out, modelToMatchEventLog(model))
	}
	return out, nil
}

func (r *ExchangeRepository) GetMatchEventLog(ctx context.Context, market string, sequence uint64) (*MatchEventLog, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" || sequence == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var model ExchangeMatchEventLog
	err := r.db.WithContext(ctx).
		First(&model, "market = ? AND sequence_id = ? AND type = ?", market, sequence, MatchEventTypeResult).Error
	if err != nil {
		return nil, err
	}
	out := modelToMatchEventLog(model)
	return &out, nil
}

func (r *ExchangeRepository) LatestMatchEventSequence(ctx context.Context, market string) (uint64, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return 0, nil
	}
	var model ExchangeMatchEventLog
	err := r.db.WithContext(ctx).
		Where(&ExchangeMatchEventLog{Market: market, Type: MatchEventTypeResult}).
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

func modelToMatchEventLog(model ExchangeMatchEventLog) MatchEventLog {
	return MatchEventLog{
		ID:          model.ID,
		Market:      model.Market,
		SequenceID:  model.SequenceID,
		Type:        model.Type,
		PayloadHash: model.PayloadHash,
		Payload:     model.Payload,
		CreatedAt:   model.CreatedAt,
	}
}
