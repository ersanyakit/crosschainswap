package aerodrome

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const factoryABI = `[
  {"inputs":[],"name":"allPoolsLength","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"uint256","name":"","type":"uint256"}],"name":"allPools","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}
]`

const pairABI = `[
  {"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"getReserves","outputs":[
    {"internalType":"uint256","name":"_reserve0","type":"uint256"},
    {"internalType":"uint256","name":"_reserve1","type":"uint256"},
    {"internalType":"uint256","name":"_blockTimestampLast","type":"uint256"}
  ],"stateMutability":"view","type":"function"}
]`

func callContractWithRetry(ctx context.Context, client *ethclient.Client, msg ethereum.CallMsg) ([]byte, error) {
	var out []byte
	var err error
	maxRetries := 5
	backoff := 200 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		out, err = client.CallContract(ctx, msg, nil)
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return nil, err
}

func callPoolsLength(ctx context.Context, client *ethclient.Client, contract common.Address) (*big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return nil, err
	}

	data, err := parsed.Pack("allPoolsLength")
	if err != nil {
		return nil, err
	}

	out, err := callContractWithRetry(ctx, client, ethereum.CallMsg{
		To:   &contract,
		Data: data,
	})
	if err != nil {
		return nil, err
	}

	values, err := parsed.Unpack("allPoolsLength", out)
	if err != nil {
		return nil, err
	}

	return values[0].(*big.Int), nil
}

func callPoolAt(ctx context.Context, client *ethclient.Client, contract common.Address, index *big.Int) (common.Address, error) {
	parsed, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return common.Address{}, err
	}

	data, err := parsed.Pack("allPools", index)
	if err != nil {
		return common.Address{}, err
	}

	out, err := callContractWithRetry(ctx, client, ethereum.CallMsg{
		To:   &contract,
		Data: data,
	})
	if err != nil {
		return common.Address{}, err
	}

	values, err := parsed.Unpack("allPools", out)
	if err != nil {
		return common.Address{}, err
	}

	return values[0].(common.Address), nil
}

func callAddress(ctx context.Context, client *ethclient.Client, contract common.Address, method string) (common.Address, error) {
	parsed, err := abi.JSON(strings.NewReader(pairABI))
	if err != nil {
		return common.Address{}, err
	}

	data, err := parsed.Pack(method)
	if err != nil {
		return common.Address{}, err
	}

	out, err := callContractWithRetry(ctx, client, ethereum.CallMsg{
		To:   &contract,
		Data: data,
	})
	if err != nil {
		return common.Address{}, err
	}

	values, err := parsed.Unpack(method, out)
	if err != nil {
		return common.Address{}, err
	}

	return values[0].(common.Address), nil
}

func callReserves(ctx context.Context, client *ethclient.Client, pair common.Address) (*big.Int, *big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(pairABI))
	if err != nil {
		return nil, nil, err
	}

	data, err := parsed.Pack("getReserves")
	if err != nil {
		return nil, nil, err
	}

	out, err := callContractWithRetry(ctx, client, ethereum.CallMsg{
		To:   &pair,
		Data: data,
	})
	if err != nil {
		return nil, nil, err
	}

	values, err := parsed.Unpack("getReserves", out)
	if err != nil {
		return nil, nil, err
	}

	return values[0].(*big.Int), values[1].(*big.Int), nil
}
