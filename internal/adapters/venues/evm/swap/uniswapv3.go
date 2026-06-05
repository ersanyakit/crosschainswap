package swap

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	coreswap "exchange/internal/core/swap"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const uniswapV3RouterABI = `[
  {
    "inputs": [
      {
        "components": [
          {"internalType":"address","name":"tokenIn","type":"address"},
          {"internalType":"address","name":"tokenOut","type":"address"},
          {"internalType":"uint24","name":"fee","type":"uint24"},
          {"internalType":"address","name":"recipient","type":"address"},
          {"internalType":"uint256","name":"deadline","type":"uint256"},
          {"internalType":"uint256","name":"amountIn","type":"uint256"},
          {"internalType":"uint256","name":"amountOutMinimum","type":"uint256"},
          {"internalType":"uint160","name":"sqrtPriceLimitX96","type":"uint160"}
        ],
        "internalType":"struct ISwapRouter.ExactInputSingleParams",
        "name":"params",
        "type":"tuple"
      }
    ],
    "name":"exactInputSingle",
    "outputs":[{"internalType":"uint256","name":"amountOut","type":"uint256"}],
    "stateMutability":"payable",
    "type":"function"
  }
]`

type V3Quoter interface {
	QuoteExactInputSingle(ctx context.Context, req coreswap.Request, fee uint32) (*big.Int, error)
}

type UniswapV3Executor struct {
	RouterAddress string
	Fee           uint32
	Quoter        V3Quoter
	routerABI     abi.ABI
}

type exactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	Fee               *big.Int
	Recipient         common.Address
	Deadline          *big.Int
	AmountIn          *big.Int
	AmountOutMinimum  *big.Int
	SqrtPriceLimitX96 *big.Int
}

func NewUniswapV3Executor(routerAddress string, fee uint32, quoter V3Quoter) (*UniswapV3Executor, error) {
	parsed, err := abi.JSON(strings.NewReader(uniswapV3RouterABI))
	if err != nil {
		return nil, err
	}
	if fee == 0 {
		fee = 3000
	}
	return &UniswapV3Executor{
		RouterAddress: routerAddress,
		Fee:           fee,
		Quoter:        quoter,
		routerABI:     parsed,
	}, nil
}

func (e *UniswapV3Executor) Quote(ctx context.Context, req coreswap.Request) (*coreswap.Quote, error) {
	if e.Quoter == nil {
		return nil, fmt.Errorf("uniswap v3 quote requires on-chain quoter")
	}
	if req.AmountIn == nil || req.AmountIn.Sign() <= 0 {
		return nil, fmt.Errorf("amountIn must be positive")
	}

	amountOut, err := e.Quoter.QuoteExactInputSingle(ctx, req, e.Fee)
	if err != nil {
		return nil, err
	}
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
		FeeBps:    0,
	}, nil
}

func (e *UniswapV3Executor) BuildTransaction(_ context.Context, req coreswap.Request, quote coreswap.Quote) (*coreswap.TransactionIntent, error) {
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

	params := exactInputSingleParams{
		TokenIn:           common.HexToAddress(req.TokenIn),
		TokenOut:          common.HexToAddress(req.TokenOut),
		Fee:               new(big.Int).SetUint64(uint64(e.Fee)),
		Recipient:         common.HexToAddress(req.Recipient),
		Deadline:          new(big.Int).SetUint64(req.DeadlineUnix),
		AmountIn:          req.AmountIn,
		AmountOutMinimum:  quote.MinOut,
		SqrtPriceLimitX96: big.NewInt(0),
	}
	data, err := e.routerABI.Pack("exactInputSingle", params)
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
