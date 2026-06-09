package postgres

import (
	"context"
	"strings"
	"time"

	"exchange/internal/core/matching"
	"exchange/pkg/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ExchangeRepository) SaveMatcherSnapshot(ctx context.Context, snapshot matching.BookStateSnapshot) error {
	if snapshot.Checksum == "" {
		if err := snapshot.Seal(); err != nil {
			return err
		}
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	payload, err := matching.EncodeBookStateSnapshot(snapshot)
	if err != nil {
		return err
	}
	now := time.Now()
	model := ExchangeMatcherSnapshot{
		ID:              idgen.New("msn"),
		Market:          strings.ToUpper(strings.TrimSpace(snapshot.Market)),
		SequenceID:      snapshot.LastAppliedSequence,
		SchemaVersion:   snapshot.SchemaVersion,
		Payload:         string(payload),
		Checksum:        snapshot.Checksum,
		CreatedAt:       now,
		LastValidatedAt: now,
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r *ExchangeRepository) LatestMatcherSnapshot(ctx context.Context, market string) (*matching.BookStateSnapshot, error) {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var model ExchangeMatcherSnapshot
	err := r.db.WithContext(ctx).
		Where(&ExchangeMatcherSnapshot{Market: market}).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "sequence_id"}, Desc: true},
			{Column: clause.Column{Name: "created_at"}, Desc: true},
		}}).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	snapshot, err := matching.DecodeBookStateSnapshot([]byte(model.Payload))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := r.db.WithContext(ctx).
		Model(&ExchangeMatcherSnapshot{}).
		Where(&ExchangeMatcherSnapshot{ID: model.ID}).
		Update("last_validated_at", now).Error; err != nil {
		return nil, err
	}
	return &snapshot, nil
}
