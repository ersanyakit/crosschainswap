package config

import (
	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	"exchange/internal/core/market"
	"exchange/internal/core/venue"
	"os"
	"strings"
)

type Registries struct {
	Chains  chain.Registry
	Assets  asset.Registry
	Venues  venue.Registry
	Markets market.Registry
}

func ptr(v int64) *int64 { return &v }

func LoadDefaultRegistries() Registries {
	chains := []chain.Chain{
		{
			Key:         chain.ChainKeyChiliz,
			Name:        "Chiliz Chain",
			Kind:        chain.KindEVM,
			ChainID:     ptr(88888),
			Network:     "mainnet",
			NativeAsset: "CHZ",
			RPCURLs: rpcURLsFromEnv("CHILIZ_RPC_URLS", chain.RPCURLs{
				"https://rpc.chiliz.com",
				"https://chiliz.publicnode.com",
				"https://rpc.ankr.com/chiliz",
			}),
			ExplorerURL:       "https://chiliscan.com",
			Confirmations:     3,
			Multicall3Address: "0xcA11bde05977b3631167028862bE2a173976CA11",
			Enabled:           true,
		},
		{
			Key:         chain.ChainKeySolana,
			Name:        "Solana Mainnet",
			Kind:        chain.KindSolana,
			Network:     "mainnet-beta",
			NativeAsset: "SOL",
			RPCURLs: rpcURLsFromEnv("SOLANA_RPC_URLS", chain.RPCURLs{
				"https://api.mainnet-beta.solana.com",
				"https://solana-rpc.publicnode.com",
			}),
			ExplorerURL:   "https://solscan.io",
			Confirmations: 32,
			Enabled:       envBoolOrDefault("SOLANA_ENABLED", true),
		},
		{
			Key:         chain.ChainKeyBase,
			Name:        "Base",
			Kind:        chain.KindEVM,
			ChainID:     ptr(8453),
			Network:     "mainnet",
			NativeAsset: "ETH",
			RPCURLs: rpcURLsFromEnv("BASE_RPC_URLS", chain.RPCURLs{
				"https://base-rpc.publicnode.com",
				"https://mainnet.base.org",
				"https://1rpc.io/base",
				"https://base.drpc.org",
			}),
			ExplorerURL:       "https://basescan.org",
			Confirmations:     3,
			Enabled:           true,
			Multicall3Address: "0xcA11bde05977b3631167028862bE2a173976CA11",
		},
		{
			Key:         chain.ChainKeyEthereum,
			Name:        "Ethereum",
			Kind:        chain.KindEVM,
			ChainID:     ptr(1),
			Network:     "mainnet",
			NativeAsset: "ETH",
			RPCURLs: rpcURLsFromEnv("ETHEREUM_RPC_URLS", chain.RPCURLs{
				"https://ethereum-rpc.publicnode.com",
				"https://eth.llamarpc.com",
				"https://rpc.ankr.com/eth",
			}),
			ExplorerURL:       "https://etherscan.io",
			Confirmations:     6,
			Enabled:           true,
			Multicall3Address: "0xcA11bde05977b3631167028862bE2a173976CA11",
		},
		{
			Key:         chain.ChainKeyAvalanche,
			Name:        "Avalanche C-Chain",
			Kind:        chain.KindEVM,
			ChainID:     ptr(43114),
			Network:     "mainnet",
			NativeAsset: "AVAX",
			RPCURLs: rpcURLsFromEnv("AVALANCHE_RPC_URLS", chain.RPCURLs{
				"https://api.avax.network/ext/bc/C/rpc",
				"https://avalanche-c-chain-rpc.publicnode.com",
				"https://rpc.ankr.com/avalanche",
			}),
			ExplorerURL:       "https://snowtrace.io",
			Confirmations:     3,
			Enabled:           true,
			Multicall3Address: "0xcA11bde05977b3631167028862bE2a173976CA11",
		},
	}

	assets := []asset.Asset{
		{Symbol: "BTC", Name: "Bitcoin", Type: "native", Decimals: 8, IconURL: coinMarketCapIconURL("1"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKey("bitcoin"), Native: true, Decimals: 8, Enabled: true},
				{ChainKey: chain.ChainKeyEthereum, Symbol: "WBTC", Name: "Wrapped BTC", Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Decimals: 8, Enabled: true},
				{ChainKey: chain.ChainKeyAvalanche, Symbol: "WBTC", Name: "Wrapped BTC", Address: "0x0555E30da8f98308EdB960aa94C0Db47230d2B9c", Decimals: 8, Enabled: true},
				{ChainKey: chain.ChainKeyBinanceSmartChain, Symbol: "WBTC", Name: "Wrapped BTC", Address: "0x0555E30da8f98308EdB960aa94C0Db47230d2B9c", Decimals: 8, Enabled: true},
				{ChainKey: chain.ChainKey("tron"), Symbol: "WBTC", Name: "Wrapped BTC", Address: "TYhWwKpw43ENFWBTGpzLHn3882f2au7SMi", Decimals: 8, Enabled: true},
			},
		},
		{Symbol: "CHZ", Name: "Chiliz", Type: "native", Decimals: 18, IconURL: coinMarketCapIconURL("4066"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Decimals: 8, Enabled: true, Address: "6eftxVbSAunVEoxUWdGhPdxg5UdsJ8Wkwy5w5YFuxouw"},
				{ChainKey: chain.ChainKeyBase, Decimals: 18, Enabled: true, Address: "0x70c8392DE9b39a1E48d12A70Af6FF4Be25D6D0A2"},
				{ChainKey: chain.ChainKeyChiliz, Name: "Wrapped Chiliz", Symbol: "wCHZ", Decimals: 18, Enabled: true, Address: "0x677f7e16c7dd57be1d4c8ad1244883214953dc47"},
			},
		},
		{Symbol: "CHZINU", Name: "ChilizINU", Type: "token", Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0xF3928e7871eb136DD6648Ad08aEEF6B6ea893001", Decimals: 4, Enabled: true},
			},
		},
		{Symbol: "BNB", Name: "BNB", Type: "native", Decimals: 18, IconURL: coinMarketCapIconURL("1839"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyBinanceSmartChain, Native: true, Decimals: 18, Enabled: true},
				{ChainKey: chain.ChainKeyBinanceSmartChain, Symbol: "WBNB", Name: "Wrapped BNB", Address: "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", Decimals: 18, Enabled: true},
			},
		},
		{Symbol: "PEPPER", Name: "PEPPER", Type: "token", Decimals: 18, IconURL: coinMarketCapIconURL("33603"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "GozPNCAseytzxCR3d2k8hTsTYkr4SDpuXy2RQAZFVx2g", Decimals: 3, Enabled: true},
				{ChainKey: chain.ChainKeyBase, Address: "0x5e985E4BCa4664E985f3FaF8140EbA25b10E28C2", Decimals: 18, Enabled: true},
				{ChainKey: chain.ChainKeyChiliz, Address: "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67", Decimals: 18, Enabled: true},
			},
		},
		{Symbol: "LGBT", Name: "COOLVIBES", Type: "token", Decimals: 6,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "9qpnNkj8wqecEhnrKwzhNAtzSknizFqDEzxPd1Ajpump", Decimals: 6, Enabled: true},
			},
		},
		{Symbol: "SOL", Name: "Solana", Type: "native", Decimals: 9, IconURL: coinMarketCapIconURL("5426"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Name: "Wrapped Solana", Symbol: "WSOL", Mint: "So11111111111111111111111111111111111111112", Decimals: 9, Enabled: true},
			},
		},
		{Symbol: "TBT", Name: "TBT Token", Type: "token", Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0xfed7A6423cdeBb4c05552DC888de5acC657444F4", Decimals: 18, Enabled: true},
			},
		},
		{Symbol: "TRX", Name: "Tron", Type: "native", Decimals: 6, IconURL: coinMarketCapIconURL("1958"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKey("tron"), Native: true, Decimals: 6, Enabled: true},
			},
		},
		{Symbol: "ETH", Name: "Ether", Type: "native", Decimals: 18, IconURL: coinMarketCapIconURL("1027"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyEthereum, Name: "Wrapped Ether", Symbol: "WETH", Address: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Decimals: 18, Enabled: true},
				{ChainKey: chain.ChainKeyBase, Name: "Wrapped Ether", Symbol: "WETH", Address: "0x4200000000000000000000000000000000000006", Decimals: 18, Enabled: true},
			},
		},
		{Symbol: "AVAX", Name: "Avalanche", Type: "native", Decimals: 18, IconURL: coinMarketCapIconURL("5805"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyAvalanche, Name: "Wrapped AVAX", Symbol: "WAVAX", Address: "0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7", Decimals: 18, Enabled: true},
			},
		},
		{Symbol: "USDC", Name: "USD Coin", Type: "token", Decimals: 6, IconURL: coinMarketCapIconURL("3408"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyChiliz, Address: "0xa37936F56249965d407E39347528a1A91eB1cbef", Decimals: 6, Enabled: true},
				{ChainKey: chain.ChainKeyEthereum, Address: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, Enabled: true},
				{ChainKey: chain.ChainKeyBase, Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6, Enabled: true},
				{ChainKey: chain.ChainKeyAvalanche, Address: "0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E", Decimals: 6, Enabled: true},
				{ChainKey: chain.ChainKeySolana, Mint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6, Enabled: true},
			},
		},
		{Symbol: "USDT", Name: "Tether USD", Type: "token", Decimals: 6, IconURL: coinMarketCapIconURL("825"),
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeyEthereum, Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6, Enabled: true},
				{ChainKey: chain.ChainKeySolana, Mint: "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB", Decimals: 6, Enabled: true},
				{ChainKey: chain.ChainKey("tron"), Address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", Decimals: 6, Enabled: true},
			},
		},
	}

	venues := []venue.Venue{

		{
			Key:      venue.VenueKeyKewlSwap,
			Name:     "KewlSwap",
			ChainKey: chain.ChainKeyChiliz,
			Kind:     venue.VenueKindUniswapV2,
			Enabled:  true,
			Config: venue.UniswapV2Config{
				FactoryAddress:  "0xA0BB8f9865f732C277d0C162249A4F6c157ae9D0",
				RouterAddress:   envOrDefault("KEWLSWAP_ROUTER_ADDRESS", ""),
				DeploymentBlock: 0,
			},
		},
		{
			Key:      venue.VenueKeyDiviSwap,
			Name:     "DiviSwap",
			ChainKey: chain.ChainKeyChiliz,
			Kind:     venue.VenueKindUniswapV2,
			Enabled:  true,
			Config: venue.UniswapV2Config{
				FactoryAddress:  "0xbdd9c322ecf401e09c9d2dca3be46a7e45d48bb1",
				RouterAddress:   envOrDefault("DIVISWAP_ROUTER_ADDRESS", ""),
				DeploymentBlock: 0,
			},
		},
		{
			Key:      venue.VenueKeyKayenSwap,
			Name:     "KayenSwap",
			ChainKey: chain.ChainKeyChiliz,
			Kind:     venue.VenueKindUniswapV2,
			Enabled:  true,
			Config: venue.UniswapV2Config{
				FactoryAddress:  "0xE2918AA38088878546c1A18F2F9b1BC83297fdD3",
				RouterAddress:   envOrDefault("KAYENSWAP_ROUTER_ADDRESS", "0x1918EbB39492C8b98865c5E53219c3f1AE79e76F"),
				DeploymentBlock: 0,
			},
		},
		{
			Key:      venue.VenueKeyRaydium,
			Name:     "Raydium",
			ChainKey: chain.ChainKeySolana,
			Kind:     venue.VenueKindRaydium,
			Enabled:  true,
			Config: venue.RaydiumConfig{
				AMMProgramID:  "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8",
				CLMMProgramID: "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK",
				CPMMProgramID: "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C",
			},
		},
		{
			Key:      venue.VenueKeyOrca,
			Name:     "Orca",
			ChainKey: chain.ChainKeySolana,
			Kind:     venue.VenueKindOrca,
			Enabled:  true,
			Config: venue.OrcaConfig{
				WhirlpoolProgramID: "whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc",
				ConfigAccounts:     []string{"2LecshUwdy9xi7meFgHtFJQNSKk4KdTrcpvaB56dP2NQ"},
			},
		},
		{
			Key:      venue.VenueKeyMeteora,
			Name:     "Meteora DLMM",
			ChainKey: chain.ChainKeySolana,
			Kind:     venue.VenueKindMeteora,
			Enabled:  true,
			Config: venue.MeteoraConfig{
				DLMMProgramID: "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo",
			},
		},
		{

			Key:      venue.VenueKeyAerodromeClassic,
			Name:     "Aerodrome Classic",
			ChainKey: chain.ChainKeyBase,
			Kind:     venue.VenueKindAerodrome,
			Enabled:  true,
			Config: venue.AerodromeClassicConfig{
				PoolFactoryAddress: "0x420DD381b31aEf6683db6B902084cB0FFECe40Da",
				RouterAddress:      "0xcF77a3Ba9A5CA399B7c97c74d54e5b1Beb874E43",
				VoterAddress:       "0x16613524e02ad97eDfeF371bC883F2F5d6C480A5",
				DeploymentBlock:    0,
			},
		},
		{

			Key:      venue.VenueKeyAerodromeSlipstream,
			Name:     "Aerodrome Slipstream",
			ChainKey: chain.ChainKeyBase,
			Kind:     venue.VenueKindUniswapV3,
			Enabled:  true,
			Config: venue.UniswapV3Config{
				FactoryAddress:         envOrDefault("AERODROME_SLIPSTREAM_FACTORY_ADDRESS", "0x5e7BB104d84c7CB9B682AaC2F3d509f5F406809A"),
				RouterAddress:          envOrDefault("AERODROME_SLIPSTREAM_ROUTER_ADDRESS", "0xBE6D8f0d05cC4be24d5167a3eF062215bE6D18a5"),
				QuoterAddress:          envOrDefault("AERODROME_SLIPSTREAM_QUOTER_ADDRESS", "0x254cF9E1E6e233aa1AC962CB9B05b2cfeAaE15b0"),
				PositionManagerAddress: envOrDefault("AERODROME_SLIPSTREAM_POSITION_MANAGER_ADDRESS", "0x827922686190790b37229fd06084350E74485b72"),
				DeploymentBlock:        0,
			},
		},
		{
			Key:      venue.VenueKeyUniswapV1Ethereum,
			Name:     "Uniswap V1",
			ChainKey: chain.ChainKeyEthereum,
			Kind:     venue.VenueKindUniswapV1,
			Enabled:  true,
			Config: venue.UniswapV1Config{
				FactoryAddress:  envOrDefault("UNISWAP_V1_FACTORY_ADDRESS", "0xc0a47dFe034B400B47bDaD5FecDa2621de6c4d95"),
				WETHAddress:     envOrDefault("ETHEREUM_WETH_ADDRESS", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"),
				DeploymentBlock: 0,
			},
		},
		{
			Key:      venue.VenueKeyUniswapV2Ethereum,
			Name:     "Uniswap V2",
			ChainKey: chain.ChainKeyEthereum,
			Kind:     venue.VenueKindUniswapV2,
			Enabled:  true,
			Config: venue.UniswapV2Config{
				FactoryAddress:  envOrDefault("UNISWAP_V2_FACTORY_ADDRESS", "0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"),
				RouterAddress:   envOrDefault("UNISWAP_V2_ROUTER_ADDRESS", "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"),
				DeploymentBlock: 0,
			},
		},
		{
			Key:      venue.VenueKeyUniswapV3Ethereum,
			Name:     "Uniswap V3",
			ChainKey: chain.ChainKeyEthereum,
			Kind:     venue.VenueKindUniswapV3,
			Enabled:  true,
			Config: venue.UniswapV3Config{
				FactoryAddress:         envOrDefault("UNISWAP_V3_FACTORY_ADDRESS", "0x1F98431c8aD98523631AE4a59f267346ea31F984"),
				RouterAddress:          envOrDefault("UNISWAP_V3_ROUTER_ADDRESS", "0xE592427A0AEce92De3Edee1F18E0157C05861564"),
				QuoterAddress:          envOrDefault("UNISWAP_V3_QUOTER_ADDRESS", "0x61fFE014bA17989E743c5F6cB21bF9697530B21e"),
				PositionManagerAddress: envOrDefault("UNISWAP_V3_POSITION_MANAGER_ADDRESS", "0xC36442b4a4522E871399CD717aBDD847Ab11FE88"),
				DeploymentBlock:        0,
			},
		},
		{
			Key:      venue.VenueKeyPangolinAvalanche,
			Name:     "Pangolin",
			ChainKey: chain.ChainKeyAvalanche,
			Kind:     venue.VenueKindUniswapV2,
			Enabled:  true,
			Config: venue.UniswapV2Config{
				FactoryAddress:  envOrDefault("PANGOLIN_FACTORY_ADDRESS", "0xefa94de7a5529449c8a6857d5b3b61e4c03ee475"),
				RouterAddress:   envOrDefault("PANGOLIN_ROUTER_ADDRESS", "0xe54ca86531e17ef3616d22ca28b0d458b6c89106"),
				DeploymentBlock: 0,
			},
		},
		{
			Key:      venue.VenueKeyTraderJoeAvalanche,
			Name:     "Trader Joe",
			ChainKey: chain.ChainKeyAvalanche,
			Kind:     venue.VenueKindUniswapV2,
			Enabled:  true,
			Config: venue.UniswapV2Config{
				FactoryAddress:  envOrDefault("TRADERJOE_FACTORY_ADDRESS", "0x9Ad6C38BE94206cA50bb0d90783181662f0Cfa10"),
				RouterAddress:   envOrDefault("TRADERJOE_ROUTER_ADDRESS", "0x60aE616a2155Ee3d9A68541Ba4544862310933d4"),
				DeploymentBlock: 0,
			},
		},
	}

	markets := usdMarketsForAssets(assets)

	return Registries{Chains: chain.NewRegistry(chains), Assets: asset.NewRegistry(assets), Venues: venue.NewRegistry(venues), Markets: market.NewRegistry(markets)}
}

func usdMarketsForAssets(assets []asset.Asset) []market.Market {
	markets := make([]market.Market, 0, len(assets))
	for _, item := range assets {
		if item.Symbol == "" || strings.EqualFold(item.Symbol, "USD") {
			continue
		}
		markets = append(markets, market.Market{
			Symbol:     item.Symbol + "/USD",
			BaseAsset:  item.Symbol,
			QuoteAsset: "USD",
			Enabled:    true,
		})
	}
	return markets
}

func rpcURLsFromEnv(key string, fallback chain.RPCURLs) chain.RPCURLs {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return fallback
	}

	parts := strings.Split(raw, ",")
	out := make(chain.RPCURLs, 0, len(parts))
	for _, part := range parts {
		url := strings.TrimSpace(part)
		if url != "" {
			out = append(out, url)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func coinMarketCapIconURL(id string) string {
	if strings.TrimSpace(id) == "" {
		return ""
	}
	return "https://s2.coinmarketcap.com/static/img/coins/64x64/" + strings.TrimSpace(id) + ".png"
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on", "enabled":
		return true
	case "0", "false", "no", "n", "off", "disabled":
		return false
	default:
		return fallback
	}
}
