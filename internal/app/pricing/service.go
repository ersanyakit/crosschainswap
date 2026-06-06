package pricing

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	"exchange/internal/core/venue"
)

var (
	ErrSymbolRequired    = errors.New("symbol is required")
	ErrUnknownAsset      = errors.New("asset is not registered")
	ErrNoAssetDeployment = errors.New("asset has no registered deployments")
)

type PoolStore interface {
	ListPoolsByAssetIDs(ctx context.Context, ids []venue.AssetID) ([]venue.Pool, error)
}

type Service struct {
	assets asset.Registry
	pools  PoolStore
}

type AssetPrices struct {
	Symbol string      `json:"symbol"`
	Prices []PoolPrice `json:"prices"`
}

type PoolPrice struct {
	ChainKey     chain.ChainKey `json:"chain_key"`
	VenueKey     venue.VenueKey `json:"venue_key"`
	PoolID       venue.PoolID   `json:"pool_id"`
	BaseSymbol   string         `json:"base_symbol"`
	BaseAssetID  venue.AssetID  `json:"base_asset_id"`
	QuoteSymbol  string         `json:"quote_symbol"`
	QuoteAssetID venue.AssetID  `json:"quote_asset_id"`
	Price        string         `json:"price"`
	ReserveBase  string         `json:"reserve_base"`
	ReserveQuote string         `json:"reserve_quote"`
	PoolKind     venue.PoolKind `json:"pool_kind"`
}

type deploymentRef struct {
	Symbol   string
	ChainKey chain.ChainKey
	AssetID  venue.AssetID
	Decimals int
}

func NewService(assets asset.Registry, pools PoolStore) *Service {
	return &Service{
		assets: assets,
		pools:  pools,
	}
}

func (s *Service) Prices(ctx context.Context, symbol string) (*AssetPrices, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, ErrSymbolRequired
	}

	target, ok := s.assets.Get(symbol)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAsset, symbol)
	}

	targetRefs := deploymentRefs(target)
	if len(targetRefs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoAssetDeployment, symbol)
	}

	targetIDs := make([]venue.AssetID, 0, len(targetRefs))
	targetByKey := make(map[string]deploymentRef, len(targetRefs))
	for _, ref := range targetRefs {
		targetIDs = append(targetIDs, ref.AssetID)
		targetByKey[deploymentKey(ref.ChainKey, ref.AssetID)] = ref
	}

	pools, err := s.pools.ListPoolsByAssetIDs(ctx, targetIDs)
	if err != nil {
		return nil, err
	}

	allDeployments := s.deploymentIndex()
	prices := make([]PoolPrice, 0, len(pools))
	for _, pool := range pools {
		price, ok := s.poolPrice(symbol, pool, targetByKey, allDeployments)
		if ok {
			prices = append(prices, price)
		}
	}

	sort.Slice(prices, func(i, j int) bool {
		a, b := prices[i], prices[j]
		if a.ChainKey != b.ChainKey {
			return a.ChainKey < b.ChainKey
		}
		if a.QuoteSymbol != b.QuoteSymbol {
			return a.QuoteSymbol < b.QuoteSymbol
		}
		if a.VenueKey != b.VenueKey {
			return a.VenueKey < b.VenueKey
		}
		return a.PoolID < b.PoolID
	})

	return &AssetPrices{Symbol: symbol, Prices: prices}, nil
}

