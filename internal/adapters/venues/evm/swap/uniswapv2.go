package swap

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	coreswap "exchange/internal/core/swap"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const uniswapV2RouterABI = `[
  {
    "inputs": [
      {"internalType":"uint256","name":"amountIn","type":"uint256"},
      {"internalType":"uint256","name":"amountOutMin","type":"uint256"},
      {"internalType":"address[]","name":"path","type":"address[]"},
      {"internalType":"address","name":"to","type":"address"},
      {"internalType":"uint256","name":"deadline","type":"uint256"}
    ],
    "name":"swapExactTokensForTokens",
    "outputs":[{"internalType":"uint256[]","name":"amounts","type":"uint256[]"}],
    "stateMutability":"nonpayable",
    "type":"function"
  }
]`

type PoolProvider interface {
	GetPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error)
}

type UniswapV2Executor struct {
	RouterAddress string
	FeeBps        uint32
	Pools         PoolProvider
	routerABI     abi.ABI
}

func NewUniswapV2Executor(routerAddress string, feeBps uint32, pools PoolProvider) (*UniswapV2Executor, error) {
	parsed, err := abi.JSON(strings.NewReader(uniswapV2RouterABI))
	if err != nil {
		return nil, err
	}
	if feeBps == 0 {
		feeBps = 30
	}
	return &UniswapV2Executor{
		RouterAddress: routerAddress,
		FeeBps:        feeBps,
		Pools:         pools,
		routerABI:     parsed,
	}, nil
}

func (e *UniswapV2Executor) Quote(ctx context.Context, req coreswap.Request) (*coreswap.Quote, error) {
	if e.Pools == nil {
		return nil, fmt.Errorf("uniswap v2 quote requires pool provider")
	}
	if req.AmountIn == nil || req.AmountIn.Sign() <= 0 {
		return nil, fmt.Errorf("amountIn must be positive")
	}

	pool, err := e.Pools.GetPool(ctx, req.PoolID)
	if err != nil {
		return nil, err
	}

	reserveIn, reserveOut, err := reservesForDirection(*pool, req.TokenIn, req.TokenOut)
	if err != nil {
		return nil, err
	}

	amountOut := constantProductOut(req.AmountIn, reserveIn, reserveOut, e.FeeBps)
	return &coreswap.Quote{
		ChainKey:  req.ChainKey,
		VenueKey:  req.VenueKey,
		VenueKind: req.VenueKind,
		PoolID:    req.PoolID,
		TokenIn:   req.TokenIn,
		TokenOut:  req.TokenOut,
		AmountIn:  new(big.Int).Set(req.AmountIn),
		AmountOut: amountOut,
		MinOut:    coreswap.MinOut(amountOut, req.SlippageBps),
		FeeBps:    e.FeeBps,
	}, nil
}

func (e *UniswapV2Executor) BuildTransaction(_ context.Context, req coreswap.Request, quote coreswap.Quote) (*coreswap.TransactionIntent, error) {
	if e.RouterAddress == "" {
		return nil, fmt.Errorf("router address is required")
	}
	if req.Recipient == "" {
		return nil, fmt.Errorf("recipient is required")
	}
	if req.AmountIn == nil || req.AmountIn.Sign() <= 0 {
		return nil, fmt.Errorf("amountIn must be positive")
	}
	if quote.MinOut == nil {
		return nil, fmt.Errorf("quote minOut is required")
	}

	path := []common.Address{
		common.HexToAddress(req.TokenIn),
		common.HexToAddress(req.TokenOut),
	}
	data, err := e.routerABI.Pack(
		"swapExactTokensForTokens",
		req.AmountIn,
		quote.MinOut,
		path,
		common.HexToAddress(req.Recipient),
		new(big.Int).SetUint64(req.DeadlineUnix),
	)
	if err != nil {
		return nil, err
	}

	return &coreswap.TransactionIntent{
		ChainKey:  req.ChainKey,
		VenueKind: req.VenueKind,
		EVM: &coreswap.EVMTransaction{
			To:    common.HexToAddress(e.RouterAddress).Hex(),
			Data:  data,
			Value: big.NewInt(0),
		},
	}, nil
}

func reservesForDirection(pool venue.Pool, tokenIn string, tokenOut string) (*big.Int, *big.Int, error) {
	token0 := strings.ToLower(string(pool.Token0))
	token1 := strings.ToLower(string(pool.Token1))
	in := strings.ToLower(tokenIn)
	out := strings.ToLower(tokenOut)

	if in == token0 && out == token1 {
		return nonNil(pool.Reserve0), nonNil(pool.Reserve1), nil
	}
	if in == token1 && out == token0 {
		return nonNil(pool.Reserve1), nonNil(pool.Reserve0), nil
	}
	return nil, nil, fmt.Errorf("pool %s does not match token direction %s -> %s", pool.ID, tokenIn, tokenOut)
}

func constantProductOut(amountIn, reserveIn, reserveOut *big.Int, feeBps uint32) *big.Int {
	if amountIn == nil || reserveIn == nil || reserveOut == nil || amountIn.Sign() <= 0 || reserveIn.Sign() <= 0 || reserveOut.Sign() <= 0 {
		return big.NewInt(0)
	}
	if feeBps >= 10_000 {
		return big.NewInt(0)
	}

	feeDenom := big.NewInt(10_000)
	amountInWithFee := new(big.Int).Mul(amountIn, big.NewInt(int64(10_000-feeBps)))
	numerator := new(big.Int).Mul(amountInWithFee, reserveOut)
	denominator := new(big.Int).Add(new(big.Int).Mul(reserveIn, feeDenom), amountInWithFee)
	return numerator.Div(numerator, denominator)
}

func nonNil(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(v)
}
