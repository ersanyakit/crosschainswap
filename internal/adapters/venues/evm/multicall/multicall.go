package multicall

import (
	"context"
	"exchange/internal/adapters/venues/evm/rpc"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const ABI = `[
	{
		"inputs": [
			{
				"components": [
					{"internalType":"address","name":"target","type":"address"},
					{"internalType":"bool","name":"allowFailure","type":"bool"},
					{"internalType":"bytes","name":"callData","type":"bytes"}
				],
				"internalType":"struct Multicall3.Call3[]",
				"name":"calls",
				"type":"tuple[]"
			}
		],
		"name":"aggregate3",
		"outputs": [
			{
				"components": [
					{"internalType":"bool","name":"success","type":"bool"},
					{"internalType":"bytes","name":"returnData","type":"bytes"}
				],
				"internalType":"struct Multicall3.Result[]",
				"name":"returnData",
				"type":"tuple[]"
			}
		],
		"stateMutability":"payable",
		"type":"function"
	}
]`

type Call3 struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

type Result struct {
	Success    bool
	ReturnData []byte
}

type Client struct {
	rpc     *rpc.Pool
	address common.Address
	abi     abi.ABI
}

func NewClient(rpc *rpc.Pool, address common.Address) (*Client, error) {
	parsed, err := abi.JSON(strings.NewReader(ABI))
	if err != nil {
		return nil, err
	}

	return &Client{
		rpc:     rpc,
		address: address,
		abi:     parsed,
	}, nil
}

func (c *Client) Aggregate3(
	ctx context.Context,
	calls []Call3,
) ([]Result, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	data, err := c.abi.Pack("aggregate3", calls)
	if err != nil {
		return nil, err
	}

	out, err := c.rpc.CallContract(ctx, ethereum.CallMsg{
		To:    &c.address,
		Data:  data,
		Value: big.NewInt(0),
	})
	if err != nil {
		return nil, err
	}

	values, err := c.abi.Unpack("aggregate3", out)
	if err != nil {
		return nil, err
	}

	results, ok := values[0].([]struct {
		Success    bool   `json:"success"`
		ReturnData []byte `json:"returnData"`
	})
	if !ok {
		return nil, fmt.Errorf("invalid aggregate3 result type")
	}

	outResults := make([]Result, 0, len(results))

	for _, r := range results {
		outResults = append(outResults, Result{
			Success:    r.Success,
			ReturnData: r.ReturnData,
		})
	}

	return outResults, nil
}
