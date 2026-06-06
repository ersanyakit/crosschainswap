package apiruntime

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"

	"exchange/internal/adapters/storage/postgres"
	appauth "exchange/internal/app/auth"
	"exchange/internal/app/orders"
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
	if err := postgres.SyncExchangeMarkets(db, registries.Markets.All()); err != nil {
		return err
	}
	poolRepo := postgres.NewPoolRepository(db)
	exchangeRepo := postgres.NewExchangeRepository(db)
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
	orderService := orders.NewService(registries.Markets, exchangeRepo)
	oidcAuth, err := appauth.NewOIDCService(ctx, appauth.ConfigFromEnv())
	if err != nil {
		return err
	}
	if oidcAuth == nil || !oidcAuth.Enabled() {
		log.Printf("OIDC auth disabled: set OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET and OIDC_REDIRECT_URI to enable it")
	}
	server := rest.NewServer(priceService, swapService, orderService, oidcAuth)
	orderService.SetPublisher(server.Publish)

	go func() {
		if err := postgres.Listen(ctx, os.Getenv("DATABASE_URL"), pricing.UpdatesChannel, func(_ context.Context, payload []byte) error {
			server.Publish(priceUpdateSocketPayload(ctx, priceService, payload))
			return nil
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("price update listener stopped: %v", err)
		}
	}()

	return server.Listen(ctx, Addr())
}

func priceUpdateSocketPayload(ctx context.Context, priceService *pricing.Service, payload []byte) []byte {
	var event struct {
		Type string `json:"type"`
		Data struct {
			Symbol string          `json:"symbol"`
			Prices json.RawMessage `json:"prices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return payload
	}
	if event.Type != "prices.updated" || event.Data.Symbol == "" || len(event.Data.Prices) != 0 {
		return payload
	}

	prices, err := priceService.Prices(ctx, event.Data.Symbol)
	if err != nil {
		log.Printf("failed to expand price update for %s: %v", event.Data.Symbol, err)
		return payload
	}
	out, err := json.Marshal(pricing.NewUpdateEvent(prices))
	if err != nil {
		log.Printf("failed to encode price update for %s: %v", event.Data.Symbol, err)
		return payload
	}
	return out
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
