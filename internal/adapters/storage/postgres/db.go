package postgres

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// LoadEnv loads environment variables from a .env file if it exists.
func LoadEnv(rootPath string) error {
	envPath := filepath.Join(rootPath, ".env")
	file, err := os.Open(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // ignore missing .env if variables are set externally
		}
		return err
	}
	defer file.Close()

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
		os.Setenv(key, val)
	}
	return scanner.Err()
}

// ConnectAndMigrate establishes a PostgreSQL connection via GORM and auto-migrates the models.
func ConnectAndMigrate() (*gorm.DB, error) {
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

	// Auto-run GORM migrations
	log.Println("Running AutoMigrate for Pool model...")
	if err := db.AutoMigrate(&Pool{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate Pool model: %w", err)
	}

	log.Println("GORM migrations applied successfully.")
	return db, nil
}
