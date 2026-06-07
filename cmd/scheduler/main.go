package main

import (
	"log"
	"time"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunPlaceholder("scheduler", 15*time.Second); err != nil {
		log.Fatal(err)
	}
}
