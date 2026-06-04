package config

import (
	"exchange/internal/core/asset"
	"exchange/internal/core/chain"
	"exchange/internal/core/market"
	"exchange/internal/core/venue"
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
			RPCURLs: chain.RPCURLs{
				"https://rpc.chiliz.com",
				"https://chiliz.publicnode.com",
				"https://rpc.ankr.com/chiliz",
			},
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
			RPCURLs: chain.RPCURLs{
				"https://api.mainnet-beta.solana.com",
			},
			ExplorerURL:   "https://solscan.io",
			Confirmations: 32,
			Enabled:       true,
		},
		{
			Key:         chain.ChainKeyBase,
			Name:        "Base",
			Kind:        chain.KindEVM,
			ChainID:     ptr(8453),
			Network:     "mainnet",
			NativeAsset: "ETH",
			RPCURLs: chain.RPCURLs{
				"https://base-rpc.publicnode.com",
				"https://mainnet.base.org",
				"https://1rpc.io/base",
				"https://base.drpc.org",
			},
			ExplorerURL:       "https://basescan.org",
			Confirmations:     3,
			Enabled:           true,
			Multicall3Address: "0xcA11bde05977b3631167028862bE2a173976CA11",
		},
	}

	assets := []asset.Asset{
		{Symbol: "CHZ", Name: "Chiliz", Type: "native", Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Decimals: 8, Enabled: true, Address: "6eftxVbSAunVEoxUWdGhPdxg5UdsJ8Wkwy5w5YFuxouw"},
				{ChainKey: chain.ChainKeyBase, Decimals: 18, Enabled: true, Address: "0x70c8392DE9b39a1E48d12A70Af6FF4Be25D6D0A2"},
				{ChainKey: chain.ChainKeyChiliz, Name: "Wrapped Chiliz", Symbol: "wCHZ", Decimals: 18, Enabled: true, Address: "0x677f7e16c7dd57be1d4c8ad1244883214953dc47"},
			},
		},
		{Symbol: "PEPPER", Name: "PEPPER", Type: "token", Decimals: 18,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Mint: "GozPNCAseytzxCR3d2k8hTsTYkr4SDpuXy2RQAZFVx2g", Decimals: 3},
				{ChainKey: chain.ChainKeyBase, Address: "0x5e985E4BCa4664E985f3FaF8140EbA25b10E28C2", Decimals: 18},
				{ChainKey: chain.ChainKeyChiliz, Address: "0x60f397acbcfb8f4e3234c659a3e10867e6fa6b67", Decimals: 18},
			},
		},
		{Symbol: "SOL", Name: "Solana", Type: "native", Decimals: 9,
			Deployments: []asset.Deployment{
				{ChainKey: chain.ChainKeySolana, Decimals: 9, Enabled: true},
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
				RouterAddress:   "0xA0BB8f9865f732C277d0C162249A4F6c157ae9D0",
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
				RouterAddress:   "0xbdd9c322ecf401e09c9d2dca3be46a7e45d48bb1",
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
				RouterAddress:   "0xE2918AA38088878546c1A18F2F9b1BC83297fdD3",
				DeploymentBlock: 0,
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
			Enabled:  false,
			Config: venue.UniswapV3Config{
				FactoryAddress:         "0x5e7BB104d84c7CB9B682AaC2F3d509f5F406809A",
				RouterAddress:          "0xBE6D8f0d05cC4be24d5167a3eF062215bE6D18a5",
				QuoterAddress:          "0x254cF9E1E6e233aa1AC962CB9B05b2cfeAaE15b0",
				PositionManagerAddress: "0x827922686190790b37229fd06084350E74485b72",
				DeploymentBlock:        0,
			},
		},
	}

	markets := []market.Market{
		{Symbol: "PEPPER/USDC", BaseAsset: "PEPPER", QuoteAsset: "USDC", ChainKeys: []string{string(chain.ChainKeyChiliz), string(chain.ChainKeyBase), string(chain.ChainKeySolana), string(chain.ChainKeyUnichain)}, Enabled: true},
		{Symbol: "CHZ/USDC", BaseAsset: "CHZ", QuoteAsset: "USDC", ChainKeys: []string{string(chain.ChainKeyChiliz)}, Enabled: true},
		{Symbol: "SOL/USDC", BaseAsset: "SOL", QuoteAsset: "USDC", ChainKeys: []string{string(chain.ChainKeySolana)}, Enabled: true},
		{Symbol: "AVAX/USDC", BaseAsset: "AVAX", QuoteAsset: "USDC", ChainKeys: []string{string(chain.ChainKeyAvalanche)}, Enabled: true},
		{Symbol: "ETH/USDC", BaseAsset: "ETH", QuoteAsset: "USDC", ChainKeys: []string{string(chain.ChainKeyBase), string(chain.ChainKeyUnichain)}, Enabled: true},
	}

	return Registries{Chains: chain.NewRegistry(chains), Assets: asset.NewRegistry(assets), Venues: venue.NewRegistry(venues), Markets: market.NewRegistry(markets)}
}
