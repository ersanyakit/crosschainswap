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

	log.Println("Syncing GORM Pool model...")
	if err := db.AutoMigrate(&Pool{}); err != nil {
		return nil, fmt.Errorf("failed to sync Pool model: %w", err)
	}
	if err := db.Exec("UPDATE pools SET pool_address = id WHERE pool_address IS NULL OR pool_address = ''").Error; err != nil {
		return nil, fmt.Errorf("failed to backfill pool addresses: %w", err)
	}

	log.Println("GORM model sync completed successfully.")
	return db, nil
}
