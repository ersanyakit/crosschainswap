package order

import (
	"errors"
	"strings"
	"time"
)

type ID string
type ClientOrderID string

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Type string

const (
	TypeLimit     Type = "limit"
	TypeMarket    Type = "market"
	TypeStopLimit Type = "stop_limit"
)

type Status string

const (
	StatusPendingStop     Status = "pending_stop"
	StatusOpen            Status = "open"
	StatusPartiallyFilled Status = "partially_filled"
	StatusFilled          Status = "filled"
	StatusCanceled        Status = "canceled"
	StatusExpired         Status = "expired"
	StatusRejected        Status = "rejected"
)

type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "gtc"
	TimeInForceIOC TimeInForce = "ioc"
)

type Order struct {
	ID                ID            `json:"id"`
	ClientOrderID     ClientOrderID `json:"client_order_id,omitempty"`
	UserID            string        `json:"user_id"`
	Market            string        `json:"market"`
	BaseAsset         string        `json:"base_asset"`
	QuoteAsset        string        `json:"quote_asset"`
	Side              Side          `json:"side"`
	Type              Type          `json:"type"`
	Status            Status        `json:"status"`
	TimeInForce       TimeInForce   `json:"time_in_force"`
	Price             string        `json:"price"`
	StopPrice         string        `json:"stop_price,omitempty"`
	Quantity          string        `json:"quantity"`
	FilledQuantity    string        `json:"filled_quantity"`
	RemainingQuantity string        `json:"remaining_quantity"`
	SequenceID        uint64        `json:"sequence_id"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

func NormalizeSide(value string) Side {
	return Side(strings.ToLower(strings.TrimSpace(value)))
}

func NormalizeType(value string) Type {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return TypeLimit
	}
	return Type(value)
}

func NormalizeTimeInForce(value string) TimeInForce {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return TimeInForceGTC
	}
	return TimeInForce(value)
}

func (s Side) Validate() error {
	switch s {
	case SideBuy, SideSell:
		return nil
	default:
		return errors.New("side must be buy or sell")
	}
}

func (t Type) Validate() error {
	switch t {
	case TypeLimit, TypeMarket, TypeStopLimit:
		return nil
	default:
		return errors.New("order type must be limit, market or stop_limit")
	}
}

func (t TimeInForce) Validate() error {
	switch t {
	case TimeInForceGTC, TimeInForceIOC:
		return nil
	default:
		return errors.New("time_in_force must be gtc or ioc")
	}
}
