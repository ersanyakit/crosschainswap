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
	Asset  AssetInfo   `json:"asset"`
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
	InversePrice string         `json:"inverse_price,omitempty"`
	BaseUSDC     string         `json:"base_price_usdc,omitempty"`
	PriceUSDC    string         `json:"price_usdc,omitempty"`
	QuoteUSDC    string         `json:"quote_price_usdc,omitempty"`
	USDCRoute    *USDCRoute     `json:"usdc_route,omitempty"`
	BaseAsset    DeploymentInfo `json:"base_asset"`
	QuoteAsset   DeploymentInfo `json:"quote_asset"`
	ReserveBase  string         `json:"reserve_base"`
	ReserveQuote string         `json:"reserve_quote"`
	PoolKind     venue.PoolKind `json:"pool_kind"`
}

type USDCRoute struct {
	ChainKey    chain.ChainKey `json:"chain_key"`
	VenueKey    venue.VenueKey `json:"venue_key"`
	PoolID      venue.PoolID   `json:"pool_id"`
	FromSymbol  string         `json:"from_symbol"`
	FromAssetID venue.AssetID  `json:"from_asset_id"`
	ToSymbol    string         `json:"to_symbol"`
	ToAssetID   venue.AssetID  `json:"to_asset_id"`
	Price       string         `json:"price"`
}

type AssetInfo struct {
	Symbol      string           `json:"symbol"`
	Name        string           `json:"name"`
	Type        string           `json:"type"`
	Decimals    int              `json:"decimals"`
	IconURL     string           `json:"icon_url,omitempty"`
	Deployments []DeploymentInfo `json:"deployments"`
}

type DeploymentInfo struct {
	ChainKey chain.ChainKey `json:"chain_key"`
	AssetID  venue.AssetID  `json:"asset_id"`
	Address  string         `json:"address,omitempty"`
	Mint     string         `json:"mint,omitempty"`
	Symbol   string         `json:"symbol"`
	Name     string         `json:"name"`
	Decimals int            `json:"decimals"`
	Enabled  bool           `json:"enabled"`
	IconURL  string         `json:"icon_url,omitempty"`
}

type deploymentRef struct {
	Symbol           string
	DeploymentSymbol string
	Name             string
	Type             string
	ChainKey         chain.ChainKey
	AssetID          venue.AssetID
	Address          string
	Mint             string
	Decimals         int
	Enabled          bool
	IconURL          string
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

	if err := s.enrichUSDCPrices(ctx, prices, allDeployments); err != nil {
		return nil, err
	}

	return &AssetPrices{Symbol: symbol, Asset: assetInfo(target), Prices: prices}, nil
}

func (s *Service) Assets() []AssetInfo {
	items := s.assets.All()
	out := make([]AssetInfo, 0, len(items))
	for _, item := range items {
		out = append(out, assetInfo(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol < out[j].Symbol
	})
	return out
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
	if pool.Kind == venue.PoolKindCLMM && !positive(pool.SqrtPriceX96) {
		return PoolPrice{}, false
	}

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
		InversePrice: inverseDecimalString(price),
		BaseAsset:    deploymentInfo(base),
		QuoteAsset:   deploymentInfo(quote),
		ReserveBase:  bigString(baseReserve),
		ReserveQuote: bigString(quoteReserve),
		PoolKind:     pool.Kind,
	}, true
}

