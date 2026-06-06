package swap

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	evmrpc "exchange/internal/adapters/venues/evm/rpc"
	"exchange/internal/core/chain"
	coreswap "exchange/internal/core/swap"
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const uniswapV3QuoterV2ABI = `[
  {
    "inputs": [
      {
        "components": [
          {"internalType":"address","name":"tokenIn","type":"address"},
          {"internalType":"address","name":"tokenOut","type":"address"},
          {"internalType":"uint256","name":"amountIn","type":"uint256"},
          {"internalType":"uint24","name":"fee","type":"uint24"},
          {"internalType":"uint160","name":"sqrtPriceLimitX96","type":"uint160"}
        ],
        "internalType":"struct IQuoterV2.QuoteExactInputSingleParams",
        "name":"params",
        "type":"tuple"
      }
    ],
    "name":"quoteExactInputSingle",
    "outputs":[
      {"internalType":"uint256","name":"amountOut","type":"uint256"},
      {"internalType":"uint160","name":"sqrtPriceX96After","type":"uint160"},
      {"internalType":"uint32","name":"initializedTicksCrossed","type":"uint32"},
      {"internalType":"uint256","name":"gasEstimate","type":"uint256"}
    ],
    "stateMutability":"nonpayable",
    "type":"function"
  }
]`

const uniswapV3QuoterV1ABI = `[
  {
    "inputs": [
      {"internalType":"address","name":"tokenIn","type":"address"},
      {"internalType":"address","name":"tokenOut","type":"address"},
      {"internalType":"uint24","name":"fee","type":"uint24"},
      {"internalType":"uint256","name":"amountIn","type":"uint256"},
      {"internalType":"uint160","name":"sqrtPriceLimitX96","type":"uint160"}
    ],
    "name":"quoteExactInputSingle",
    "outputs":[{"internalType":"uint256","name":"amountOut","type":"uint256"}],
    "stateMutability":"nonpayable",
    "type":"function"
  }
]`

const aerodromeSlipstreamQuoterABI = `[
  {
    "inputs": [
      {
        "components": [
          {"internalType":"address","name":"tokenIn","type":"address"},
          {"internalType":"address","name":"tokenOut","type":"address"},
          {"internalType":"uint256","name":"amountIn","type":"uint256"},
          {"internalType":"int24","name":"tickSpacing","type":"int24"},
          {"internalType":"uint160","name":"sqrtPriceLimitX96","type":"uint160"}
        ],
        "internalType":"struct IQuoterV2.QuoteExactInputSingleParams",
        "name":"params",
        "type":"tuple"
      }
    ],
    "name":"quoteExactInputSingle",
    "outputs":[
      {"internalType":"uint256","name":"amountOut","type":"uint256"},
      {"internalType":"uint160","name":"sqrtPriceX96After","type":"uint160"},
      {"internalType":"uint32","name":"initializedTicksCrossed","type":"uint32"},
      {"internalType":"uint256","name":"gasEstimate","type":"uint256"}
    ],
    "stateMutability":"nonpayable",
    "type":"function"
  }
]`

type UniswapV3Quoter struct {
	RPCByChain        map[chain.ChainKey]*evmrpc.Pool
	QuoterByVenue     map[venue.VenueKey]common.Address
	SlipstreamByVenue map[venue.VenueKey]bool
	quoterV2ABI       abi.ABI
	quoterV1ABI       abi.ABI
	slipstreamABI     abi.ABI
}

type quoteExactInputSingleV2Params struct {
	TokenIn           common.Address
	TokenOut          common.Address
	AmountIn          *big.Int
	Fee               *big.Int
	SqrtPriceLimitX96 *big.Int
}

type slipstreamQuoteExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	AmountIn          *big.Int
	TickSpacing       *big.Int
	SqrtPriceLimitX96 *big.Int
}

func NewUniswapV3Quoter(
	rpcByChain map[chain.ChainKey]*evmrpc.Pool,
	quoterByVenue map[venue.VenueKey]common.Address,
	slipstreamByVenue map[venue.VenueKey]bool,
) (*UniswapV3Quoter, error) {
	quoterV2ABI, err := abi.JSON(strings.NewReader(uniswapV3QuoterV2ABI))
	if err != nil {
		return nil, err
	}
	quoterV1ABI, err := abi.JSON(strings.NewReader(uniswapV3QuoterV1ABI))
	if err != nil {
		return nil, err
	}
	slipstreamABI, err := abi.JSON(strings.NewReader(aerodromeSlipstreamQuoterABI))
	if err != nil {
		return nil, err
	}
	return &UniswapV3Quoter{
		RPCByChain:        rpcByChain,
		QuoterByVenue:     quoterByVenue,
		SlipstreamByVenue: slipstreamByVenue,
		quoterV2ABI:       quoterV2ABI,
		quoterV1ABI:       quoterV1ABI,
		slipstreamABI:     slipstreamABI,
	}, nil
}

