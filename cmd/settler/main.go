package main

import (
	"log"
	"time"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunPlaceholder("settler", 5*time.Second); err != nil {
		log.Fatal(err)
	}
}