func (s *Service) SymbolsForPools(pools []venue.Pool) []string {
	if len(pools) == 0 {
		return nil
	}

	deployments := s.deploymentIndex()
	seen := make(map[string]struct{})
	for _, pool := range pools {
		for _, token := range []venue.AssetID{pool.Token0, pool.Token1} {
			ref, ok := deployments[deploymentKey(pool.ChainKey, token)]
			if !ok {
				continue
			}
			seen[ref.Symbol] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for symbol := range seen {
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out
}

func (s *Service) poolPrice(
	symbol string,
	pool venue.Pool,
	targetByKey map[string]deploymentRef,
	allDeployments map[string]deploymentRef,
) (PoolPrice, bool) {
	token0Key := deploymentKey(pool.ChainKey, pool.Token0)
	token1Key := deploymentKey(pool.ChainKey, pool.Token1)

	var base, quote deploymentRef
	var baseAssetID, quoteAssetID venue.AssetID
	var baseReserve, quoteReserve *big.Int
	var baseIsToken0 bool

	if ref, ok := targetByKey[token0Key]; ok {
		base = ref
		baseAssetID = pool.Token0
		baseReserve = pool.Reserve0
		baseIsToken0 = true
		quoteAssetID = pool.Token1
		quoteReserve = pool.Reserve1
		quote, ok = allDeployments[token1Key]
		if !ok {
			return PoolPrice{}, false
		}
	} else if ref, ok := targetByKey[token1Key]; ok {
		base = ref
		baseAssetID = pool.Token1
		baseReserve = pool.Reserve1
		quoteAssetID = pool.Token0
		quoteReserve = pool.Reserve0
		quote, ok = allDeployments[token0Key]
		if !ok {
			return PoolPrice{}, false
		}
	} else {
		return PoolPrice{}, false
	}

	if quote.Symbol == symbol || !positive(baseReserve) || !positive(quoteReserve) {
		if quote.Symbol == symbol || !positive(pool.SqrtPriceX96) {
			return PoolPrice{}, false
		}
	}

	price := ""
	if positive(baseReserve) && positive(quoteReserve) {
		price = reservePrice(baseReserve, quoteReserve, base.Decimals, quote.Decimals)
	} else {
		price = sqrtPrice(pool.SqrtPriceX96, base.Decimals, quote.Decimals, baseIsToken0)
	}

	return PoolPrice{
		ChainKey:     pool.ChainKey,
		VenueKey:     pool.VenueKey,
		PoolID:       pool.ID,
		BaseSymbol:   base.Symbol,
		BaseAssetID:  baseAssetID,
		QuoteSymbol:  quote.Symbol,
		QuoteAssetID: quoteAssetID,
		Price:        price,
		ReserveBase:  bigString(baseReserve),
		ReserveQuote: bigString(quoteReserve),
		PoolKind:     pool.Kind,
	}, true
}

func (s *Service) deploymentIndex() map[string]deploymentRef {
	assets := s.assets.All()
	out := make(map[string]deploymentRef)
	for _, item := range assets {
		for _, ref := range deploymentRefs(item) {
			out[deploymentKey(ref.ChainKey, ref.AssetID)] = ref
		}
	}
	return out
}

func deploymentRefs(item asset.Asset) []deploymentRef {
	out := make([]deploymentRef, 0, len(item.Deployments))
	for _, deployment := range item.Deployments {
		id := strings.TrimSpace(deployment.AssetID())
		if id == "" {
			continue
		}
		decimals := deployment.Decimals
		if decimals == 0 {
			decimals = item.Decimals
		}
		out = append(out, deploymentRef{
			Symbol:   item.Symbol,
			ChainKey: deployment.ChainKey,
			AssetID:  venue.AssetID(id),
			Decimals: decimals,
		})
	}
	return out
}

func deploymentKey(chainKey chain.ChainKey, assetID venue.AssetID) string {
	id := strings.TrimSpace(string(assetID))
	if chainKey != chain.ChainKeySolana {
		id = strings.ToLower(id)
	}
	return string(chainKey) + ":" + id
}

func reservePrice(baseReserve, quoteReserve *big.Int, baseDecimals, quoteDecimals int) string {
	numerator := new(big.Int).Set(quoteReserve)
	numerator.Mul(numerator, pow10(baseDecimals))

	denominator := new(big.Int).Set(baseReserve)
	denominator.Mul(denominator, pow10(quoteDecimals))

	rat := new(big.Rat).SetFrac(numerator, denominator)
	return trimDecimal(rat.FloatString(18))
}

func sqrtPrice(sqrtPriceX96 *big.Int, baseDecimals, quoteDecimals int, baseIsToken0 bool) string {
	ratio := new(big.Int).Mul(sqrtPriceX96, sqrtPriceX96)
	q192 := new(big.Int).Lsh(big.NewInt(1), 192)

	var numerator, denominator *big.Int
	if baseIsToken0 {
		numerator = new(big.Int).Mul(ratio, pow10(baseDecimals))
		denominator = new(big.Int).Mul(q192, pow10(quoteDecimals))
	} else {
		numerator = new(big.Int).Mul(q192, pow10(baseDecimals))
		denominator = new(big.Int).Mul(ratio, pow10(quoteDecimals))
	}

	rat := new(big.Rat).SetFrac(numerator, denominator)
	return trimDecimal(rat.FloatString(18))
}

func bigString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

func pow10(decimals int) *big.Int {
	if decimals < 0 {
		decimals = 0
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
}

func positive(v *big.Int) bool {
	return v != nil && v.Sign() > 0
}

func trimDecimal(value string) string {
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	if value == "" {
		return "0"
	}
	return value
}
