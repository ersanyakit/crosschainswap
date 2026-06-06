package pricing

const UpdatesChannel = "price_updates"

type UpdateEvent struct {
	Type string       `json:"type"`
	Data *AssetPrices `json:"data"`
}

type SymbolUpdateEvent struct {
	Type string           `json:"type"`
	Data SymbolUpdateData `json:"data"`
}

type SymbolUpdateData struct {
	Symbol string `json:"symbol"`
}

func NewUpdateEvent(prices *AssetPrices) UpdateEvent {
	return UpdateEvent{
		Type: "prices.updated",
		Data: prices,
	}
}

func NewSymbolUpdateEvent(symbol string) SymbolUpdateEvent {
	return SymbolUpdateEvent{
		Type: "prices.updated",
		Data: SymbolUpdateData{Symbol: symbol},
	}
}
