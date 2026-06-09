package balance

import (
	"errors"
	"time"
)

var ErrInsufficientBalance = errors.New("insufficient available balance")

type Wallet struct {
	UserID    string    `json:"user_id"`
	ChainKey  string    `json:"chain_key"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Balance struct {
	UserID    string    `json:"user_id"`
	Asset     string    `json:"asset"`
	Available string    `json:"available"`
	Locked    string    `json:"locked"`
	Pending   string    `json:"pending"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WithdrawalID string

type WithdrawalStatus string

const (
	WithdrawalRequested WithdrawalStatus = "requested"
	WithdrawalCompleted WithdrawalStatus = "completed"
	WithdrawalCanceled  WithdrawalStatus = "canceled"
)

type Withdrawal struct {
	ID        WithdrawalID     `json:"id"`
	UserID    string           `json:"user_id"`
	Asset     string           `json:"asset"`
	Amount    string           `json:"amount"`
	ChainKey  string           `json:"chain_key"`
	Address   string           `json:"address"`
	Status    WithdrawalStatus `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type EventID string

type EventType string

const (
	EventDepositPending      EventType = "deposit_pending"
	EventDepositSettled      EventType = "deposit_settled"
	EventWithdrawalRequested EventType = "withdrawal_requested"
	EventWithdrawalCompleted EventType = "withdrawal_completed"
	EventWithdrawalCanceled  EventType = "withdrawal_canceled"
	EventReserve             EventType = "reserve"
	EventRelease             EventType = "release"
	EventDebitLocked         EventType = "debit_locked"
	EventSettlementReceive   EventType = "settlement_receive"
)

type Event struct {
	ID            EventID   `json:"id"`
	UserID        string    `json:"user_id"`
	Asset         string    `json:"asset"`
	Type          EventType `json:"type"`
	Amount        string    `json:"amount"`
	OrderID       string    `json:"order_id,omitempty"`
	ReservationID string    `json:"reservation_id,omitempty"`
	TradeID       string    `json:"trade_id,omitempty"`
	WithdrawalID  string    `json:"withdrawal_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
