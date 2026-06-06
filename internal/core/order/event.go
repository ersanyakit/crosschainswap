package order

import "time"

type EventID string

type EventType string

const (
	EventOrderAccepted EventType = "order_accepted"
	EventOrderCanceled EventType = "order_canceled"
	EventOrderFilled   EventType = "order_filled"
	EventOrderExpired  EventType = "order_expired"
	EventTradeCreated  EventType = "trade_created"
)

type Event struct {
	ID        EventID   `json:"id"`
	OrderID   ID        `json:"order_id"`
	UserID    string    `json:"user_id"`
	Market    string    `json:"market"`
	Type      EventType `json:"type"`
	RefID     string    `json:"ref_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
