package marketdata

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Pool struct {
	Address  common.Address
	Token0   common.Address
	Token1   common.Address
	Reserve0 *big.Int
	Reserve1 *big.Int
	VenueID  string
	ChainID  string
}

type VenueScanner interface {
	AllPairsLength(ctx context.Context) (*big.Int, error)
	PairAt(ctx context.Context, index *big.Int) (common.Address, error)
	LoadPair(ctx context.Context, pair common.Address) (*Pool, error)
	ScanAllPairs(ctx context.Context) ([]Pool, error)
}
