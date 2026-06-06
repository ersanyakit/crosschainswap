package asset

import (
	"exchange/internal/core/chain"
	"fmt"
)

type Deployment struct {
	ChainKey chain.ChainKey
	Address  string
	Name     string
	Symbol   string
	Mint     string
	Decimals int
	Enabled  bool
	IconURL  string
}

func (d Deployment) Identifier() string {
	if d.Mint != "" {
		return d.Mint
	}
	return d.Address
}

func (d Deployment) AssetID() string {
	switch d.ChainKey {
	case chain.ChainKeySolana:
		if d.Mint != "" {
			return d.Mint
		}
		return d.Address
	default:
		if d.Address != "" {
			return d.Address
		}
		return d.Mint
	}
}

func (d Deployment) Validate() error {

	switch d.ChainKey {
	case chain.ChainKeySolana:
		if d.Mint == "" {
			return fmt.Errorf("solana deployment requires mint")
		}
	default:
		if d.Address == "" {
			return fmt.Errorf("evm deployment requires address")
		}
	}
	return nil

}

type Asset struct {
	Symbol      string
	Name        string
	Type        string
	Decimals    int
	IconURL     string
	Deployments []Deployment
}

type Registry struct{ items map[string]Asset }

func NewRegistry(assets []Asset) Registry {
	items := make(map[string]Asset, len(assets))
	for _, a := range assets {
		items[a.Symbol] = a
	}
	return Registry{items: items}
}

func (r Registry) Get(symbol string) (Asset, bool) { a, ok := r.items[symbol]; return a, ok }
func (r Registry) All() []Asset {
	out := make([]Asset, 0, len(r.items))
	for _, a := range r.items {
		out = append(out, a)
	}
	return out
}
