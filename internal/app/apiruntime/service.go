package apiruntime

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"exchange/internal/adapters/eventstream"
	"exchange/internal/adapters/paymentgateway"
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

	registries := config.LoadRegistries(ctx)
	if err := postgres.SyncExchangeMarkets(db, registries.Markets.All()); err != nil {
		return err
	}
	poolRepo := postgres.NewPoolRepository(db)
	exchangeRepo := postgres.NewExchangeRepository(db)
	outboxRepo := postgres.NewOutboxRepository(db)
	eventBus, err := eventstream.NewFromEnv(db)
	if err != nil {
		return err
	}
	defer func() {
		if err := eventBus.Close(); err != nil {
			log.Printf("event backend close failed: %v", err)
		}
	}()
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
	gatewayClient := paymentgateway.NewClient(paymentgateway.ConfigFromEnv())
	if gatewayClient.Enabled() || gatewayClient.StaticAddressEnabled() || gatewayClient.QRCodeEnabled() {
		orderService.SetGatewayWalletProvider(walletGatewayAdapter{client: gatewayClient})
	} else {
		log.Printf("Payment gateway wallet sync disabled: set gateway merchant wallet or static address credentials to enable it")
	}
	oidcAuth, err := appauth.NewOIDCService(ctx, appauth.ConfigFromEnv())
	if err != nil {
		return err
	}
	if oidcAuth == nil || !oidcAuth.Enabled() {
		log.Printf("OIDC auth disabled: set OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET and OIDC_REDIRECT_URI to enable it")
	}
	server := rest.NewServer(priceService, swapService, orderService, oidcAuth)
	orderService.SetPublisher(func(payload []byte) {
		notifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := outboxRepo.Create(notifyCtx, orders.UpdatesChannel, "", payload); err != nil {
			log.Printf("failed to enqueue exchange update: %v", err)
		}
	})

	go func() {
		topics := []string{pricing.UpdatesChannel, orders.UpdatesChannel}
		if err := eventBus.Subscribe(ctx, topics, func(ctx context.Context, msg eventstream.Message) error {
			switch msg.Topic {
			case pricing.UpdatesChannel:
				server.Publish(priceUpdateSocketPayload(ctx, priceService, msg.Payload))
			case orders.UpdatesChannel:
				server.Publish(msg.Payload)
			default:
				server.Publish(msg.Payload)
			}
			return nil
		}); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("event backend listener stopped: %v", err)
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

type walletGatewayAdapter struct {
	client *paymentgateway.Client
}

func (a walletGatewayAdapter) CreateUserWallet(ctx context.Context, userID string) ([]orders.GatewayWallet, error) {
	items, err := a.client.CreateUserWallet(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]orders.GatewayWallet, 0, len(items))
	for _, item := range items {
		out = append(out, orders.GatewayWallet{ChainKey: item.ChainKey, Address: item.Address})
	}
	return out, nil
}

func (a walletGatewayAdapter) CreateStaticAddress(ctx context.Context, userID string, symbol string, chainID int64, label string) (*orders.GatewayStaticAddress, error) {
	item, err := a.client.CreateStaticAddress(ctx, paymentgateway.StaticAddressRequest{
		UserID:  userID,
		Symbol:  symbol,
		ChainID: chainID,
		Label:   label,
	})
	if err != nil {
		return nil, err
	}
	return &orders.GatewayStaticAddress{
		WalletID: item.WalletID,
		UserID:   item.UserID,
		Symbol:   item.Symbol,
		Chain:    item.Chain,
		Address:  item.Address,
		Label:    item.Label,
	}, nil
}

func (a walletGatewayAdapter) QRCode(ctx context.Context, address string, size int) ([]byte, error) {
	return a.client.QRCode(ctx, address, size)
}
