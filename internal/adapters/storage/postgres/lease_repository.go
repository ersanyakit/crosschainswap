package postgres

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

type LeaseRepository struct {
	db *gorm.DB
}

func NewLeaseRepository(db *gorm.DB) *LeaseRepository {
	return &LeaseRepository{db: db}
}

func (r *LeaseRepository) TryAcquire(ctx context.Context, name string, owner string, ttl time.Duration) (bool, error) {
	name = strings.TrimSpace(name)
	owner = strings.TrimSpace(owner)
	if name == "" || owner == "" {
		return false, nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}

	now := time.Now()
	expiresAt := now.Add(ttl)
	var acquired bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&ServiceLease{}).
			Where("name = ? AND (expires_at <= ? OR owner = ?)", name, now, owner).
			Updates(map[string]any{
				"owner":      owner,
				"expires_at": expiresAt,
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			acquired = true
			return nil
		}

		lease := ServiceLease{Name: name, Owner: owner, ExpiresAt: expiresAt, UpdatedAt: now}
		if err := tx.Create(&lease).Error; err != nil {
			if isConcurrentMigrationConflict(err) || strings.Contains(err.Error(), "duplicate key value") {
				acquired = false
				return nil
			}
			return err
		}
		acquired = true
		return nil
	})
	return acquired, err
}

func (r *LeaseRepository) Release(ctx context.Context, name string, owner string) error {
	name = strings.TrimSpace(name)
	owner = strings.TrimSpace(owner)
	if name == "" || owner == "" {
		return nil
	}
	return r.db.WithContext(ctx).
		Where(&ServiceLease{Name: name, Owner: owner}).
		Delete(&ServiceLease{}).Error
}
