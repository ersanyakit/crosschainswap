package swap

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	evmswap "exchange/internal/adapters/venues/evm/swap"
	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	coreswap "exchange/internal/core/swap"
	"exchange/internal/core/venue"
)

var (
	ErrUnsupportedSwap = errors.New("swap is not supported for this venue")
	ErrUnknownVenue    = errors.New("venue is not registered")
)

type PoolStore interface {
	GetPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error)
	ListPoolsByAssetIDs(ctx context.Context, ids []venue.AssetID) ([]venue.Pool, error)
}

type Service struct {
	assets asset.Registry
	venues venue.Registry
	pools  PoolStore
	engine *Engine
}

type Request struct {
	ChainKey       chain.ChainKey `json:"chain_key"`
	VenueKey       venue.VenueKey `json:"venue_key"`
	TokenInSymbol  string         `json:"token_in_symbol"`
	TokenOutSymbol string         `json:"token_out_symbol"`
	AmountIn       string         `json:"amount_in"`
	Recipient      string         `json:"recipient"`
	Sender         string         `json:"sender"`
	SlippageBps    uint32         `json:"slippage_bps"`
	DeadlineUnix   uint64         `json:"deadline_unix"`
	PoolID         venue.PoolID   `json:"pool_id"`
}

type QuoteResponse struct {
	ChainKey  chain.ChainKey  `json:"chain_key"`
	VenueKey  venue.VenueKey  `json:"venue_key"`
	VenueKind venue.VenueKind `json:"venue_kind"`
	PoolID    venue.PoolID    `json:"pool_id"`
	TokenIn   string          `json:"token_in"`
	TokenOut  string          `json:"token_out"`
	AmountIn  string          `json:"amount_in"`
	AmountOut string          `json:"amount_out"`
	MinOut    string          `json:"min_out"`
	FeeBps    uint32          `json:"fee_bps"`
}

type EVMTransactionResponse struct {
	ChainKey  chain.ChainKey  `json:"chain_key"`
	VenueKey  venue.VenueKey  `json:"venue_key"`
	VenueKind venue.VenueKind `json:"venue_kind"`
	To        string          `json:"to"`
	Data      string          `json:"data"`
	Value     string          `json:"value"`
}

type TransactionResponse struct {
	Quote *QuoteResponse          `json:"quote"`
	EVM   *EVMTransactionResponse `json:"evm,omitempty"`
}

func NewService(assets asset.Registry, venues venue.Registry, pools PoolStore, engine *Engine) *Service {
	return &Service{
		assets: assets,
		venues: venues,
		pools:  pools,
		engine: engine,
	}
}

func (s *Service) Quote(ctx context.Context, req Request) (*QuoteResponse, error) {
	swapReq, err := s.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	quote, err := s.engine.Quote(ctx, *swapReq)
	if err != nil {
		return nil, err
	}
	return quoteResponse(*quote), nil
}

func (s *Service) Transaction(ctx context.Context, req Request) (*TransactionResponse, error) {
	swapReq, err := s.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	quote, err := s.engine.Quote(ctx, *swapReq)
	if err != nil {
		return nil, err
	}
	intent, err := s.engine.BuildTransaction(ctx, *swapReq, *quote)
	if err != nil {
		return nil, err
	}

	return &TransactionResponse{
		Quote: quoteResponse(*quote),
		EVM:   evmTransactionResponse(intent),
	}, nil
}

func (s *Service) Approve(ctx context.Context, req Request) (*EVMTransactionResponse, error) {
	venueCfg, err := s.venue(req.ChainKey, req.VenueKey)
	if err != nil {
		return nil, err
	}
	if venueCfg.Kind != venue.VenueKindUniswapV2 && venueCfg.Kind != venue.VenueKindAerodrome && venueCfg.Kind != venue.VenueKindUniswapV3 {
		return nil, fmt.Errorf("%w: approve requires evm venue", ErrUnsupportedSwap)
	}

	tokenIn, err := s.assetID(req.TokenInSymbol, req.ChainKey)
	if err != nil {
		return nil, err
	}
	amountIn, err := parseAmount(req.AmountIn)
	if err != nil {
		return nil, err
	}
	router, err := routerAddress(venueCfg)
	if err != nil {
		return nil, err
	}
	tx, err := evmswap.BuildApproveTransaction(string(tokenIn), router, amountIn)
	if err != nil {
		return nil, err
	}

	return &EVMTransactionResponse{
		ChainKey:  req.ChainKey,
		VenueKey:  venueCfg.Key,
		VenueKind: venueCfg.Kind,
		To:        tx.To,
		Data:      "0x" + hex.EncodeToString(tx.Data),
		Value:     tx.Value.String(),
	}, nil
}

func (s *Service) buildRequest(ctx context.Context, req Request) (*coreswap.Request, error) {
	venueCfg, err := s.venue(req.ChainKey, req.VenueKey)
	if err != nil {
		return nil, err
	}

	tokenIn, err := s.assetID(req.TokenInSymbol, req.ChainKey)
	if err != nil {
		return nil, err
	}
	tokenOut, err := s.assetID(req.TokenOutSymbol, req.ChainKey)
	if err != nil {
		return nil, err
	}
	amountIn, err := parseAmount(req.AmountIn)
	if err != nil {
		return nil, err
	}

	poolID := req.PoolID
	if poolID == "" {
		poolID, err = s.findPool(ctx, req.ChainKey, venueCfg.Key, tokenIn, tokenOut)
		if err != nil {
			return nil, err
		}
	}

	deadline := req.DeadlineUnix
	if deadline == 0 {
		deadline = uint64(time.Now().Add(20 * time.Minute).Unix())
	}
	slippage := req.SlippageBps
	if slippage == 0 {
		slippage = 50
	}

	return &coreswap.Request{
		ChainKey:     req.ChainKey,
		VenueKey:     venueCfg.Key,
		VenueKind:    venueCfg.Kind,
		PoolID:       poolID,
		TokenIn:      string(tokenIn),
		TokenOut:     string(tokenOut),
		AmountIn:     amountIn,
		Sender:       req.Sender,
		Recipient:    req.Recipient,
		SlippageBps:  slippage,
		DeadlineUnix: deadline,
	}, nil
}

