package postgres

import (
	"time"
)

type Pool struct {
	ID           string `gorm:"primaryKey;type:varchar(255)"`
	PoolAddress  string `gorm:"index;type:varchar(255)"`
	ChainKey     string `gorm:"index;type:varchar(255);not null"`
	VenueKey     string `gorm:"index;type:varchar(255);not null"`
	Kind         string `gorm:"type:varchar(50);not null"`
	Token0       string `gorm:"index;type:varchar(255);not null"`
	Token1       string `gorm:"index;type:varchar(255);not null"`
	Reserve0     string `gorm:"type:numeric(78,0);not null"`
	Reserve1     string `gorm:"type:numeric(78,0);not null"`
	SqrtPriceX96 string `gorm:"type:numeric(78,0)"`
	Liquidity    string `gorm:"type:numeric(78,0)"`
	Tick         int64
	Fee          uint32
	TickSpacing  int32
	ProgramID    string    `gorm:"index;type:varchar(255)"`
	Vault0       string    `gorm:"index;type:varchar(255)"`
	Vault1       string    `gorm:"index;type:varchar(255)"`
	Enabled      bool      `gorm:"default:true;not null"`
	CreatedAt    time.Time `gorm:"default:current_timestamp"`
	UpdatedAt    time.Time `gorm:"default:current_timestamp"`
}

type ExchangeOrder struct {
	ID                string    `gorm:"primaryKey;type:varchar(64)"`
	ClientOrderID     string    `gorm:"index;index:idx_orders_client_idempotency,priority:2,unique;type:varchar(128);not null"`
	ReservationID     string    `gorm:"index;type:varchar(64)"`
	UserID            string    `gorm:"index;index:idx_orders_client_idempotency,priority:1,unique;type:varchar(128);not null"`
	Market            string    `gorm:"index;type:varchar(64);not null"`
	BaseAsset         string    `gorm:"type:varchar(32);not null"`
	QuoteAsset        string    `gorm:"type:varchar(32);not null"`
	Side              string    `gorm:"type:varchar(16);not null"`
	Type              string    `gorm:"type:varchar(32);not null"`
	Status            string    `gorm:"index;type:varchar(32);not null"`
	TimeInForce       string    `gorm:"type:varchar(16);not null"`
	Price             string    `gorm:"type:numeric(78,18);not null"`
	StopPrice         string    `gorm:"type:numeric(78,18);not null;default:0"`
	Quantity          string    `gorm:"type:numeric(78,18);not null"`
	FilledQuantity    string    `gorm:"type:numeric(78,18);not null;default:0"`
	RemainingQuantity string    `gorm:"type:numeric(78,18);not null"`
	SequenceID        uint64    `gorm:"not null;default:0"`
	CreatedAt         time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt         time.Time `gorm:"default:current_timestamp"`
}

type ExchangeOrderSequence struct {
	Market       string    `gorm:"primaryKey;type:varchar(64)"`
	NextSequence uint64    `gorm:"not null;default:1"`
	UpdatedAt    time.Time `gorm:"default:current_timestamp"`
}

type ExchangeActiveOrder struct {
	ID                string    `gorm:"primaryKey;type:varchar(64)"`
	ClientOrderID     string    `gorm:"index;type:varchar(128);not null"`
	ReservationID     string    `gorm:"index;type:varchar(64)"`
	UserID            string    `gorm:"index;type:varchar(128);not null"`
	Market            string    `gorm:"index;index:idx_active_orders_book,priority:1;type:varchar(64);not null"`
	BaseAsset         string    `gorm:"type:varchar(32);not null"`
	QuoteAsset        string    `gorm:"type:varchar(32);not null"`
	Side              string    `gorm:"index:idx_active_orders_book,priority:2;type:varchar(16);not null"`
	Type              string    `gorm:"type:varchar(32);not null"`
	Status            string    `gorm:"index;type:varchar(32);not null"`
	TimeInForce       string    `gorm:"type:varchar(16);not null"`
	Price             string    `gorm:"index:idx_active_orders_book,priority:3;type:numeric(78,18);not null"`
	StopPrice         string    `gorm:"type:numeric(78,18);not null;default:0"`
	Quantity          string    `gorm:"type:numeric(78,18);not null"`
	FilledQuantity    string    `gorm:"type:numeric(78,18);not null;default:0"`
	RemainingQuantity string    `gorm:"type:numeric(78,18);not null"`
	SequenceID        uint64    `gorm:"index:idx_active_orders_book,priority:4;not null;default:0"`
	CreatedAt         time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt         time.Time `gorm:"default:current_timestamp"`
}

