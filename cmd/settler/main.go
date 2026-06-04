package main

import (
	"log"

	"exchange/internal/runtime"
)

func main() {
	if err := runtime.RunAll("settler"); err != nil {
		log.Fatal(err)
	}
}
