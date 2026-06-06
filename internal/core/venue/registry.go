package venue

import (
	"fmt"
	"sort"

	"exchange/internal/core/chain"
)

type VenueKey string

const (
	VenueKeyKewlSwap            VenueKey = "kewlswap"
	VenueKeyDiviSwap            VenueKey = "diviswap"
	VenueKeyKayenSwap           VenueKey = "kayenswap"
	VenueKeyAerodromeClassic    VenueKey = "aerodrome_classic_base"
	VenueKeyAerodromeSlipstream VenueKey = "aerodrome_slipstream_base"
	VenueKeyUniswapV1Ethereum   VenueKey = "uniswap_v1_ethereum"
	VenueKeyUniswapV2Ethereum   VenueKey = "uniswap_v2_ethereum"
	VenueKeyUniswapV3Ethereum   VenueKey = "uniswap_v3_ethereum"
	VenueKeyPangolinAvalanche   VenueKey = "pangolin_avalanche"
	VenueKeyTraderJoeAvalanche  VenueKey = "traderjoe_avalanche"
	VenueKeyUniswapV2           VenueKey = "uniswap_v2"
	VenueKeyUniswapV3           VenueKey = "uniswap_v3"
	VenueKeyCurve               VenueKey = "curve"
	VenueKeyBalancer            VenueKey = "balancer"
	VenueKeyRaydium             VenueKey = "raydium"
	VenueKeyOrca                VenueKey = "orca"
	VenueKeyMeteora             VenueKey = "meteora"
	VenueKeyAerodrome           VenueKey = "aerodrome"
	VenueKeyTraderJoe           VenueKey = "traderjoe"
)

type VenueKind string

const (
	VenueKindUniswapV1 VenueKind = "uniswap_v1"
	VenueKindUniswapV2 VenueKind = "uniswap_v2"
	VenueKindUniswapV3 VenueKind = "uniswap_v3"
	VenueKindUniswapV4 VenueKind = "uniswap_v4"
	VenueKindCurve     VenueKind = "curve"
	VenueKindBalancer  VenueKind = "balancer"
	VenueKindRaydium   VenueKind = "raydium"
	VenueKindOrca      VenueKind = "orca"
	VenueKindMeteora   VenueKind = "meteora"
	VenueKindAerodrome VenueKind = "aerodrome"
	VenueKindTraderJoe VenueKind = "traderjoe"
)

type VenueConfig interface {
	VenueConfigKind() VenueKind
}

type Venue struct {
	Key      VenueKey
	Name     string
	ChainKey chain.ChainKey
	Kind     VenueKind
	Enabled  bool
	Config   VenueConfig
}

func (v Venue) Validate() error {
	if v.Key == "" {
		return fmt.Errorf("venue key is empty")
	}

	if v.Name == "" {
		return fmt.Errorf("venue %s name is empty", v.Key)
	}

	if v.ChainKey == "" {
		return fmt.Errorf("venue %s chain key is empty", v.Key)
	}

	if v.Kind == "" {
		return fmt.Errorf("venue %s kind is empty", v.Key)
	}

	if v.Config == nil {
		return fmt.Errorf("venue %s config is nil", v.Key)
	}

	if v.Config.VenueConfigKind() != v.Kind {
		return fmt.Errorf(
			"venue %s kind/config mismatch: kind=%s config=%s",
			v.Key,
			v.Kind,
			v.Config.VenueConfigKind(),
		)
	}

	return nil
}

type Registry struct {
	items map[VenueKey]Venue
}

func NewRegistry(venues []Venue) Registry {
	items := make(map[VenueKey]Venue, len(venues))

	for _, v := range venues {
		items[v.Key] = v
	}

	return Registry{items: items}
}

func (r Registry) Get(key VenueKey) (Venue, bool) {
	v, ok := r.items[key]
	return v, ok
}

func (r Registry) All() []Venue {
	out := make([]Venue, 0, len(r.items))

	for _, v := range r.items {
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})

	return out
}
