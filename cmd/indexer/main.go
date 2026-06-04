package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("indexer"); err != nil {
		log.Fatal(err)
	}
}