func (s *Service) enrichUSDCPrices(
	ctx context.Context,
	prices []PoolPrice,
	allDeployments map[string]deploymentRef,
) error {
	if len(prices) == 0 {
		return nil
	}

	usdcRefs := refsBySymbol(allDeployments, "USDC")
	if len(usdcRefs) == 0 {
		return nil
	}

	assetIDs := make([]venue.AssetID, 0)
	seen := make(map[string]struct{})
	for _, price := range prices {
		for _, candidate := range []struct {
			symbol  string
			chain   chain.ChainKey
			assetID venue.AssetID
		}{
			{symbol: price.BaseSymbol, chain: price.ChainKey, assetID: price.BaseAssetID},
			{symbol: price.QuoteSymbol, chain: price.ChainKey, assetID: price.QuoteAssetID},
		} {
			if strings.EqualFold(candidate.symbol, "USDC") {
				continue
			}
			key := deploymentKey(candidate.chain, candidate.assetID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			assetIDs = append(assetIDs, candidate.assetID)
		}
	}
	if len(assetIDs) == 0 {
		for i := range prices {
			if strings.EqualFold(prices[i].BaseSymbol, "USDC") {
				prices[i].BaseUSDC = "1"
				prices[i].PriceUSDC = "1"
			}
			if strings.EqualFold(prices[i].QuoteSymbol, "USDC") {
				prices[i].QuoteUSDC = "1"
				if prices[i].BaseUSDC == "" {
					prices[i].BaseUSDC = prices[i].Price
					prices[i].PriceUSDC = prices[i].BaseUSDC
				}
			}
		}
		return nil
	}

	ids := append([]venue.AssetID{}, assetIDs...)
	for _, ref := range usdcRefs {
		ids = append(ids, ref.AssetID)
	}

	pools, err := s.pools.ListPoolsByAssetIDs(ctx, ids)
	if err != nil {
		return err
	}

	usdcByChain := make(map[chain.ChainKey]deploymentRef)
	for _, ref := range usdcRefs {
		usdcByChain[ref.ChainKey] = ref
	}

	conversions := make(map[string]usdcConversion)
	for _, pool := range pools {
		usdc, ok := usdcByChain[pool.ChainKey]
		if !ok {
			continue
		}
		token0Key := deploymentKey(pool.ChainKey, pool.Token0)
		token1Key := deploymentKey(pool.ChainKey, pool.Token1)
		usdcKey := deploymentKey(pool.ChainKey, usdc.AssetID)
		if token0Key != usdcKey && token1Key != usdcKey {
			continue
		}

		var base deploymentRef
		var okBase bool
		if token0Key == usdcKey {
			base, okBase = allDeployments[token1Key]
		} else {
			base, okBase = allDeployments[token0Key]
		}
		if !okBase || strings.EqualFold(base.Symbol, "USDC") {
			continue
		}

		price, ok := poolPriceForBase(pool, base, usdc)
		if !ok {
			continue
		}
		conversion := usdcConversion{
			price:         price,
			route:         usdcRoute(pool, base, usdc, price),
			usdcReserve:   reserveForAsset(pool, usdc.AssetID),
			poolLiquidity: pool.Liquidity,
		}
		key := deploymentKey(base.ChainKey, base.AssetID)
		if existing, ok := conversions[key]; !ok || betterUSDCConversion(conversion, existing) {
			conversions[key] = conversion
		}
	}

	for i := range prices {
		if strings.EqualFold(prices[i].BaseSymbol, "USDC") {
			prices[i].BaseUSDC = "1"
		} else if conversion, ok := conversions[deploymentKey(prices[i].ChainKey, prices[i].BaseAssetID)]; ok {
			prices[i].BaseUSDC = conversion.price
			prices[i].USDCRoute = &conversion.route
		}

		if strings.EqualFold(prices[i].QuoteSymbol, "USDC") {
			prices[i].QuoteUSDC = "1"
		} else if conversion, ok := conversions[deploymentKey(prices[i].ChainKey, prices[i].QuoteAssetID)]; ok {
			prices[i].QuoteUSDC = conversion.price
			if prices[i].USDCRoute == nil {
				prices[i].USDCRoute = &conversion.route
			}
		}

		if prices[i].BaseUSDC == "" && prices[i].QuoteUSDC != "" {
			baseUSDC, ok := multiplyDecimalStrings(prices[i].Price, prices[i].QuoteUSDC)
			if ok {
				prices[i].BaseUSDC = baseUSDC
			}
		}
		if prices[i].QuoteUSDC == "" && prices[i].BaseUSDC != "" {
			quoteUSDC, ok := divideDecimalStrings(prices[i].BaseUSDC, prices[i].Price)
			if ok {
				prices[i].QuoteUSDC = quoteUSDC
			}
		}
		prices[i].PriceUSDC = prices[i].BaseUSDC
	}
	return nil
}

type usdcConversion struct {
	price         string
	route         USDCRoute
	usdcReserve   *big.Int
	poolLiquidity *big.Int
}

func usdcRoute(pool venue.Pool, base deploymentRef, usdc deploymentRef, price string) USDCRoute {
	return USDCRoute{
		ChainKey:    pool.ChainKey,
		VenueKey:    pool.VenueKey,
		PoolID:      pool.ID,
		FromSymbol:  base.Symbol,
		FromAssetID: base.AssetID,
		ToSymbol:    usdc.Symbol,
		ToAssetID:   usdc.AssetID,
		Price:       price,
	}
}

func betterUSDCConversion(candidate usdcConversion, existing usdcConversion) bool {
	if positive(candidate.usdcReserve) || positive(existing.usdcReserve) {
		return compareBig(candidate.usdcReserve, existing.usdcReserve) > 0
	}
	return compareBig(candidate.poolLiquidity, existing.poolLiquidity) > 0
}

func reserveForAsset(pool venue.Pool, assetID venue.AssetID) *big.Int {
	token0 := deploymentKey(pool.ChainKey, pool.Token0)
	token1 := deploymentKey(pool.ChainKey, pool.Token1)
	target := deploymentKey(pool.ChainKey, assetID)
	switch target {
	case token0:
		return pool.Reserve0
	case token1:
		return pool.Reserve1
	default:
		return nil
	}
}

func poolPriceForBase(pool venue.Pool, base deploymentRef, quote deploymentRef) (string, bool) {
	if pool.Kind == venue.PoolKindCLMM && !positive(pool.SqrtPriceX96) {
		return "", false
	}

	baseKey := deploymentKey(pool.ChainKey, base.AssetID)
	quoteKey := deploymentKey(pool.ChainKey, quote.AssetID)
	token0Key := deploymentKey(pool.ChainKey, pool.Token0)
	token1Key := deploymentKey(pool.ChainKey, pool.Token1)

	switch {
	case token0Key == baseKey && token1Key == quoteKey:
		if positive(pool.Reserve0) && positive(pool.Reserve1) {
			return reservePrice(pool.Reserve0, pool.Reserve1, base.Decimals, quote.Decimals), true
		}
		if positive(pool.SqrtPriceX96) {
			return sqrtPrice(pool.SqrtPriceX96, base.Decimals, quote.Decimals, true), true
		}
	case token1Key == baseKey && token0Key == quoteKey:
		if positive(pool.Reserve1) && positive(pool.Reserve0) {
			return reservePrice(pool.Reserve1, pool.Reserve0, base.Decimals, quote.Decimals), true
		}
		if positive(pool.SqrtPriceX96) {
			return sqrtPrice(pool.SqrtPriceX96, base.Decimals, quote.Decimals, false), true
		}
	}
	return "", false
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
			Symbol:           item.Symbol,
			DeploymentSymbol: effectiveSymbol(item.Symbol, deployment.Symbol),
			Name:             effectiveName(item.Name, deployment.Name),
			Type:             item.Type,
			ChainKey:         deployment.ChainKey,
			AssetID:          venue.AssetID(id),
			Address:          deployment.Address,
			Mint:             deployment.Mint,
			Decimals:         decimals,
			Enabled:          true,
			IconURL:          effectiveIconURL(item.IconURL, deployment.IconURL),
		})
	}
	return out
}

