package postgres

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exchange/internal/core/market"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LoadEnv loads environment variables from a .env file if it exists.
func LoadEnv(rootPath string) error {
	envPath, err := findEnvPath(rootPath)
	if err != nil {
		return err
	}
	if envPath == "" {
		return nil
	}

	file, err := os.Open(envPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip optional quotes
		val = strings.Trim(val, `"'`)
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func findEnvPath(startPath string) (string, error) {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	for {
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// Connect establishes a PostgreSQL connection and syncs the GORM models.
func Connect() (*gorm.DB, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve sql.DB instance: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("GORM database connection established successfully.")

	log.Println("Syncing GORM models...")
	if err := autoMigrateWithRetry(db); err != nil {
		return nil, fmt.Errorf("failed to sync GORM models: %w", err)
	}
	if err := backfillPoolAddresses(db); err != nil {
		return nil, fmt.Errorf("failed to backfill pool addresses: %w", err)
	}
	if err := backfillOrderSequences(db); err != nil {
		return nil, fmt.Errorf("failed to backfill order sequences: %w", err)
	}

	log.Println("GORM model sync completed successfully.")
	return db, nil
}

func autoMigrateWithRetry(db *gorm.DB) error {
	models := []any{&Pool{}, &ExchangeOrder{}, &ExchangeOrderSequence{}, &ExchangeTrade{}, &ExchangeCandle{}, &ExchangeOrderEvent{}, &ExchangeWallet{}, &ExchangeBalance{}, &ExchangeBalanceEvent{}, &ExchangeWithdrawal{}, &ExchangePriceLevel{}, &ExchangeMarket{}}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = db.AutoMigrate(models...)
		if err == nil {
			return nil
		}
		if !isConcurrentMigrationConflict(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return err
}

func isConcurrentMigrationConflict(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "pg_type_typname_nsp_index") ||
		strings.Contains(message, "duplicate key value violates unique constraint")
}

func SyncExchangeMarkets(db *gorm.DB, markets []market.Market) error {
	now := time.Now()
	for _, item := range markets {
		model := ExchangeMarket{
			Symbol:     item.Symbol,
			BaseAsset:  item.BaseAsset,
			QuoteAsset: item.QuoteAsset,
			ChainKeys:  "",
			Enabled:    item.Enabled,
			UpdatedAt:  now,
		}

		var existing ExchangeMarket
		result := db.Where("symbol = ?", item.Symbol).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			model.CreatedAt = existing.CreatedAt
		} else {
			model.CreatedAt = now
		}

		if err := db.Save(&model).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillOrderSequences(db *gorm.DB) error {
	var markets []string
	if err := db.Model(&ExchangeOrder{}).Distinct().Pluck("market", &markets).Error; err != nil {
		return err
	}
	repo := NewExchangeRepository(db)
	for _, marketSymbol := range markets {
		var last ExchangeOrder
		maxSeq := uint64(0)
		err := db.Where(&ExchangeOrder{Market: marketSymbol}).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "sequence_id"}, Desc: true}}}).
			First(&last).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			maxSeq = last.SequenceID
		}

		var seq ExchangeOrderSequence
		err = db.Where(&ExchangeOrderSequence{Market: marketSymbol}).First(&seq).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			seq = ExchangeOrderSequence{Market: marketSymbol, NextSequence: maxSeq + 1, UpdatedAt: time.Now()}
			if seq.NextSequence == 0 {
				seq.NextSequence = 1
			}
			if err := db.Create(&seq).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if seq.NextSequence <= maxSeq {
			seq.NextSequence = maxSeq + 1
			seq.UpdatedAt = time.Now()
			if err := db.Save(&seq).Error; err != nil {
				return err
			}
		}

		var zeroSeqOrders []ExchangeOrder
		if err := db.Where(&ExchangeOrder{Market: marketSymbol, SequenceID: 0}).
			Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "created_at"}},
				{Column: clause.Column{Name: "id"}},
			}}).
			Find(&zeroSeqOrders).Error; err != nil {
			return err
		}
		next := seq.NextSequence
		if next == 0 {
			next = 1
		}
		for _, item := range zeroSeqOrders {
			item.SequenceID = next
			next++
			if err := db.Save(&item).Error; err != nil {
				return err
			}
		}
		if next != seq.NextSequence {
			seq.NextSequence = next
			seq.UpdatedAt = time.Now()
			if err := db.Save(&seq).Error; err != nil {
				return err
			}
		}
		if err := repo.RebuildPriceLevels(context.Background(), marketSymbol); err != nil {
			return err
		}
	}
	return nil
}

func backfillPoolAddresses(db *gorm.DB) error {
	var pools []Pool
	if err := db.Where("pool_address = ? OR pool_address IS NULL", "").Find(&pools).Error; err != nil {
		return err
	}
	for i := range pools {
		pools[i].PoolAddress = pools[i].ID
		if err := db.Save(&pools[i]).Error; err != nil {
			return err
		}
	}
	return nil
}
