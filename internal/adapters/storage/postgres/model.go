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
