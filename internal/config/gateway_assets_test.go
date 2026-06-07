package config

import (
	"testing"

	"exchange/internal/adapters/paymentgateway"
	"exchange/internal/core/chain"
)

func TestGatewayAssetsToRegistryAssetsMapsTokenDeployments(t *testing.T) {
	assets := gatewayAssetsToRegistryAssets([]paymentgateway.Asset{
		{
			Symbol:   "eth",
			Name:     "Ether",
			Type:     "native",
			Decimals: 18,
			LogoURL:  "/static/coins/eth.svg",
			Deployments: []paymentgateway.AssetDeployment{
				{Symbol: "ETH", Name: "Base Ether", Network: "base", ChainID: 8453, Decimals: 18, Native: true, Enabled: true, Identifier: "ETH", LogoURL: "/static/coins/eth.svg"},
				{Symbol: "WETH", Name: "Wrapped Ether", Network: "base", ChainID: 8453, Decimals: 18, Enabled: true, TokenAddress: "0x4200000000000000000000000000000000000006", LogoURL: "/static/coins/eth.svg", ChainLogoURL: "/static/chains/base.svg"},
			},
		},
		{
			Symbol:   "sol",
			Name:     "Solana",
			Type:     "native",
			Decimals: 9,
			LogoURL:  "/static/coins/sol.svg",
			Deployments: []paymentgateway.AssetDeployment{
				{Symbol: "SOL", Name: "Solana", Network: "solana", ChainID: 99999999, Decimals: 9, Native: true, Enabled: true, Identifier: "SOL", LogoURL: "/static/coins/sol.svg"},
				{Symbol: "WSOL", Name: "Wrapped Solana", Network: "solana", ChainID: 99999999, Decimals: 9, Enabled: true, MintAddress: "So11111111111111111111111111111111111111112", LogoURL: "/static/coins/sol.svg"},
			},
		},
		{
			Symbol:   "chz",
			Name:     "Chiliz",
			Type:     "native",
			Decimals: 18,
			LogoURL:  "/static/coins/chz.svg",
			Deployments: []paymentgateway.AssetDeployment{
				{Symbol: "CHZ", Name: "Chiliz Spicy", Network: "chiliz-spicy", ChainID: 88882, Decimals: 18, Native: true, Enabled: true, Identifier: "CHZ", LogoURL: "/static/coins/chz.svg"},
				{Symbol: "WCHZ", Name: "Wrapped Chiliz", Network: "chiliz", ChainID: 88888, Decimals: 18, Enabled: true, TokenAddress: "0x677f7e16c7dd57be1d4c8ad1244883214953dc47", LogoURL: "/static/coins/chz.svg"},
			},
		},
	}, "http://localhost:3001")

	bySymbol := make(map[string]int, len(assets))
	for i, item := range assets {
		bySymbol[item.Symbol] = i
	}

	eth := assets[bySymbol["ETH"]]
	if eth.IconURL != "http://localhost:3001/static/coins/eth.svg" {
		t.Fatalf("unexpected ETH icon URL: %q", eth.IconURL)
	}
	if len(eth.Deployments) != 1 {
		t.Fatalf("expected only wrapped ETH deployment, got %#v", eth.Deployments)
	}
	if eth.Deployments[0].ChainKey != chain.ChainKeyBase || eth.Deployments[0].Symbol != "WETH" || eth.Deployments[0].Address != "0x4200000000000000000000000000000000000006" {
		t.Fatalf("unexpected ETH deployment: %#v", eth.Deployments[0])
	}
	if eth.Deployments[0].ChainLogoURL != "http://localhost:3001/static/chains/base.svg" {
		t.Fatalf("unexpected ETH deployment chain logo URL: %q", eth.Deployments[0].ChainLogoURL)
	}

	sol := assets[bySymbol["SOL"]]
	if len(sol.Deployments) != 1 || sol.Deployments[0].Mint != "So11111111111111111111111111111111111111112" {
		t.Fatalf("expected only wrapped SOL mint deployment, got %#v", sol.Deployments)
	}

	chz := assets[bySymbol["CHZ"]]
	if len(chz.Deployments) != 1 || chz.Deployments[0].ChainKey != chain.ChainKeyChiliz {
		t.Fatalf("expected only mainnet CHZ token deployment, got %#v", chz.Deployments)
	}
}

func TestGatewayAssetURLKeepsAbsoluteValues(t *testing.T) {
	got := gatewayAssetURL("http://localhost:3001", "https://cdn.example.test/coin.svg")
	if got != "https://cdn.example.test/coin.svg" {
		t.Fatalf("unexpected absolute URL: %q", got)
	}
}
