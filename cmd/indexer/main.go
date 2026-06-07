package main

import (
	"log"
	"time"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunPlaceholder("indexer", 10*time.Second); err != nil {
		log.Fatal(err)
	}
}
