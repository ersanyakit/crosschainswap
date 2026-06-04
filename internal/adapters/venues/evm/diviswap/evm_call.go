package diviswap

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
  {"inputs":[],"name":"allPairsLength","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"uint256","name":"","type":"uint256"}],"name":"allPairs","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"}
]`

const pairABI = `[
  {"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
  {"inputs":[],"name":"getReserves","outputs":[
    {"internalType":"uint112","name":"_reserve0","type":"uint112"},
    {"internalType":"uint112","name":"_reserve1","type":"uint112"},
    {"internalType":"uint32","name":"_blockTimestampLast","type":"uint32"}
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

func callUint256(ctx context.Context, client *ethclient.Client, contract common.Address, method string) (*big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return nil, err
	}

	methodName := methodNameOnly(method)

	data, err := parsed.Pack(methodName)
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

	values, err := parsed.Unpack(methodName, out)
	if err != nil {
		return nil, err
	}

	return values[0].(*big.Int), nil
}

func callAddressWithUint256(ctx context.Context, client *ethclient.Client, contract common.Address, method string, value *big.Int) (common.Address, error) {
	parsed, err := abi.JSON(strings.NewReader(factoryABI))
	if err != nil {
		return common.Address{}, err
	}

	methodName := methodNameOnly(method)

	data, err := parsed.Pack(methodName, value)
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

	values, err := parsed.Unpack(methodName, out)
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

	methodName := methodNameOnly(method)

	data, err := parsed.Pack(methodName)
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

	values, err := parsed.Unpack(methodName, out)
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

	reserve0 := values[0].(*big.Int)
	reserve1 := values[1].(*big.Int)

	return reserve0, reserve1, nil
}

func methodNameOnly(signature string) string {
	if idx := strings.Index(signature, "("); idx >= 0 {
		return signature[:idx]
	}
	return signature
}
