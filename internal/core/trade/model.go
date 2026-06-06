package trade

import (
	"time"

	"exchange/internal/core/order"
)

type ID string

type Trade struct {
	ID            ID         `json:"id"`
	Market        string     `json:"market"`
	BaseAsset     string     `json:"base_asset"`
	QuoteAsset    string     `json:"quote_asset"`
	MakerOrderID  order.ID   `json:"maker_order_id"`
	TakerOrderID  order.ID   `json:"taker_order_id"`
	MakerUserID   string     `json:"maker_user_id"`
	TakerUserID   string     `json:"taker_user_id"`
	TakerSide     order.Side `json:"taker_side"`
	Price         string     `json:"price"`
	Quantity      string     `json:"quantity"`
	QuoteQuantity string     `json:"quote_quantity"`
	CreatedAt     time.Time  `json:"created_at"`
}
