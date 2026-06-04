package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("scheduler"); err != nil {
		log.Fatal(err)
	}
}