type ExchangeOrderCommandSequence struct {
	Market       string    `gorm:"primaryKey;type:varchar(64)"`
	NextSequence uint64    `gorm:"not null;default:1"`
	UpdatedAt    time.Time `gorm:"default:current_timestamp"`
}

type ExchangeTrade struct {
	ID            string    `gorm:"primaryKey;type:varchar(64)"`
	Market        string    `gorm:"index;type:varchar(64);not null"`
	BaseAsset     string    `gorm:"type:varchar(32);not null"`
	QuoteAsset    string    `gorm:"type:varchar(32);not null"`
	MakerOrderID  string    `gorm:"index;type:varchar(64);not null"`
	TakerOrderID  string    `gorm:"index;type:varchar(64);not null"`
	MakerUserID   string    `gorm:"index;type:varchar(128);not null"`
	TakerUserID   string    `gorm:"index;type:varchar(128);not null"`
	TakerSide     string    `gorm:"type:varchar(16);not null"`
	Price         string    `gorm:"type:numeric(78,18);not null"`
	Quantity      string    `gorm:"type:numeric(78,18);not null"`
	QuoteQuantity string    `gorm:"type:numeric(78,18);not null"`
	CreatedAt     time.Time `gorm:"index;default:current_timestamp"`
}

type ExchangeCandle struct {
	Market      string    `gorm:"primaryKey;type:varchar(64)"`
	Interval    string    `gorm:"primaryKey;type:varchar(16)"`
	OpenTime    time.Time `gorm:"primaryKey;index:idx_candles_lookup,priority:3"`
	CloseTime   time.Time `gorm:"index"`
	Open        string    `gorm:"type:numeric(78,18);not null"`
	High        string    `gorm:"type:numeric(78,18);not null"`
	Low         string    `gorm:"type:numeric(78,18);not null"`
	Close       string    `gorm:"type:numeric(78,18);not null"`
	VolumeBase  string    `gorm:"type:numeric(78,18);not null"`
	VolumeQuote string    `gorm:"type:numeric(78,18);not null"`
	TradeCount  int64     `gorm:"not null;default:0"`
	LastTradeAt time.Time `gorm:"index"`
}

type ExchangeOrderEvent struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	OrderID   string    `gorm:"index;type:varchar(64);not null"`
	UserID    string    `gorm:"index;type:varchar(128);not null"`
	Market    string    `gorm:"index;type:varchar(64);not null"`
	Type      string    `gorm:"index;type:varchar(64);not null"`
	RefID     string    `gorm:"index;type:varchar(64)"`
	CreatedAt time.Time `gorm:"index;default:current_timestamp"`
}

type ExchangeWallet struct {
	UserID    string    `gorm:"primaryKey;type:varchar(128)"`
	ChainKey  string    `gorm:"primaryKey;type:varchar(64)"`
	Address   string    `gorm:"index;type:varchar(255);not null"`
	CreatedAt time.Time `gorm:"default:current_timestamp"`
	UpdatedAt time.Time `gorm:"default:current_timestamp"`
}

type ExchangeBalance struct {
	UserID    string    `gorm:"primaryKey;type:varchar(128)"`
	Asset     string    `gorm:"primaryKey;type:varchar(64)"`
	Available string    `gorm:"type:numeric(78,18);not null;default:0"`
	Locked    string    `gorm:"type:numeric(78,18);not null;default:0"`
	Pending   string    `gorm:"type:numeric(78,18);not null;default:0"`
	UpdatedAt time.Time `gorm:"default:current_timestamp"`
}

type ExchangeBalanceEvent struct {
	ID            string    `gorm:"primaryKey;type:varchar(64)"`
	UserID        string    `gorm:"index;type:varchar(128);not null"`
	Asset         string    `gorm:"index;type:varchar(64);not null"`
	Type          string    `gorm:"index;type:varchar(64);not null"`
	Amount        string    `gorm:"type:numeric(78,18);not null"`
	OrderID       string    `gorm:"index;type:varchar(64)"`
	ReservationID string    `gorm:"index;type:varchar(64)"`
	TradeID       string    `gorm:"index;type:varchar(64)"`
	WithdrawalID  string    `gorm:"index;type:varchar(64)"`
	CreatedAt     time.Time `gorm:"index;default:current_timestamp"`
}

