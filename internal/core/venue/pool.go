package venue

import (
	"math/big"

	"exchange/internal/core/chain"
)

type PoolID string
type AssetID string

type PoolKind string

const (
	PoolKindV2   PoolKind = "v2"
	PoolKindV3   PoolKind = "v3"
	PoolKindV4   PoolKind = "v4"
	PoolKindCLMM PoolKind = "clmm"
)

type Pool struct {
	ID       PoolID
	ChainKey chain.ChainKey
	VenueKey VenueKey
	Kind     PoolKind

	Token0 AssetID
	Token1 AssetID

	Reserve0 *big.Int
	Reserve1 *big.Int

	SqrtPriceX96 *big.Int
	Liquidity    *big.Int
	Tick         int64
	Fee          uint32
	TickSpacing  int32

	ProgramID string
	Vault0    string
	Vault1    string

	Enabled bool
}
