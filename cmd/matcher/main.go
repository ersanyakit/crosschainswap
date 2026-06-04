package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("matcher"); err != nil {
		log.Fatal(err)
	}
}
