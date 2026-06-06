package rest

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"

	appauth "exchange/internal/app/auth"
	apporders "exchange/internal/app/orders"
	"exchange/internal/app/pricing"
	appswap "exchange/internal/app/swap"
	"exchange/internal/core/balance"
	"exchange/internal/core/order"

	"github.com/gofiber/fiber/v3"
)

type Server struct {
	app    *fiber.App
	hub    *Hub
	prices *pricing.Service
	swaps  *appswap.Service
	orders *apporders.Service
	auth   *appauth.Service
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewServer(prices *pricing.Service, swaps *appswap.Service, orders *apporders.Service, oidcAuth *appauth.Service) *Server {
	server := &Server{
		app:    fiber.New(),
		hub:    NewHub(),
		prices: prices,
		swaps:  swaps,
		orders: orders,
		auth:   oidcAuth,
	}
	server.routes()
	return server
}

func (s *Server) Listen(ctx context.Context, addr string) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.app.Listen(addr)
	}()

	select {
	case <-ctx.Done():
		if err := s.app.Shutdown(); err != nil {
			return err
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *Server) Publish(payload []byte) {
	s.hub.Publish(payload)
}

func (s *Server) routes() {
	s.app.Get("/health", s.health)
	s.app.Get("/auth/oidc/status", s.oidcStatus)
	s.app.Get("/auth/oidc/login", s.oidcLogin)
	s.app.Get("/auth/oidc/callback", s.oidcCallback)
	s.app.Get("/auth/me", s.authMe)
	s.app.Post("/auth/logout", s.authLogout)
	s.app.Get("/swagger", s.swagger)
	s.app.Get("/swagger.json", s.openapi)
	s.app.Get("/openapi.json", s.openapi)
	s.app.Get("/v1/assets", s.listAssets)
	s.app.Get("/v1/prices/:symbol", s.assetPrices)
	s.app.Get("/v1/assets/:symbol/prices", s.assetPrices)
	s.app.Post("/v1/swaps/quote", s.swapQuote)
	s.app.Post("/v1/swaps/transaction", s.swapTransaction)
	s.app.Post("/v1/swaps/approve", s.swapApprove)
	s.app.Post("/v1/orders", s.placeOrder)
	s.app.Get("/v1/orders/:id", s.getOrder)
	s.app.Delete("/v1/orders/:id", s.cancelOrder)
	s.app.Post("/v1/orders/triggers", s.triggerStops)
	s.app.Get("/v1/markets", s.listMarkets)
	s.app.Get("/v1/orderbook", s.orderBook)
	s.app.Get("/v1/orderbook/:market", s.orderBook)
	s.app.Get("/v1/markets/trades", s.marketTrades)
	s.app.Get("/v1/markets/:market/trades", s.marketTrades)
	s.app.Get("/v1/markets/candles", s.marketCandles)
	s.app.Get("/v1/markets/:market/candles", s.marketCandles)
	s.app.Get("/v1/users/:user_id/orders", s.orderHistory)
	s.app.Get("/v1/users/:user_id/trades", s.userTrades)
	s.app.Get("/v1/users/:user_id/balances", s.listBalances)
	s.app.Post("/v1/users/:user_id/deposits/pending", s.markDepositPending)
	s.app.Post("/v1/users/:user_id/deposits/settle", s.settleDeposit)
	s.app.Get("/v1/users/:user_id/withdrawals", s.listWithdrawals)
	s.app.Post("/v1/users/:user_id/withdrawals", s.requestWithdrawal)
	s.app.Post("/v1/withdrawals/:id/complete", s.completeWithdrawal)
	s.app.Post("/v1/withdrawals/:id/cancel", s.cancelWithdrawal)
	s.app.Get("/v1/users/:user_id/wallets", s.listWallets)
	s.app.Put("/v1/users/:user_id/wallets", s.registerGatewayWallet)
	s.app.Get("/ws", s.hub.Handle)
	s.app.Get("/ws/orders", s.hub.Handle)
	s.app.Get("/ws/prices", s.hub.Handle)
}

func (s *Server) health(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

func (s *Server) assetPrices(c fiber.Ctx) error {
	result, err := s.prices.Prices(c.Context(), c.Params("symbol"))
	if err != nil {
		return pricingError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) listAssets(c fiber.Ctx) error {
	return c.JSON(s.prices.Assets())
}

func (s *Server) swapQuote(c fiber.Ctx) error {
	req, err := bindSwapRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.swaps.Quote(c.Context(), req)
	if err != nil {
		return swapError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) swapTransaction(c fiber.Ctx) error {
	req, err := bindSwapRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.swaps.Transaction(c.Context(), req)
	if err != nil {
		return swapError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) swapApprove(c fiber.Ctx) error {
	req, err := bindSwapRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.swaps.Approve(c.Context(), req)
	if err != nil {
		return swapError(c, err)
	}
	return c.JSON(result)
}

func bindSwapRequest(c fiber.Ctx) (appswap.Request, error) {
	var req appswap.Request
	if err := c.Bind().Body(&req); err != nil {
		return req, err
	}
	return req, nil
}

func (s *Server) placeOrder(c fiber.Ctx) error {
	claims, err := s.requireUser(c)
	if err != nil {
		return err
	}
	req, err := bindOrderRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	if claims != nil {
		req.UserID = claims.Subject
	}
	result, err := s.orders.Place(c.Context(), req)
	if err != nil {
		return orderError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func (s *Server) getOrder(c fiber.Ctx) error {
	result, err := s.orders.Get(c.Context(), order.ID(c.Params("id")))
	if err != nil {
		return orderError(c, err)
	}
	if err := s.requireOrderOwner(c, result.UserID); err != nil {
		return err
	}
	return c.JSON(result)
}

func (s *Server) cancelOrder(c fiber.Ctx) error {
	claims, err := s.requireUser(c)
	if err != nil {
		return err
	}
	userID := c.Query("user_id")
	if claims != nil {
		userID = claims.Subject
	}
	result, err := s.orders.Cancel(c.Context(), order.ID(c.Params("id")), apporders.CancelRequest{
		UserID: userID,
	})
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) triggerStops(c fiber.Ctx) error {
	var req apporders.TriggerRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.orders.TriggerStops(c.Context(), req)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) listMarkets(c fiber.Ctx) error {
	result, err := s.orders.MarketSummaries(c.Context())
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) orderBook(c fiber.Ctx) error {
	depth := 100
	if raw := c.Query("depth"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: "depth must be an integer"})
		}
		depth = parsed
	}
	result, err := s.orders.Book(c.Context(), marketParam(c), depth)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) marketTrades(c fiber.Ctx) error {
	limit, err := queryInt(c, "limit", 100)
	if err != nil {
		return err
	}
	result, err := s.orders.MarketTrades(c.Context(), apporders.MarketHistoryRequest{
		Market: marketParam(c),
		Limit:  limit,
	})
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) marketCandles(c fiber.Ctx) error {
	limit, err := queryInt(c, "limit", 500)
	if err != nil {
		return err
	}
	result, err := s.orders.Candles(c.Context(), apporders.MarketHistoryRequest{
		Market:   marketParam(c),
		Interval: c.Query("interval", "1m"),
		Limit:    limit,
	})
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) orderHistory(c fiber.Ctx) error {
	userID, err := s.requirePathUser(c)
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 100)
	if err != nil {
		return err
	}
	result, err := s.orders.OrderHistory(c.Context(), apporders.HistoryRequest{
		UserID: userID,
		Market: c.Query("market"),
		Status: c.Query("status"),
		Limit:  limit,
	})
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) userTrades(c fiber.Ctx) error {
	userID, err := s.requirePathUser(c)
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 100)
	if err != nil {
		return err
	}
	result, err := s.orders.UserTrades(c.Context(), apporders.HistoryRequest{
		UserID: userID,
		Market: c.Query("market"),
		Limit:  limit,
	})
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) listBalances(c fiber.Ctx) error {
	userID, err := s.requirePathUser(c)
	if err != nil {
		return err
	}
	result, err := s.orders.ListBalances(c.Context(), userID)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) markDepositPending(c fiber.Ctx) error {
	if err := requireGatewaySecret(c); err != nil {
		return err
	}
	return s.balanceMutation(c, s.orders.MarkDepositPending)
}

func (s *Server) settleDeposit(c fiber.Ctx) error {
	if err := requireGatewaySecret(c); err != nil {
		return err
	}
	return s.balanceMutation(c, s.orders.SettleDeposit)
}

func (s *Server) balanceMutation(c fiber.Ctx, fn func(context.Context, string, apporders.BalanceAmountRequest) (*balance.Balance, error)) error {
	var req apporders.BalanceAmountRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := fn(c.Context(), c.Params("user_id"), req)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) listWithdrawals(c fiber.Ctx) error {
	userID, err := s.requirePathUser(c)
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 100)
	if err != nil {
		return err
	}
	result, err := s.orders.ListWithdrawals(c.Context(), userID, limit)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) requestWithdrawal(c fiber.Ctx) error {
	userID, err := s.requirePathUser(c)
	if err != nil {
		return err
	}
	var req apporders.WithdrawalRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.orders.RequestWithdrawal(c.Context(), userID, req)
	if err != nil {
		return orderError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func (s *Server) completeWithdrawal(c fiber.Ctx) error {
	if err := requireGatewaySecret(c); err != nil {
		return err
	}
	result, err := s.orders.CompleteWithdrawal(c.Context(), balance.WithdrawalID(c.Params("id")))
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) cancelWithdrawal(c fiber.Ctx) error {
	if err := requireGatewaySecret(c); err != nil {
		return err
	}
	result, err := s.orders.CancelWithdrawal(c.Context(), balance.WithdrawalID(c.Params("id")))
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) listWallets(c fiber.Ctx) error {
	userID, err := s.requirePathUser(c)
	if err != nil {
		return err
	}
	result, err := s.orders.ListWallets(c.Context(), userID)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func (s *Server) registerGatewayWallet(c fiber.Ctx) error {
	if err := requireGatewaySecret(c); err != nil {
		return err
	}
	var req apporders.WalletRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.orders.RegisterGatewayWallet(c.Context(), c.Params("user_id"), req)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(result)
}

func requireGatewaySecret(c fiber.Ctx) error {
	expected := os.Getenv("PAYMENT_GATEWAY_SECRET")
	if expected == "" {
		return nil
	}
	actual := c.Get("X-Gateway-Secret")
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: "invalid gateway secret"})
	}
	return nil
}

func queryInt(c fiber.Ctx, key string, fallback int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: key + " must be a positive integer"})
	}
	return parsed, nil
}

