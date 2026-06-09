package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProjectionOffset struct {
	Projection     string
	Market         string
	Stream         string
	LastSequence   uint64
	LastEventID    string
	LastEventHash  string
	LastEventAt    time.Time
	UpdatedAt      time.Time
	LastAdvancedAt time.Time
}

func (r *ExchangeRepository) GetProjectionOffset(ctx context.Context, projection string, market string) (*ProjectionOffset, error) {
	projection = normalizeProjectionName(projection)
	market = strings.ToUpper(strings.TrimSpace(market))
	if projection == "" || market == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var model ExchangeProjectionOffset
	err := r.db.WithContext(ctx).First(&model, "projection = ? AND market = ?", projection, market).Error
	if err != nil {
		return nil, err
	}
	out := modelToProjectionOffset(model)
	return &out, nil
}

func (r *ExchangeRepository) AdvanceProjectionOffset(ctx context.Context, item ProjectionOffset) (*ProjectionOffset, error) {
	item.Projection = normalizeProjectionName(item.Projection)
	item.Market = strings.ToUpper(strings.TrimSpace(item.Market))
	item.Stream = strings.TrimSpace(item.Stream)
	item.LastEventID = strings.TrimSpace(item.LastEventID)
	item.LastEventHash = strings.TrimSpace(item.LastEventHash)
	if item.Projection == "" || item.Market == "" {
		return nil, fmt.Errorf("projection and market are required for projection offset")
	}
	if item.Stream == "" {
		item.Stream = item.Market
	}

	var out ProjectionOffset
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var existing ExchangeProjectionOffset
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&existing, "projection = ? AND market = ?", item.Projection, item.Market).Error
		if err == nil {
			if item.LastSequence < existing.LastSequence {
				out = modelToProjectionOffset(existing)
				return nil
			}
			if item.LastSequence == existing.LastSequence {
				if existing.LastEventID != "" && item.LastEventID != "" && existing.LastEventID != item.LastEventID {
					return fmt.Errorf("projection offset conflict for %s/%s sequence %d", item.Projection, item.Market, item.LastSequence)
				}
				if existing.LastEventHash != "" && item.LastEventHash != "" && existing.LastEventHash != item.LastEventHash {
					return fmt.Errorf("projection offset hash conflict for %s/%s sequence %d", item.Projection, item.Market, item.LastSequence)
				}
				out = modelToProjectionOffset(existing)
				return nil
			}
			existing.Stream = item.Stream
			existing.LastSequence = item.LastSequence
			existing.LastEventID = item.LastEventID
			existing.LastEventHash = item.LastEventHash
			existing.LastEventAt = item.LastEventAt
			existing.UpdatedAt = now
			existing.LastAdvancedAt = now
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			out = modelToProjectionOffset(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		model := ExchangeProjectionOffset{
			Projection:     item.Projection,
			Market:         item.Market,
			Stream:         item.Stream,
			LastSequence:   item.LastSequence,
			LastEventID:    item.LastEventID,
			LastEventHash:  item.LastEventHash,
			LastEventAt:    item.LastEventAt,
			UpdatedAt:      now,
			LastAdvancedAt: now,
		}
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		out = modelToProjectionOffset(model)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func normalizeProjectionName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func modelToProjectionOffset(model ExchangeProjectionOffset) ProjectionOffset {
	return ProjectionOffset{
		Projection:     model.Projection,
		Market:         model.Market,
		Stream:         model.Stream,
		LastSequence:   model.LastSequence,
		LastEventID:    model.LastEventID,
		LastEventHash:  model.LastEventHash,
		LastEventAt:    model.LastEventAt,
		UpdatedAt:      model.UpdatedAt,
		LastAdvancedAt: model.LastAdvancedAt,
	}
}