func (q *UniswapV3Quoter) QuoteExactInputSingle(ctx context.Context, req coreswap.Request, fee uint32, tickSpacing int32) (*big.Int, error) {
	if q == nil {
		return nil, fmt.Errorf("uniswap v3 quoter is not configured")
	}
	rpcPool, ok := q.RPCByChain[req.ChainKey]
	if !ok || rpcPool == nil {
		return nil, fmt.Errorf("uniswap v3 quoter rpc is not configured for %s", req.ChainKey)
	}
	quoter, ok := q.QuoterByVenue[req.VenueKey]
	if !ok || quoter == (common.Address{}) {
		return nil, fmt.Errorf("uniswap v3 quoter address is not configured for %s", req.VenueKey)
	}

	if q.SlipstreamByVenue[req.VenueKey] {
		return q.quoteSlipstream(ctx, rpcPool, quoter, req, tickSpacing)
	}

	amountOut, err := q.quoteV2(ctx, rpcPool, quoter, req, fee)
	if err == nil {
		return amountOut, nil
	}

	return q.quoteV1(ctx, rpcPool, quoter, req, fee)
}

func (q *UniswapV3Quoter) quoteSlipstream(
	ctx context.Context,
	rpcPool *evmrpc.Pool,
	quoter common.Address,
	req coreswap.Request,
	tickSpacing int32,
) (*big.Int, error) {
	if tickSpacing == 0 {
		return nil, fmt.Errorf("tick spacing is required for slipstream quote")
	}
	params := slipstreamQuoteExactInputSingleParams{
		TokenIn:           common.HexToAddress(req.TokenIn),
		TokenOut:          common.HexToAddress(req.TokenOut),
		AmountIn:          req.AmountIn,
		TickSpacing:       big.NewInt(int64(tickSpacing)),
		SqrtPriceLimitX96: big.NewInt(0),
	}
	data, err := q.slipstreamABI.Pack("quoteExactInputSingle", params)
	if err != nil {
		return nil, err
	}
	out, err := rpcPool.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: data})
	if err != nil {
		return nil, err
	}
	values, err := q.slipstreamABI.Unpack("quoteExactInputSingle", out)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty slipstream quoter response")
	}
	amountOut, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("invalid slipstream quoter amount type %T", values[0])
	}
	return amountOut, nil
}

func (q *UniswapV3Quoter) quoteV2(
	ctx context.Context,
	rpcPool *evmrpc.Pool,
	quoter common.Address,
	req coreswap.Request,
	fee uint32,
) (*big.Int, error) {
	params := quoteExactInputSingleV2Params{
		TokenIn:           common.HexToAddress(req.TokenIn),
		TokenOut:          common.HexToAddress(req.TokenOut),
		AmountIn:          req.AmountIn,
		Fee:               new(big.Int).SetUint64(uint64(fee)),
		SqrtPriceLimitX96: big.NewInt(0),
	}
	data, err := q.quoterV2ABI.Pack("quoteExactInputSingle", params)
	if err != nil {
		return nil, err
	}
	out, err := rpcPool.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: data})
	if err != nil {
		return nil, err
	}
	values, err := q.quoterV2ABI.Unpack("quoteExactInputSingle", out)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty uniswap v3 quoter response")
	}
	amountOut, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("invalid uniswap v3 quoter amount type %T", values[0])
	}
	return amountOut, nil
}

func (q *UniswapV3Quoter) quoteV1(
	ctx context.Context,
	rpcPool *evmrpc.Pool,
	quoter common.Address,
	req coreswap.Request,
	fee uint32,
) (*big.Int, error) {
	data, err := q.quoterV1ABI.Pack(
		"quoteExactInputSingle",
		common.HexToAddress(req.TokenIn),
		common.HexToAddress(req.TokenOut),
		new(big.Int).SetUint64(uint64(fee)),
		req.AmountIn,
		big.NewInt(0),
	)
	if err != nil {
		return nil, err
	}
	out, err := rpcPool.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: data})
	if err != nil {
		return nil, err
	}
	values, err := q.quoterV1ABI.Unpack("quoteExactInputSingle", out)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty uniswap v3 quoter response")
	}
	amountOut, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("invalid uniswap v3 quoter amount type %T", values[0])
	}
	return amountOut, nil
}
