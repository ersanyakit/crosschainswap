package onchain

import (
	"encoding/binary"
	"testing"

	"exchange/internal/core/chain"
	"exchange/internal/core/venue"
)

func TestParseMeteoraLbPairSetsSpotPrice(t *testing.T) {
	data := make([]byte, meteoraDLMMAccountSize)
	copy(data, anchorDiscriminator("LbPair"))
	activeID := int32(-1786)
	binary.LittleEndian.PutUint32(data[76:80], uint32(activeID))
	binary.LittleEndian.PutUint16(data[80:82], 80)

	pool, ok := parseMeteoraLbPair("pool", data, chain.ChainKeySolana, venue.VenueKeyMeteora, meteoraDLMMProgram)
	if !ok {
		t.Fatal("expected Meteora pair to parse")
	}
	if pool.SqrtPriceX96 == nil || pool.SqrtPriceX96.Sign() <= 0 {
		t.Fatalf("expected positive sqrt price, got %v", pool.SqrtPriceX96)
	}
	if pool.Tick != -1786 || pool.TickSpacing != 80 {
		t.Fatalf("unexpected active bin metadata: tick=%d bin_step=%d", pool.Tick, pool.TickSpacing)
	}
}
