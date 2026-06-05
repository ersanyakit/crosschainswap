package swap

import (
	"math/big"
	"testing"
)

func TestConstantProductOut(t *testing.T) {
	out := constantProductOut(
		big.NewInt(1_000),
		big.NewInt(10_000),
		big.NewInt(20_000),
		30,
	)
	if out.String() != "1813" {
		t.Fatalf("unexpected amount out: %s", out)
	}
}

func TestConstantProductOutRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		amountIn   *big.Int
		reserveIn  *big.Int
		reserveOut *big.Int
		feeBps     uint32
	}{
		{name: "nil amount", amountIn: nil, reserveIn: big.NewInt(1), reserveOut: big.NewInt(1), feeBps: 30},
		{name: "zero reserve", amountIn: big.NewInt(1), reserveIn: big.NewInt(0), reserveOut: big.NewInt(1), feeBps: 30},
		{name: "fee too high", amountIn: big.NewInt(1), reserveIn: big.NewInt(1), reserveOut: big.NewInt(1), feeBps: 10_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := constantProductOut(tt.amountIn, tt.reserveIn, tt.reserveOut, tt.feeBps)
			if out.Sign() != 0 {
				t.Fatalf("expected zero amount out, got %s", out)
			}
		})
	}
}
