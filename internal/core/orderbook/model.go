package orderbook

import (
	"time"

	"exchange/internal/core/order"
)

type PriceLevel struct {
	Market        string     `json:"market"`
	Side          order.Side `json:"side"`
	Price         string     `json:"price"`
	Quantity      string     `json:"quantity"`
	OrderCount    int64      `json:"order_count"`
	LastUpdatedAt time.Time  `json:"last_updated_at"`
}

type Snapshot struct {
	Market string       `json:"market"`
	Bids   []PriceLevel `json:"bids"`
	Asks   []PriceLevel `json:"asks"`
}
