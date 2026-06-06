package pricing

const UpdatesChannel = "price_updates"

type UpdateEvent struct {
	Type string       `json:"type"`
	Data *AssetPrices `json:"data"`
}

func NewUpdateEvent(prices *AssetPrices) UpdateEvent {
	return UpdateEvent{
		Type: "prices.updated",
		Data: prices,
	}
}
