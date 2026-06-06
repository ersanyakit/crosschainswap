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

const aerodromeSlipstreamRouterABI = `[
  {
    "inputs": [
      {
        "components": [
          {"internalType":"address","name":"tokenIn","type":"address"},
          {"internalType":"address","name":"tokenOut","type":"address"},
          {"internalType":"int24","name":"tickSpacing","type":"int24"},
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
	QuoteExactInputSingle(ctx context.Context, req coreswap.Request, fee uint32, tickSpacing int32) (*big.Int, error)
}

type UniswapV3Executor struct {
	RouterAddress       string
	RouterByVenue       map[venue.VenueKey]string
	SlipstreamByVenue   map[venue.VenueKey]bool
	Fee                 uint32
	FeeByVenue          map[venue.VenueKey]uint32
	Quoter              V3Quoter
	Pools               PoolProvider
	routerABI           abi.ABI
	slipstreamRouterABI abi.ABI
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

type slipstreamExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	TickSpacing       *big.Int
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
	parsedSlipstream, err := abi.JSON(strings.NewReader(aerodromeSlipstreamRouterABI))
	if err != nil {
		return nil, err
	}
	if fee == 0 {
		fee = 3000
	}
	return &UniswapV3Executor{
		RouterAddress:       routerAddress,
		Fee:                 fee,
		Quoter:              quoter,
		routerABI:           parsed,
		slipstreamRouterABI: parsedSlipstream,
	}, nil
}

func (e *UniswapV3Executor) Quote(ctx context.Context, req coreswap.Request) (*coreswap.Quote, error) {
	if e.Quoter == nil {
		return nil, fmt.Errorf("uniswap v3 quote requires on-chain quoter")
	}
	if req.AmountIn == nil || req.AmountIn.Sign() <= 0 {
		return nil, fmt.Errorf("amountIn must be positive")
	}

	fee, tickSpacing, err := e.routeParams(ctx, req)
	if err != nil {
		return nil, err
	}

	amountOut, err := e.Quoter.QuoteExactInputSingle(ctx, req, fee, tickSpacing)
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
		FeeBps:    fee / 100,
	}, nil
}

func (e *UniswapV3Executor) BuildTransaction(ctx context.Context, req coreswap.Request, quote coreswap.Quote) (*coreswap.TransactionIntent, error) {
	routerAddress, err := e.routerAddress(req)
	if err != nil {
		return nil, err
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
	fee, tickSpacing, err := e.routeParams(ctx, req)
	if err != nil {
		return nil, err
	}

	if e.isSlipstream(req.VenueKey) {
		params := slipstreamExactInputSingleParams{
			TokenIn:           common.HexToAddress(req.TokenIn),
			TokenOut:          common.HexToAddress(req.TokenOut),
			TickSpacing:       big.NewInt(int64(tickSpacing)),
			Recipient:         common.HexToAddress(req.Recipient),
			Deadline:          new(big.Int).SetUint64(req.DeadlineUnix),
			AmountIn:          req.AmountIn,
			AmountOutMinimum:  quote.MinOut,
			SqrtPriceLimitX96: big.NewInt(0),
		}
		data, err := e.slipstreamRouterABI.Pack("exactInputSingle", params)
		if err != nil {
			return nil, err
		}

		return &coreswap.TransactionIntent{
			ChainKey:  req.ChainKey,
			VenueKey:  req.VenueKey,
			VenueKind: req.VenueKind,
			EVM: &coreswap.EVMTransaction{
				To:    common.HexToAddress(routerAddress).Hex(),
				Data:  data,
				Value: big.NewInt(0),
			},
		}, nil
	}

	params := exactInputSingleParams{
		TokenIn:           common.HexToAddress(req.TokenIn),
		TokenOut:          common.HexToAddress(req.TokenOut),
		Fee:               new(big.Int).SetUint64(uint64(fee)),
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
		VenueKey:  req.VenueKey,
		VenueKind: req.VenueKind,
		EVM: &coreswap.EVMTransaction{
			To:    common.HexToAddress(routerAddress).Hex(),
			Data:  data,
			Value: big.NewInt(0),
		},
	}, nil
}

func (e *UniswapV3Executor) routerAddress(req coreswap.Request) (string, error) {
	if e.RouterByVenue != nil {
		if routerAddress, ok := e.RouterByVenue[req.VenueKey]; ok && routerAddress != "" {
			return routerAddress, nil
		}
	}
	if e.RouterAddress != "" {
		return e.RouterAddress, nil
	}
	return "", fmt.Errorf("router address is required for venue %s", req.VenueKey)
}

func (e *UniswapV3Executor) routeParams(ctx context.Context, req coreswap.Request) (uint32, int32, error) {
	poolID := requestPoolID(req)
	if e.Pools != nil && poolID != "" {
		pool, err := e.Pools.GetPool(ctx, poolID)
		if err != nil {
			return 0, 0, err
		}
		if pool == nil {
			return 0, 0, fmt.Errorf("pool %s not found", poolID)
		}
		if e.isSlipstream(req.VenueKey) && pool.TickSpacing == 0 {
			return 0, 0, fmt.Errorf("tick spacing is required for slipstream pool %s", poolID)
		}
		if pool.Fee > 0 {
			return pool.Fee, pool.TickSpacing, nil
		}
		return 0, pool.TickSpacing, nil
	}
	if e.FeeByVenue != nil {
		if fee, ok := e.FeeByVenue[req.VenueKey]; ok && fee > 0 {
			return fee, 0, nil
		}
	}
	if e.Fee > 0 {
		return e.Fee, 0, nil
	}
	return 3000, 0, nil
}

func (e *UniswapV3Executor) isSlipstream(venueKey venue.VenueKey) bool {
	if e.SlipstreamByVenue == nil {
		return false
	}
	return e.SlipstreamByVenue[venueKey]
}