type ExchangeReservation struct {
	ID              string    `gorm:"primaryKey;type:varchar(64)"`
	UserID          string    `gorm:"index;type:varchar(128);not null"`
	Market          string    `gorm:"index;type:varchar(64);not null;default:''"`
	Asset           string    `gorm:"index;type:varchar(64);not null"`
	Amount          string    `gorm:"type:numeric(78,18);not null"`
	ConsumedAmount  string    `gorm:"type:numeric(78,18);not null;default:0"`
	ReleasedAmount  string    `gorm:"type:numeric(78,18);not null;default:0"`
	RemainingAmount string    `gorm:"type:numeric(78,18);not null"`
	Status          string    `gorm:"index;type:varchar(32);not null"`
	OrderID         string    `gorm:"uniqueIndex;type:varchar(64);not null"`
	CommandID       string    `gorm:"index;type:varchar(128)"`
	CreatedAt       time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt       time.Time `gorm:"default:current_timestamp"`
}

type ExchangeWithdrawal struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	UserID    string    `gorm:"index;type:varchar(128);not null"`
	Asset     string    `gorm:"index;type:varchar(64);not null"`
	Amount    string    `gorm:"type:numeric(78,18);not null"`
	ChainKey  string    `gorm:"index;type:varchar(64);not null"`
	Address   string    `gorm:"index;type:varchar(255);not null"`
	Status    string    `gorm:"index;type:varchar(32);not null"`
	CreatedAt time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt time.Time `gorm:"default:current_timestamp"`
}

type ExchangePriceLevel struct {
	Market          string    `gorm:"primaryKey;type:varchar(64)"`
	Side            string    `gorm:"primaryKey;type:varchar(16)"`
	Price           string    `gorm:"primaryKey;type:numeric(78,18)"`
	Quantity        string    `gorm:"type:numeric(78,18);not null"`
	OrderCount      int64     `gorm:"not null;default:0"`
	FirstSequenceID uint64    `gorm:"index;not null;default:0"`
	LastUpdatedAt   time.Time `gorm:"default:current_timestamp"`
}

type ExchangeMarket struct {
	Symbol     string    `gorm:"primaryKey;type:varchar(64)"`
	BaseAsset  string    `gorm:"index;type:varchar(64);not null"`
	QuoteAsset string    `gorm:"index;type:varchar(64);not null"`
	ChainKeys  string    `gorm:"type:text;not null;default:''"`
	Enabled    bool      `gorm:"index;default:true;not null"`
	CreatedAt  time.Time `gorm:"default:current_timestamp"`
	UpdatedAt  time.Time `gorm:"default:current_timestamp"`
}

type ExchangeMatchJob struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)"`
	OrderID     string    `gorm:"uniqueIndex;type:varchar(64);not null"`
	Market      string    `gorm:"index;type:varchar(64);not null"`
	Status      string    `gorm:"index;type:varchar(32);not null"`
	Attempts    int       `gorm:"not null;default:0"`
	AvailableAt time.Time `gorm:"index;default:current_timestamp"`
	LockedBy    string    `gorm:"index;type:varchar(128)"`
	LockedAt    time.Time `gorm:"index"`
	LastError   string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt   time.Time `gorm:"default:current_timestamp"`
}

type ExchangeOrderCommand struct {
	ID            string    `gorm:"primaryKey;type:varchar(128)"`
	ClientOrderID string    `gorm:"uniqueIndex:idx_order_commands_client,priority:2;type:varchar(128);not null"`
	UserID        string    `gorm:"uniqueIndex:idx_order_commands_client,priority:1;index;type:varchar(128);not null"`
	Market        string    `gorm:"index;type:varchar(64);not null"`
	Type          string    `gorm:"index;type:varchar(32);not null"`
	PayloadHash   string    `gorm:"type:varchar(64);not null"`
	Payload       string    `gorm:"type:jsonb;not null"`
	Status        string    `gorm:"index;type:varchar(32);not null"`
	Outcome       string    `gorm:"type:varchar(64)"`
	OrderID       string    `gorm:"index;type:varchar(64)"`
	Attempts      int       `gorm:"not null;default:0"`
	LastError     string    `gorm:"type:text"`
	QuarantinedAt time.Time `gorm:"index"`
	CreatedAt     time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt     time.Time `gorm:"default:current_timestamp"`
}

