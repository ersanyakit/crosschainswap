package rpc

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Pool struct {
	clients []*ethclient.Client
	next    atomic.Uint64
}

func New(urls []string) (*Pool, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("rpc pool requires at least one url")
	}

	clients := make([]*ethclient.Client, 0, len(urls))

	for _, url := range urls {
		if strings.TrimSpace(url) == "" {
			continue
		}

		client, err := ethclient.Dial(url)
		if err != nil {
			continue
		}

		clients = append(clients, client)
	}

	if len(clients) == 0 {
		return nil, fmt.Errorf("failed to connect to all rpc urls")
	}

	return &Pool{
		clients: clients,
	}, nil
}

func (p *Pool) Close() {
	for _, c := range p.clients {
		c.Close()
	}
}

func (p *Pool) Client() *ethclient.Client {
	i := p.next.Add(1) - 1
	return p.clients[int(i)%len(p.clients)]
}

func (p *Pool) CallContract(
	ctx context.Context,
	msg ethereum.CallMsg,
) ([]byte, error) {
	var lastErr error
	attempts := len(p.clients) * 3
	if attempts < 3 {
		attempts = 3
	}

	for i := 0; i < attempts; i++ {
		client := p.Client()

		out, err := client.CallContract(ctx, msg, nil)
		if err == nil {
			return out, nil
		}

		lastErr = err

		if !isRetryableRPCError(err) {
			return nil, err
		}

		sleep := time.Duration(i%len(p.clients)+1) * 250 * time.Millisecond

		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func (p *Pool) BalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	var lastErr error
	attempts := len(p.clients) * 3
	if attempts < 3 {
		attempts = 3
	}

	for i := 0; i < attempts; i++ {
		client := p.Client()

		out, err := client.BalanceAt(ctx, account, nil)
		if err == nil {
			return out, nil
		}

		lastErr = err

		if !isRetryableRPCError(err) {
			return nil, err
		}

		sleep := time.Duration(i%len(p.clients)+1) * 250 * time.Millisecond
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "administrative rules") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "over rate limit") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "500 internal server error") ||
		strings.Contains(msg, "502 bad gateway") ||
		strings.Contains(msg, "503 service unavailable") ||
		strings.Contains(msg, "504 gateway timeout")
}
