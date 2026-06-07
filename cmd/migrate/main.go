package main

import (
	"log"

	"exchange/internal/adapters/storage/postgres"
)

func main() {
	if err := postgres.LoadEnv("."); err != nil {
		log.Printf("Warning: failed to load .env file: %v", err)
	}
	db, err := postgres.ConnectWithOptions(postgres.ConnectOptions{AutoMigrate: false})
	if err != nil {
		log.Fatal(err)
	}
	if err := postgres.Migrate(db); err != nil {
		log.Fatal(err)
	}
}