func assetInfo(item asset.Asset) AssetInfo {
	return AssetInfo{
		Symbol:      item.Symbol,
		Name:        item.Name,
		Type:        item.Type,
		Decimals:    item.Decimals,
		IconURL:     item.IconURL,
		Deployments: deploymentInfos(item),
	}
}

func deploymentInfos(item asset.Asset) []DeploymentInfo {
	refs := deploymentRefs(item)
	out := make([]DeploymentInfo, 0, len(refs))
	for _, ref := range refs {
		out = append(out, deploymentInfo(ref))
	}
	return out
}

func deploymentInfo(ref deploymentRef) DeploymentInfo {
	return DeploymentInfo{
		ChainKey: ref.ChainKey,
		AssetID:  ref.AssetID,
		Address:  ref.Address,
		Mint:     ref.Mint,
		Symbol:   ref.DeploymentSymbol,
		Name:     ref.Name,
		Decimals: ref.Decimals,
		Enabled:  ref.Enabled,
		IconURL:  ref.IconURL,
	}
}

func effectiveIconURL(assetIconURL string, deploymentIconURL string) string {
	if deploymentIconURL != "" {
		return deploymentIconURL
	}
	return assetIconURL
}

func effectiveSymbol(assetSymbol string, deploymentSymbol string) string {
	if strings.TrimSpace(deploymentSymbol) != "" {
		return deploymentSymbol
	}
	return assetSymbol
}

func effectiveName(assetName string, deploymentName string) string {
	if strings.TrimSpace(deploymentName) != "" {
		return deploymentName
	}
	return assetName
}

func refsBySymbol(deployments map[string]deploymentRef, symbol string) []deploymentRef {
	out := make([]deploymentRef, 0)
	for _, ref := range deployments {
		if strings.EqualFold(ref.Symbol, symbol) {
			out = append(out, ref)
		}
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

func multiplyDecimalStrings(left string, right string) (string, bool) {
	leftRat, ok := new(big.Rat).SetString(left)
	if !ok {
		return "", false
	}
	rightRat, ok := new(big.Rat).SetString(right)
	if !ok {
		return "", false
	}
	out := new(big.Rat).Mul(leftRat, rightRat)
	return trimDecimal(out.FloatString(18)), true
}

func divideDecimalStrings(left string, right string) (string, bool) {
	leftRat, ok := new(big.Rat).SetString(left)
	if !ok {
		return "", false
	}
	rightRat, ok := new(big.Rat).SetString(right)
	if !ok || rightRat.Sign() == 0 {
		return "", false
	}
	out := new(big.Rat).Quo(leftRat, rightRat)
	return trimDecimal(out.FloatString(18)), true
}

func inverseDecimalString(value string) string {
	rat, ok := new(big.Rat).SetString(value)
	if !ok || rat.Sign() == 0 {
		return ""
	}
	out := new(big.Rat).Inv(rat)
	return trimDecimal(out.FloatString(18))
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

func compareBig(left *big.Int, right *big.Int) int {
	if left == nil {
		left = big.NewInt(0)
	}
	if right == nil {
		right = big.NewInt(0)
	}
	return left.Cmp(right)
}

func trimDecimal(value string) string {
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	if value == "" {
		return "0"
	}
	return value
}
