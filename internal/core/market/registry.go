package market

type Market struct {
	Symbol     string
	BaseAsset  string
	QuoteAsset string
	ChainKeys  []string
	Enabled    bool
}

type Registry struct{ items map[string]Market }

func NewRegistry(markets []Market) Registry {
	items := make(map[string]Market, len(markets))
	for _, m := range markets {
		items[m.Symbol] = m
	}
	return Registry{items: items}
}

func (r Registry) Get(symbol string) (Market, bool) { m, ok := r.items[symbol]; return m, ok }
func (r Registry) All() []Market {
	out := make([]Market, 0, len(r.items))
	for _, m := range r.items {
		out = append(out, m)
	}
	return out
}
