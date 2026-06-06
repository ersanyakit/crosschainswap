package apiruntime

import (
	"context"
	"errors"
	"log"
	"os"

	"exchange/internal/adapters/storage/postgres"
	"exchange/internal/app/pricing"
	"exchange/internal/app/swap"
	"exchange/internal/bootstrap"
	"exchange/internal/config"
	"exchange/internal/interfaces/rest"
)

func Run(ctx context.Context) error {
	if err := postgres.LoadEnv("."); err != nil {
		log.Printf("Warning: failed to load .env file: %v", err)
	}

	db, err := postgres.Connect()
	if err != nil {
		return err
	}

	registries := config.LoadDefaultRegistries()
	poolRepo := postgres.NewPoolRepository(db)
	priceService := pricing.NewService(registries.Assets, poolRepo)
	v3Quoter, closeV3Quoter, err := bootstrap.NewUniswapV3Quoter(registries.Chains, registries.Venues)
	if err != nil {
		return err
	}
	defer closeV3Quoter()

	swapEngine, err := bootstrap.NewSwapEngine(registries.Venues, bootstrap.SwapEngineOptions{
		PoolProvider: poolRepo,
		V3Quoter:     v3Quoter,
	})
	if err != nil {
		return err
	}
	swapService := swap.NewService(registries.Assets, registries.Venues, poolRepo, swapEngine)
	server := rest.NewServer(priceService, swapService)

	go func() {
		if err := postgres.Listen(ctx, os.Getenv("DATABASE_URL"), pricing.UpdatesChannel, func(_ context.Context, payload []byte) error {
			server.Publish(payload)
			return nil
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("price update listener stopped: %v", err)
		}
	}()

	return server.Listen(ctx, Addr())
}

func Addr() string {
	if addr := os.Getenv("API_ADDR"); addr != "" {
		return addr
	}
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}
