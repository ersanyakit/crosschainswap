package postgres

import (
	"context"
	"fmt"
	"time"

	"exchange/internal/core/venue"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PoolRepository struct {
	db *gorm.DB
}

func NewPoolRepository(db *gorm.DB) *PoolRepository {
	return &PoolRepository{db: db}
}

// SavePools batch-upserts the pools into the database using GORM.
func (r *PoolRepository) SavePools(ctx context.Context, pools []venue.Pool) error {
	if len(pools) == 0 {
		return nil
	}

	dbPools := make([]Pool, len(pools))
	now := time.Now()

	for i, p := range pools {
		reserve0Str := "0"
		if p.Reserve0 != nil {
			reserve0Str = p.Reserve0.String()
		}
		reserve1Str := "0"
		if p.Reserve1 != nil {
			reserve1Str = p.Reserve1.String()
		}

		dbPools[i] = Pool{
			ID:        string(p.ID),
			ChainKey:  string(p.ChainKey),
			VenueKey:  string(p.VenueKey),
			Kind:      string(p.Kind),
			Token0:    string(p.Token0),
			Token1:    string(p.Token1),
			Reserve0:  reserve0Str,
			Reserve1:  reserve1Str,
			Enabled:   p.Enabled,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	// Use GORM transactional Create with OnConflict upsert behaviour
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"reserve0", "reserve1", "enabled", "updated_at"}),
		}).Create(&dbPools).Error
	})

	if err != nil {
		return fmt.Errorf("failed to upsert pools: %w", err)
	}

	return nil
}
