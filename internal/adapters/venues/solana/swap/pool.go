package swap

import (
	"context"
	"fmt"

	coreswap "exchange/internal/core/swap"
)

type InstructionBuilder interface {
	Quote(ctx context.Context, req coreswap.Request) (*coreswap.Quote, error)
	BuildInstructions(ctx context.Context, req coreswap.Request, quote coreswap.Quote) ([]coreswap.SolanaInstruction, error)
}

type PoolExecutor struct {
	Builder InstructionBuilder
}

func NewPoolExecutor(builder InstructionBuilder) *PoolExecutor {
	return &PoolExecutor{Builder: builder}
}

func (e *PoolExecutor) Quote(ctx context.Context, req coreswap.Request) (*coreswap.Quote, error) {
	if e.Builder == nil {
		return nil, fmt.Errorf("solana swap requires program-specific instruction builder")
	}
	return e.Builder.Quote(ctx, req)
}

func (e *PoolExecutor) BuildTransaction(ctx context.Context, req coreswap.Request, quote coreswap.Quote) (*coreswap.TransactionIntent, error) {
	if e.Builder == nil {
		return nil, fmt.Errorf("solana swap requires program-specific instruction builder")
	}
	instructions, err := e.Builder.BuildInstructions(ctx, req, quote)
	if err != nil {
		return nil, err
	}
	return &coreswap.TransactionIntent{
		ChainKey:  req.ChainKey,
		VenueKind: req.VenueKind,
		Solana:    instructions,
	}, nil
}
