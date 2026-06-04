package chain

type ChainKind string

const (
	KindEVM     ChainKind = "evm"
	KindSolana  ChainKind = "solana"
	KindBitcoin ChainKind = "bitcoin"
)

type ChainKey string

const (
	ChainKeyBinanceSmartChain ChainKey = "binance_smart_chain"
	ChainKeyEthereum          ChainKey = "ethereum"
	ChainKeyChiliz            ChainKey = "chiliz"
	ChainKeySolana            ChainKey = "solana"
	ChainKeyBase              ChainKey = "base"
	ChainKeyAvalanche         ChainKey = "avalanche"
	ChainKeyUnichain          ChainKey = "unichain"
	ChainKeyArbitrum          ChainKey = "arbitrum"
	ChainKeyOptimism          ChainKey = "optimism"
	ChainKeyPolygon           ChainKey = "polygon"
)

type RPCURLs []string

type Chain struct {
	Key               ChainKey
	Name              string
	Kind              ChainKind
	ChainID           *int64
	Network           string
	NativeAsset       string
	RPCURLs           RPCURLs
	ExplorerURL       string
	Confirmations     int
	Enabled           bool
	Multicall3Address string
}

type Registry struct {
	items map[string]Chain
}

func NewRegistry(chains []Chain) Registry {
	items := make(map[string]Chain, len(chains))
	for _, c := range chains {
		items[string(c.Key)] = c
	}
	return Registry{items: items}
}

func (r Registry) Get(key string) (Chain, bool) { c, ok := r.items[key]; return c, ok }
func (r Registry) All() []Chain {
	out := make([]Chain, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, c)
	}
	return out
}
