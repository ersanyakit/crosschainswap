package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("executor"); err != nil {
		log.Fatal(err)
	}
}
