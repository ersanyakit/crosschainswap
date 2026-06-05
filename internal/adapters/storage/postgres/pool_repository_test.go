package postgres

import (
	"math/big"
	"testing"

	"exchange/internal/core/chain"
	"exchange/internal/core/venue"
)

func TestPoolToVenuePoolPreservesOnChainFields(t *testing.T) {
	dbPool := Pool{
		ID:           "pool-id",
		PoolAddress:  "pool-address",
		ChainKey:     string(chain.ChainKeySolana),
		VenueKey:     string(venue.VenueKeyOrca),
		Kind:         string(venue.PoolKindCLMM),
		Token0:       "token-a",
		Token1:       "token-b",
		Reserve0:     "123",
		Reserve1:     "456",
		SqrtPriceX96: "789",
		Liquidity:    "987",
		Tick:         -42,
		Fee:          64,
		TickSpacing:  8,
		ProgramID:    "program",
		Vault0:       "vault-a",
		Vault1:       "vault-b",
		Enabled:      true,
	}

	pool, err := dbPool.toVenuePool()
	if err != nil {
		t.Fatal(err)
	}

	if pool.ID != venue.PoolID(dbPool.ID) || pool.Address != dbPool.PoolAddress {
		t.Fatalf("unexpected pool identity: %#v", pool)
	}
	if pool.ChainKey != chain.ChainKeySolana || pool.VenueKey != venue.VenueKeyOrca || pool.Kind != venue.PoolKindCLMM {
		t.Fatalf("unexpected pool classification: %#v", pool)
	}
	if pool.Reserve0.Cmp(big.NewInt(123)) != 0 || pool.Reserve1.Cmp(big.NewInt(456)) != 0 {
		t.Fatalf("unexpected reserves: %s %s", pool.Reserve0, pool.Reserve1)
	}
	if pool.SqrtPriceX96.Cmp(big.NewInt(789)) != 0 || pool.Liquidity.Cmp(big.NewInt(987)) != 0 {
		t.Fatalf("unexpected clmm values: %s %s", pool.SqrtPriceX96, pool.Liquidity)
	}
	if pool.Tick != -42 || pool.Fee != 64 || pool.TickSpacing != 8 {
		t.Fatalf("unexpected tick/fee fields: tick=%d fee=%d spacing=%d", pool.Tick, pool.Fee, pool.TickSpacing)
	}
	if pool.ProgramID != "program" || pool.Vault0 != "vault-a" || pool.Vault1 != "vault-b" {
		t.Fatalf("unexpected solana fields: %#v", pool)
	}
}