func (s *Service) venue(chainKey chain.ChainKey, venueKey venue.VenueKey) (venue.Venue, error) {
	if chainKey == "" {
		return venue.Venue{}, fmt.Errorf("chain_key is required")
	}
	if venueKey == "" {
		return venue.Venue{}, fmt.Errorf("venue_key is required")
	}
	v, ok := s.venues.Get(venueKey)
	if !ok {
		return venue.Venue{}, fmt.Errorf("%w: %s", ErrUnknownVenue, venueKey)
	}
	if !v.Enabled {
		return venue.Venue{}, fmt.Errorf("%w: %s is disabled", ErrUnsupportedSwap, venueKey)
	}
	if v.ChainKey != chainKey {
		return venue.Venue{}, fmt.Errorf("venue %s is on %s, not %s", venueKey, v.ChainKey, chainKey)
	}
	return v, nil
}

func (s *Service) assetID(symbol string, chainKey chain.ChainKey) (venue.AssetID, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return "", fmt.Errorf("asset symbol is required")
	}
	item, ok := s.assets.Get(symbol)
	if !ok {
		return "", fmt.Errorf("asset is not registered: %s", symbol)
	}
	for _, deployment := range item.Deployments {
		if deployment.ChainKey != chainKey {
			continue
		}
		id := deployment.AssetID()
		if id != "" {
			return venue.AssetID(id), nil
		}
	}
	return "", fmt.Errorf("asset %s is not deployed on %s", symbol, chainKey)
}

func (s *Service) findPool(ctx context.Context, chainKey chain.ChainKey, venueKey venue.VenueKey, tokenIn, tokenOut venue.AssetID) (venue.PoolID, error) {
	pools, err := s.pools.ListPoolsByAssetIDs(ctx, []venue.AssetID{tokenIn, tokenOut})
	if err != nil {
		return "", err
	}
	for _, pool := range pools {
		if pool.ChainKey != chainKey || pool.VenueKey != venueKey || !pool.Enabled {
			continue
		}
		if poolMatches(pool, tokenIn, tokenOut) {
			return pool.ID, nil
		}
	}
	return "", fmt.Errorf("no pool found for %s -> %s on %s/%s", tokenIn, tokenOut, chainKey, venueKey)
}

func poolMatches(pool venue.Pool, tokenIn, tokenOut venue.AssetID) bool {
	token0 := normalizeAssetID(pool.ChainKey, pool.Token0)
	token1 := normalizeAssetID(pool.ChainKey, pool.Token1)
	in := normalizeAssetID(pool.ChainKey, tokenIn)
	out := normalizeAssetID(pool.ChainKey, tokenOut)
	return (token0 == in && token1 == out) || (token0 == out && token1 == in)
}

func normalizeAssetID(chainKey chain.ChainKey, id venue.AssetID) string {
	value := strings.TrimSpace(string(id))
	if chainKey != chain.ChainKeySolana {
		value = strings.ToLower(value)
	}
	return value
}

func parseAmount(value string) (*big.Int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("amount_in is required")
	}
	out, ok := new(big.Int).SetString(value, 10)
	if !ok || out.Sign() <= 0 {
		return nil, fmt.Errorf("amount_in must be a positive integer in base units")
	}
	return out, nil
}

func quoteResponse(quote coreswap.Quote) *QuoteResponse {
	return &QuoteResponse{
		ChainKey:  quote.ChainKey,
		VenueKey:  quote.VenueKey,
		VenueKind: quote.VenueKind,
		PoolID:    quote.PoolID,
		TokenIn:   quote.TokenIn,
		TokenOut:  quote.TokenOut,
		AmountIn:  bigString(quote.AmountIn),
		AmountOut: bigString(quote.AmountOut),
		MinOut:    bigString(quote.MinOut),
		FeeBps:    quote.FeeBps,
	}
}

func evmTransactionResponse(intent *coreswap.TransactionIntent) *EVMTransactionResponse {
	if intent == nil || intent.EVM == nil {
		return nil
	}
	return &EVMTransactionResponse{
		ChainKey:  intent.ChainKey,
		VenueKey:  intent.VenueKey,
		VenueKind: intent.VenueKind,
		To:        intent.EVM.To,
		Data:      "0x" + hex.EncodeToString(intent.EVM.Data),
		Value:     bigString(intent.EVM.Value),
	}
}

func routerAddress(v venue.Venue) (string, error) {
	switch cfg := v.Config.(type) {
	case venue.UniswapV2Config:
		if cfg.RouterAddress != "" {
			return cfg.RouterAddress, nil
		}
	case venue.AerodromeClassicConfig:
		if cfg.RouterAddress != "" {
			return cfg.RouterAddress, nil
		}
	case venue.UniswapV3Config:
		if cfg.RouterAddress != "" {
			return cfg.RouterAddress, nil
		}
	}
	return "", fmt.Errorf("router address is required for venue %s", v.Key)
}

func bigString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}