type ExchangeOrderCommandLog struct {
	ID             string    `gorm:"primaryKey;type:varchar(64)"`
	CommandID      string    `gorm:"uniqueIndex;type:varchar(128);not null"`
	SequenceID     uint64    `gorm:"index:idx_order_command_log_market_sequence,priority:2;not null"`
	Market         string    `gorm:"index:idx_order_command_log_market_sequence,priority:1;index;type:varchar(64);not null"`
	Type           string    `gorm:"index;type:varchar(32);not null"`
	Key            string    `gorm:"index;type:varchar(255)"`
	PayloadHash    string    `gorm:"type:varchar(64);not null"`
	Payload        string    `gorm:"type:jsonb;not null"`
	Status         string    `gorm:"index;type:varchar(32);not null"`
	Attempts       int       `gorm:"not null;default:0"`
	AvailableAt    time.Time `gorm:"index;default:current_timestamp"`
	LockedBy       string    `gorm:"index;type:varchar(128)"`
	LockedAt       time.Time `gorm:"index"`
	LastError      string    `gorm:"type:text"`
	AppliedOrderID string    `gorm:"index;type:varchar(64)"`
	AppliedAt      time.Time `gorm:"index"`
	QuarantinedAt  time.Time `gorm:"index"`
	CreatedAt      time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt      time.Time `gorm:"default:current_timestamp"`
}

type ExchangeMatcherSnapshot struct {
	ID              string    `gorm:"primaryKey;type:varchar(64)"`
	Market          string    `gorm:"index:idx_matcher_snapshots_market_sequence,priority:1;type:varchar(64);not null"`
	SequenceID      uint64    `gorm:"index:idx_matcher_snapshots_market_sequence,priority:2;not null"`
	SchemaVersion   int       `gorm:"not null"`
	Payload         string    `gorm:"type:jsonb;not null"`
	Checksum        string    `gorm:"index;type:varchar(64);not null"`
	CreatedAt       time.Time `gorm:"index;default:current_timestamp"`
	LastValidatedAt time.Time `gorm:"index"`
}

type ExchangeMatchEventLog struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)"`
	Market      string    `gorm:"uniqueIndex:idx_match_event_market_sequence_type,priority:1;index;type:varchar(64);not null"`
	SequenceID  uint64    `gorm:"uniqueIndex:idx_match_event_market_sequence_type,priority:2;index;not null"`
	Type        string    `gorm:"uniqueIndex:idx_match_event_market_sequence_type,priority:3;index;type:varchar(64);not null"`
	PayloadHash string    `gorm:"type:varchar(64);not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	CreatedAt   time.Time `gorm:"index;default:current_timestamp"`
}

type ExchangeProjectionOffset struct {
	Projection     string    `gorm:"primaryKey;type:varchar(128)"`
	Market         string    `gorm:"primaryKey;type:varchar(64)"`
	Stream         string    `gorm:"type:varchar(128);not null"`
	LastSequence   uint64    `gorm:"not null;default:0"`
	LastEventID    string    `gorm:"type:varchar(128)"`
	LastEventHash  string    `gorm:"type:varchar(128)"`
	LastEventAt    time.Time `gorm:"index"`
	UpdatedAt      time.Time `gorm:"default:current_timestamp"`
	LastAdvancedAt time.Time `gorm:"index"`
}

type ExchangeOutboxEvent struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)"`
	Topic       string    `gorm:"index;type:varchar(128);not null"`
	EventKey    string    `gorm:"index;type:varchar(255)"`
	Payload     string    `gorm:"type:jsonb;not null"`
	Status      string    `gorm:"index;type:varchar(32);not null"`
	Attempts    int       `gorm:"not null;default:0"`
	AvailableAt time.Time `gorm:"index;default:current_timestamp"`
	LockedBy    string    `gorm:"index;type:varchar(128)"`
	LockedAt    time.Time `gorm:"index"`
	LastError   string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"index;default:current_timestamp"`
	UpdatedAt   time.Time `gorm:"default:current_timestamp"`
	PublishedAt time.Time `gorm:"index"`
}

type ServiceLease struct {
	Name      string    `gorm:"primaryKey;type:varchar(255)"`
	Owner     string    `gorm:"index;type:varchar(128);not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	UpdatedAt time.Time `gorm:"default:current_timestamp"`
}