func marketParam(c fiber.Ctx) string {
	if market := strings.TrimSpace(c.Query("market")); market != "" {
		return market
	}
	return decodeMarketSymbol(c.Params("market"))
}

func decodeMarketSymbol(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return decoded
}

func bindOrderRequest(c fiber.Ctx) (apporders.PlaceRequest, error) {
	var req apporders.PlaceRequest
	if err := c.Bind().Body(&req); err != nil {
		return req, err
	}
	return req, nil
}

func pricingError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, pricing.ErrSymbolRequired):
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, pricing.ErrUnknownAsset):
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, pricing.ErrNoAssetDeployment):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errorResponse{Error: err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(errorResponse{Error: err.Error()})
	}
}

func swapError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, appswap.ErrUnknownVenue):
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, appswap.ErrUnsupportedSwap):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errorResponse{Error: err.Error()})
	default:
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
}

func orderError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, apporders.ErrUnknownMarket), errors.Is(err, apporders.ErrOrderNotFound):
		return c.Status(fiber.StatusNotFound).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, apporders.ErrOrderNotCancelable):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, apporders.ErrPriceBandExceeded):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, apporders.ErrInvalidOrder):
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, balance.ErrInsufficientBalance):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errorResponse{Error: err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(errorResponse{Error: err.Error()})
	}
}
