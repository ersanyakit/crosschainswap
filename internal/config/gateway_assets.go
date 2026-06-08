package config

import (
	"context"
	"log"
	"net/url"
	"strings"

	"exchange/internal/adapters/paymentgateway"
	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	"exchange/internal/core/market"
)

func LoadRegistries(ctx context.Context) Registries {
	registries := LoadDefaultRegistries()
	cfg := paymentgateway.ConfigFromEnv()
	client := paymentgateway.NewClient(cfg)

	assetCtx := ctx
	cancel := func() {}
	if cfg.Timeout > 0 {
		assetCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	defer cancel()

	gatewayAssets, err := client.Assets(assetCtx)
	if err != nil {
		log.Printf("Payment gateway asset registry unavailable, using local defaults: %v", err)
		return registries
	}

	assets := gatewayAssetsToRegistryAssets(gatewayAssets, client.BaseURL())
	if len(assets) == 0 {
		log.Printf("Payment gateway asset registry returned no usable assets, using local defaults")
		return registries
	}

	registries.Assets = asset.NewRegistry(assets)
	registries.Markets = market.NewRegistry(usdMarketsForAssets(assets))
	log.Printf("Loaded %d assets from payment gateway registry", len(assets))
	return registries
}

func gatewayAssetsToRegistryAssets(items []paymentgateway.Asset, gatewayBaseURL string) []asset.Asset {
	out := make([]asset.Asset, 0, len(items))
	index := make(map[string]int, len(items))
	deploymentSeen := make(map[string]struct{})

	for _, item := range items {
		symbol := normalizeGatewaySymbol(item.Symbol)
		if symbol == "" {
			continue
		}

		pos, ok := index[symbol]
		if !ok {
			out = append(out, asset.Asset{
				Symbol:   symbol,
				Name:     firstNonEmpty(item.Name, symbol),
				Type:     firstNonEmpty(item.Type, "token"),
				Decimals: gatewayAssetDecimals(item),
				IconURL:  gatewayAssetURL(gatewayBaseURL, item.LogoURL),
			})
			pos = len(out) - 1
			index[symbol] = pos
		}

		for _, deployment := range item.Deployments {
			mapped, ok := gatewayDeploymentToRegistryDeployment(item, deployment, gatewayBaseURL)
			if !ok {
				continue
			}
			key := string(mapped.ChainKey) + ":" + strings.ToLower(mapped.AssetID())
			if _, ok := deploymentSeen[symbol+":"+key]; ok {
				continue
			}
			deploymentSeen[symbol+":"+key] = struct{}{}
			out[pos].Deployments = append(out[pos].Deployments, mapped)
		}
	}

	return out
}

func gatewayDeploymentToRegistryDeployment(item paymentgateway.Asset, deployment paymentgateway.AssetDeployment, gatewayBaseURL string) (asset.Deployment, bool) {
	if !deployment.Enabled {
		return asset.Deployment{}, false
	}

	chainKey, ok := gatewayChainKey(deployment.Network, deployment.ChainID)
	if !ok {
		return asset.Deployment{}, false
	}

	address := strings.TrimSpace(deployment.TokenAddress)
	mint := strings.TrimSpace(deployment.MintAddress)
	native := deployment.Native || strings.EqualFold(deployment.Type, "native")
	if chainKey == chain.ChainKeySolana {
		if !native && mint == "" {
			return asset.Deployment{}, false
		}
		address = ""
		if native {
			mint = ""
		}
	} else if !native && address == "" {
		return asset.Deployment{}, false
	} else if native {
		address = ""
	}

	decimals := deployment.Decimals
	if decimals == 0 {
		decimals = item.Decimals
	}

	return asset.Deployment{
		ChainKey:     chainKey,
		Address:      address,
		Mint:         mint,
		Symbol:       normalizeGatewaySymbol(deployment.Symbol),
		Name:         deployment.Name,
		Decimals:     decimals,
		Enabled:      deployment.Enabled,
		Native:       native,
		IconURL:      gatewayAssetURL(gatewayBaseURL, firstNonEmpty(deployment.LogoURL, item.LogoURL)),
		ChainLogoURL: gatewayAssetURL(gatewayBaseURL, deployment.ChainLogoURL),
	}, true
}

func gatewayChainKey(network string, chainID int64) (chain.ChainKey, bool) {
	switch normalizeGatewayNetwork(network) {
	case "ethereum":
		return chain.ChainKeyEthereum, true
	case "base":
		return chain.ChainKeyBase, true
	case "chiliz":
		return chain.ChainKeyChiliz, true
	case "solana":
		return chain.ChainKeySolana, true
	case "avalanche":
		return chain.ChainKeyAvalanche, true
	case "arbitrum":
		return chain.ChainKeyArbitrum, true
	case "unichain":
		return chain.ChainKeyUnichain, true
	case "bnbchain", "bnb-chain", "bsc", "binance-smart-chain", "binance-smart-chain-mainnet":
		return chain.ChainKeyBinanceSmartChain, true
	case "tron":
		return chain.ChainKey("tron"), true
	case "bitcoin":
		return chain.ChainKey("bitcoin"), true
	case "chiliz-spicy":
		return "", false
	}

	switch chainID {
	case 1:
		return chain.ChainKeyEthereum, true
	case 8453:
		return chain.ChainKeyBase, true
	case 88888:
		return chain.ChainKeyChiliz, true
	case 99999999:
		return chain.ChainKeySolana, true
	case 43114:
		return chain.ChainKeyAvalanche, true
	case 42161:
		return chain.ChainKeyArbitrum, true
	case 130:
		return chain.ChainKeyUnichain, true
	case 56:
		return chain.ChainKeyBinanceSmartChain, true
	case 99999998:
		return chain.ChainKey("tron"), true
	case 0:
		return chain.ChainKey("bitcoin"), true
	case 88882:
		return "", false
	default:
		return "", false
	}
}

func gatewayAssetDecimals(item paymentgateway.Asset) int {
	if item.Decimals != 0 {
		return item.Decimals
	}
	for _, deployment := range item.Deployments {
		if deployment.Decimals != 0 {
			return deployment.Decimals
		}
	}
	return 0
}

func gatewayAssetURL(baseURL string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}

	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/")
	if err != nil || base.Scheme == "" || base.Host == "" {
		return value
	}
	rel, err := url.Parse(value)
	if err != nil {
		return value
	}
	return base.ResolveReference(rel).String()
}

func normalizeGatewaySymbol(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeGatewayNetwork(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
