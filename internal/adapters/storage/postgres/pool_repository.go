package postgres

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"exchange/internal/core/chain"
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
		poolAddress := p.Address
		if poolAddress == "" {
			poolAddress = string(p.ID)
		}

		dbPools[i] = Pool{
			ID:           string(p.ID),
			PoolAddress:  poolAddress,
			ChainKey:     string(p.ChainKey),
			VenueKey:     string(p.VenueKey),
			Kind:         string(p.Kind),
			Token0:       string(p.Token0),
			Token1:       string(p.Token1),
			Reserve0:     bigIntString(p.Reserve0),
			Reserve1:     bigIntString(p.Reserve1),
			SqrtPriceX96: bigIntString(p.SqrtPriceX96),
			Liquidity:    bigIntString(p.Liquidity),
			Tick:         p.Tick,
			Fee:          p.Fee,
			TickSpacing:  p.TickSpacing,
			ProgramID:    p.ProgramID,
			Vault0:       p.Vault0,
			Vault1:       p.Vault1,
			Enabled:      p.Enabled,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
	}

	// Use GORM transactional Create with OnConflict upsert behaviour
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"pool_address", "chain_key", "venue_key", "kind", "token0", "token1", "reserve0", "reserve1", "sqrt_price_x96", "liquidity", "tick", "fee", "tick_spacing", "program_id", "vault0", "vault1", "enabled", "updated_at"}),
		}).Create(&dbPools).Error
	})

	if err != nil {
		return fmt.Errorf("failed to upsert pools: %w", err)
	}

	return nil
}

func (r *PoolRepository) GetPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	var dbPool Pool
	if err := r.db.WithContext(ctx).First(&dbPool, "id = ?", string(id)).Error; err != nil {
		return nil, fmt.Errorf("failed to load pool %s: %w", id, err)
	}

	pool, err := dbPool.toVenuePool()
	if err != nil {
		return nil, fmt.Errorf("failed to decode pool %s: %w", id, err)
	}

	return pool, nil
}

func (p Pool) toVenuePool() (*venue.Pool, error) {
	reserve0, err := parseBigInt("reserve0", p.Reserve0)
	if err != nil {
		return nil, err
	}
	reserve1, err := parseBigInt("reserve1", p.Reserve1)
	if err != nil {
		return nil, err
	}
	sqrtPriceX96, err := parseBigInt("sqrt_price_x96", p.SqrtPriceX96)
	if err != nil {
		return nil, err
	}
	liquidity, err := parseBigInt("liquidity", p.Liquidity)
	if err != nil {
		return nil, err
	}

	poolAddress := p.PoolAddress
	if poolAddress == "" {
		poolAddress = p.ID
	}

	return &venue.Pool{
		ID:           venue.PoolID(p.ID),
		Address:      poolAddress,
		ChainKey:     chain.ChainKey(p.ChainKey),
		VenueKey:     venue.VenueKey(p.VenueKey),
		Kind:         venue.PoolKind(p.Kind),
		Token0:       venue.AssetID(p.Token0),
		Token1:       venue.AssetID(p.Token1),
		Reserve0:     reserve0,
		Reserve1:     reserve1,
		SqrtPriceX96: sqrtPriceX96,
		Liquidity:    liquidity,
		Tick:         p.Tick,
		Fee:          p.Fee,
		TickSpacing:  p.TickSpacing,
		ProgramID:    p.ProgramID,
		Vault0:       p.Vault0,
		Vault1:       p.Vault1,
		Enabled:      p.Enabled,
	}, nil
}

func bigIntString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

func parseBigInt(field string, value string) (*big.Int, error) {
	if value == "" {
		return big.NewInt(0), nil
	}
	out, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("%s is not a valid integer: %q", field, value)
	}
	return out, nil
}
