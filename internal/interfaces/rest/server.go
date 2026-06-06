package rest

import (
	"context"
	"errors"

	"exchange/internal/app/pricing"
	appswap "exchange/internal/app/swap"

	"github.com/gofiber/fiber/v3"
)

type Server struct {
	app    *fiber.App
	hub    *Hub
	prices *pricing.Service
	swaps  *appswap.Service
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewServer(prices *pricing.Service, swaps *appswap.Service) *Server {
	server := &Server{
		app:    fiber.New(),
		hub:    NewHub(),
		prices: prices,
		swaps:  swaps,
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
	s.app.Get("/v1/prices/:symbol", s.assetPrices)
	s.app.Get("/v1/assets/:symbol/prices", s.assetPrices)
	s.app.Post("/v1/swaps/quote", s.swapQuote)
	s.app.Post("/v1/swaps/transaction", s.swapTransaction)
	s.app.Post("/v1/swaps/approve", s.swapApprove)
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
