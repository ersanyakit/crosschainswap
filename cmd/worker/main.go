package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("worker"); err != nil {
		log.Fatal(err)
	}
}
