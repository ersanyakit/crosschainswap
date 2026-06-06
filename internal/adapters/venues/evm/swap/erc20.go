package swap

import (
	"fmt"
	"math/big"
	"strings"

	coreswap "exchange/internal/core/swap"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const erc20ABI = `[
  {
    "inputs": [
      {"internalType":"address","name":"spender","type":"address"},
      {"internalType":"uint256","name":"amount","type":"uint256"}
    ],
    "name":"approve",
    "outputs":[{"internalType":"bool","name":"","type":"bool"}],
    "stateMutability":"nonpayable",
    "type":"function"
  }
]`

func BuildApproveTransaction(tokenAddress string, spender string, amount *big.Int) (*coreswap.EVMTransaction, error) {
	if !common.IsHexAddress(tokenAddress) {
		return nil, fmt.Errorf("invalid token address: %s", tokenAddress)
	}
	if !common.IsHexAddress(spender) {
		return nil, fmt.Errorf("invalid spender address: %s", spender)
	}
	if amount == nil || amount.Sign() <= 0 {
		return nil, fmt.Errorf("approve amount must be positive")
	}

	parsed, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}
	data, err := parsed.Pack("approve", common.HexToAddress(spender), amount)
	if err != nil {
		return nil, err
	}

	return &coreswap.EVMTransaction{
		To:    common.HexToAddress(tokenAddress).Hex(),
		Data:  data,
		Value: big.NewInt(0),
	}, nil
}
