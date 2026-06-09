package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"exchange/internal/app/reconciliation"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := reconciliation.Run(ctx, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
