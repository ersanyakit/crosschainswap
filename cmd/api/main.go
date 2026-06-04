package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("api"); err != nil {
		log.Fatal(err)
	}
}
