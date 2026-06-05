package swap

import (
	"context"
	"math/big"

	"exchange/internal/core/chain"
	"exchange/internal/core/venue"
)

type Request struct {
	ChainKey    chain.ChainKey
	VenueKey    venue.VenueKey
	VenueKind   venue.VenueKind
	PoolID      venue.PoolID
	PoolAddress string

	TokenIn  string
	TokenOut string
	AmountIn *big.Int

	Sender       string
	Recipient    string
	SlippageBps  uint32
	DeadlineUnix uint64
}

type Quote struct {
	ChainKey  chain.ChainKey
	VenueKey  venue.VenueKey
	VenueKind venue.VenueKind
	PoolID    venue.PoolID

	TokenIn   string
	TokenOut  string
	AmountIn  *big.Int
	AmountOut *big.Int
	MinOut    *big.Int
	FeeBps    uint32
}

type EVMTransaction struct {
	To    string
	Data  []byte
	Value *big.Int
}

type SolanaInstruction struct {
	ProgramID string
	Accounts  []string
	Data      []byte
}

type TransactionIntent struct {
	ChainKey  chain.ChainKey
	VenueKind venue.VenueKind

	EVM    *EVMTransaction
	Solana []SolanaInstruction
}

type Executor interface {
	Quote(ctx context.Context, req Request) (*Quote, error)
	BuildTransaction(ctx context.Context, req Request, quote Quote) (*TransactionIntent, error)
}

func MinOut(amountOut *big.Int, slippageBps uint32) *big.Int {
	if amountOut == nil {
		return big.NewInt(0)
	}
	if slippageBps > 10_000 {
		slippageBps = 10_000
	}

	multiplier := big.NewInt(int64(10_000 - slippageBps))
	out := new(big.Int).Mul(amountOut, multiplier)
	return out.Div(out, big.NewInt(10_000))
}
