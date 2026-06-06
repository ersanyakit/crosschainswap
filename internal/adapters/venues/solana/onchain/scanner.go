package onchain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"exchange/internal/core/chain"
	"exchange/internal/core/venue"
)

const (
	raydiumAMMProgram  = "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8"
	raydiumCLMMProgram = "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK"
	raydiumCPMMProgram = "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C"
	orcaProgram        = "whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"
	meteoraDLMMProgram = "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo"

	orcaWhirlpoolAccountSize = 653
	raydiumCLMMAccountSize   = 1544
	raydiumCPMMAccountSize   = 741
	raydiumAMMAccountSize    = 752
	meteoraDLMMAccountSize   = 904
	getProgramAccountsDelay  = 0 * time.Millisecond
	persistBatchSize         = 250
	maxShardDepth            = 3
	programShardConcurrency  = 32

	orcaToken0Offset        = 101
	orcaToken1Offset        = 181
	raydiumCLMMToken0Offset = 73
	raydiumCLMMToken1Offset = 105
	raydiumCPMMToken0Offset = 168
	raydiumCPMMToken1Offset = 200
	raydiumAMMToken0Offset  = 400
	raydiumAMMToken1Offset  = 432
	meteoraToken0Offset     = 88
	meteoraToken1Offset     = 120
)

type Scanner struct {
	rpc                *RPC
	chainKey           chain.ChainKey
	venueKey           venue.VenueKey
	raydiumAMMProgram  string
	raydiumCLMMProgram string
	raydiumCPMMProgram string
	orcaProgram        string
	orcaConfigAccounts []string
	meteoraDLMMProgram string
}

func NewScanner(rpcURLs []string, chainKey chain.ChainKey, venueKey venue.VenueKey, cfg venue.VenueConfig) (*Scanner, error) {
	rpc, err := NewRPC(rpcURLs)
	if err != nil {
		return nil, err
	}

	s := &Scanner{
		rpc:                rpc,
		chainKey:           chainKey,
		venueKey:           venueKey,
		raydiumAMMProgram:  raydiumAMMProgram,
		raydiumCLMMProgram: raydiumCLMMProgram,
		raydiumCPMMProgram: raydiumCPMMProgram,
		orcaProgram:        orcaProgram,
		meteoraDLMMProgram: meteoraDLMMProgram,
	}

	switch c := cfg.(type) {
	case venue.RaydiumConfig:
		if c.AMMProgramID != "" {
			s.raydiumAMMProgram = c.AMMProgramID
		}
		if c.CLMMProgramID != "" {
			s.raydiumCLMMProgram = c.CLMMProgramID
		}
		if c.CPMMProgramID != "" {
			s.raydiumCPMMProgram = c.CPMMProgramID
		}
	case venue.OrcaConfig:
		if c.WhirlpoolProgramID != "" {
			s.orcaProgram = c.WhirlpoolProgramID
		}
		s.orcaConfigAccounts = c.ConfigAccounts
	case venue.MeteoraConfig:
		if c.DLMMProgramID != "" {
			s.meteoraDLMMProgram = c.DLMMProgramID
		}
	}

	return s, nil
}

func (s *Scanner) LoadPool(ctx context.Context, id venue.PoolID) (*venue.Pool, error) {
	pools, err := s.ScanPools(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		if pools[i].ID == id {
			return &pools[i], nil
		}
	}
	return nil, fmt.Errorf("pool %s not found", id)
}

func (s *Scanner) ScanPools(ctx context.Context) ([]venue.Pool, error) {
	var pools []venue.Pool
	_, err := s.ScanPoolsStream(ctx, func(_ context.Context, batch []venue.Pool) error {
		pools = append(pools, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pools, nil
}

func (s *Scanner) ScanPoolsForAssets(ctx context.Context, assetIDs []venue.AssetID, handle venue.PoolBatchHandler) (int, error) {
	mints := solanaAssetMints(assetIDs)
	if len(mints) == 0 {
		return 0, nil
	}

	switch s.venueKey {
	case venue.VenueKeyOrca:
		return s.scanOrcaForMints(ctx, mints, handle)
	case venue.VenueKeyRaydium:
		return s.scanRaydiumForMints(ctx, mints, handle)
	case venue.VenueKeyMeteora:
		return s.scanMeteoraForMints(ctx, mints, handle)
	default:
		return 0, fmt.Errorf("unsupported solana venue %s", s.venueKey)
	}
}

func (s *Scanner) ScanPoolsStream(ctx context.Context, handle venue.PoolBatchHandler) (int, error) {
	switch s.venueKey {
	case venue.VenueKeyOrca:
		return s.scanOrcaStream(ctx, handle)
	case venue.VenueKeyRaydium:
		return s.scanRaydiumStream(ctx, handle)
	case venue.VenueKeyMeteora:
		return s.scanMeteoraStream(ctx, handle)
	default:
		return 0, fmt.Errorf("unsupported solana venue %s", s.venueKey)
	}
}

func (s *Scanner) scanOrcaStream(ctx context.Context, handle venue.PoolBatchHandler) (int, error) {
	baseFilters := []programFilter{
		dataSizeFilter(orcaWhirlpoolAccountSize),
		memcmpFilter(0, anchorDiscriminator("Whirlpool")),
	}

	if len(s.orcaConfigAccounts) == 0 {
		count, err := s.streamProgramAccountQuery(ctx, s.orcaProgram, baseFilters, orcaToken0Offset, handle, func(account programAccount) (venue.Pool, bool) {
			pool, ok := parseOrcaWhirlpool(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.orcaProgram)
			return pool, ok
		})
		if err != nil {
			return 0, err
		}
		if count == 0 {
			return 0, fmt.Errorf("orca on-chain scan returned no pools")
		}
		return count, nil
	}

	totalScanned := 0
	for _, configAccount := range s.orcaConfigAccounts {
		filters := append([]programFilter{}, baseFilters...)
		filters = append(filters, memcmpStringFilter(8, configAccount))

		count, err := s.streamProgramAccountQuery(ctx, s.orcaProgram, filters, orcaToken0Offset, handle, func(account programAccount) (venue.Pool, bool) {
			pool, ok := parseOrcaWhirlpool(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.orcaProgram)
			return pool, ok
		})
		totalScanned += count
		if err != nil {
			log.Printf("Warning: orca config %s scan failed: %v", configAccount, err)
			continue
		}
	}
	if totalScanned == 0 {
		return 0, fmt.Errorf("orca on-chain scan returned no pools")
	}
	return totalScanned, nil
}

func (s *Scanner) scanRaydiumStream(ctx context.Context, handle venue.PoolBatchHandler) (int, error) {
	totalScanned := 0

	count, err := s.streamProgramAccountQuery(ctx, s.raydiumCLMMProgram, []programFilter{
		dataSizeFilter(raydiumCLMMAccountSize),
		memcmpFilter(0, anchorDiscriminator("PoolState")),
	}, raydiumCLMMToken0Offset, handle, func(account programAccount) (venue.Pool, bool) {
		pool, ok := parseRaydiumCLMM(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.raydiumCLMMProgram)
		return pool, ok
	})
	if err != nil {
		totalScanned += count
		log.Printf("Warning: raydium clmm scan failed: %v", err)
	} else {
		totalScanned += count
	}

	count, err = s.streamProgramAccountQuery(ctx, s.raydiumCPMMProgram, []programFilter{
		dataSizeFilter(raydiumCPMMAccountSize),
		memcmpFilter(0, anchorDiscriminator("PoolState")),
	}, raydiumCPMMToken0Offset, handle, func(account programAccount) (venue.Pool, bool) {
		pool, ok := parseRaydiumCPMM(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.raydiumCPMMProgram)
		return pool, ok
	})
	if err != nil {
		totalScanned += count
		log.Printf("Warning: raydium cpmm scan failed: %v", err)
	} else {
		totalScanned += count
	}

	count, err = s.streamProgramAccountQuery(ctx, s.raydiumAMMProgram, []programFilter{
		dataSizeFilter(raydiumAMMAccountSize),
	}, raydiumAMMToken0Offset, handle, func(account programAccount) (venue.Pool, bool) {
		pool, ok := parseRaydiumAMM(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.raydiumAMMProgram)
		return pool, ok
	})
	if err != nil {
		totalScanned += count
		log.Printf("Warning: raydium amm scan failed: %v", err)
	} else {
		totalScanned += count
	}

	if totalScanned == 0 {
		return 0, fmt.Errorf("raydium on-chain scans returned no pools")
	}

	return totalScanned, nil
}

func (s *Scanner) scanMeteoraStream(ctx context.Context, handle venue.PoolBatchHandler) (int, error) {
	return s.streamProgramAccountQuery(ctx, s.meteoraDLMMProgram, []programFilter{
		dataSizeFilter(meteoraDLMMAccountSize),
		memcmpFilter(0, anchorDiscriminator("LbPair")),
	}, meteoraToken0Offset, handle, func(account programAccount) (venue.Pool, bool) {
		pool, ok := parseMeteoraLbPair(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.meteoraDLMMProgram)
		return pool, ok
	})
}

func (s *Scanner) scanOrcaForMints(ctx context.Context, mints []string, handle venue.PoolBatchHandler) (int, error) {
	baseFilters := []programFilter{
		dataSizeFilter(orcaWhirlpoolAccountSize),
		memcmpFilter(0, anchorDiscriminator("Whirlpool")),
	}
	if len(s.orcaConfigAccounts) == 0 {
		return s.scanProgramForMints(ctx, s.orcaProgram, baseFilters, []int{orcaToken0Offset, orcaToken1Offset}, mints, handle, func(account programAccount) (venue.Pool, bool) {
			return parseOrcaWhirlpool(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.orcaProgram)
		})
	}

	totalScanned := 0
	for _, configAccount := range s.orcaConfigAccounts {
		filters := append([]programFilter{}, baseFilters...)
		filters = append(filters, memcmpStringFilter(8, configAccount))
		count, err := s.scanProgramForMints(ctx, s.orcaProgram, filters, []int{orcaToken0Offset, orcaToken1Offset}, mints, handle, func(account programAccount) (venue.Pool, bool) {
			return parseOrcaWhirlpool(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.orcaProgram)
		})
		totalScanned += count
		if err != nil {
			log.Printf("Warning: targeted orca config %s scan failed: %v", configAccount, err)
			continue
		}
	}
	return totalScanned, nil
}

func (s *Scanner) scanRaydiumForMints(ctx context.Context, mints []string, handle venue.PoolBatchHandler) (int, error) {
	totalScanned := 0

	count, err := s.scanProgramForMints(ctx, s.raydiumCLMMProgram, []programFilter{
		dataSizeFilter(raydiumCLMMAccountSize),
		memcmpFilter(0, anchorDiscriminator("PoolState")),
	}, []int{raydiumCLMMToken0Offset, raydiumCLMMToken1Offset}, mints, handle, func(account programAccount) (venue.Pool, bool) {
		return parseRaydiumCLMM(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.raydiumCLMMProgram)
	})
	totalScanned += count
	if err != nil {
		log.Printf("Warning: targeted raydium clmm scan failed: %v", err)
	}

	count, err = s.scanProgramForMints(ctx, s.raydiumCPMMProgram, []programFilter{
		dataSizeFilter(raydiumCPMMAccountSize),
		memcmpFilter(0, anchorDiscriminator("PoolState")),
	}, []int{raydiumCPMMToken0Offset, raydiumCPMMToken1Offset}, mints, handle, func(account programAccount) (venue.Pool, bool) {
		return parseRaydiumCPMM(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.raydiumCPMMProgram)
	})
	totalScanned += count
	if err != nil {
		log.Printf("Warning: targeted raydium cpmm scan failed: %v", err)
	}

	count, err = s.scanProgramForMints(ctx, s.raydiumAMMProgram, []programFilter{
		dataSizeFilter(raydiumAMMAccountSize),
	}, []int{raydiumAMMToken0Offset, raydiumAMMToken1Offset}, mints, handle, func(account programAccount) (venue.Pool, bool) {
		return parseRaydiumAMM(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.raydiumAMMProgram)
	})
	totalScanned += count
	if err != nil {
		log.Printf("Warning: targeted raydium amm scan failed: %v", err)
	}

	return totalScanned, nil
}

func (s *Scanner) scanMeteoraForMints(ctx context.Context, mints []string, handle venue.PoolBatchHandler) (int, error) {
	return s.scanProgramForMints(ctx, s.meteoraDLMMProgram, []programFilter{
		dataSizeFilter(meteoraDLMMAccountSize),
		memcmpFilter(0, anchorDiscriminator("LbPair")),
	}, []int{meteoraToken0Offset, meteoraToken1Offset}, mints, handle, func(account programAccount) (venue.Pool, bool) {
		return parseMeteoraLbPair(account.Pubkey, account.Data, s.chainKey, s.venueKey, s.meteoraDLMMProgram)
	})
}

func (s *Scanner) scanProgramForMints(
	ctx context.Context,
	programID string,
	baseFilters []programFilter,
	tokenOffsets []int,
	mints []string,
	handle venue.PoolBatchHandler,
	parse func(programAccount) (venue.Pool, bool),
) (int, error) {
	seen := make(map[string]struct{})
	totalScanned := 0

	for _, mint := range mints {
		for _, offset := range tokenOffsets {
			filters := append([]programFilter{}, baseFilters...)
			filters = append(filters, memcmpStringFilter(offset, mint))

			pubkeys, err := s.rpc.GetProgramAccountPubkeys(ctx, programID, filters)
			if err != nil {
				if isRetryableRPCError(err) {
					log.Printf("Warning: targeted Solana scan skipped program=%s mint=%s offset=%d: %v", programID, mint, offset, err)
					continue
				}
				return totalScanned, err
			}

			unique := make([]string, 0, len(pubkeys))
			for _, pubkey := range pubkeys {
				if _, ok := seen[pubkey]; ok {
					continue
				}
				seen[pubkey] = struct{}{}
				unique = append(unique, pubkey)
			}
			if len(unique) == 0 {
				continue
			}

			count, err := s.streamProgramAccountPubkeys(ctx, unique, handle, parse)
			totalScanned += count
			if err != nil {
				return totalScanned, err
			}
		}
	}

	return totalScanned, nil
}

func (s *Scanner) streamProgramAccountQuery(
	ctx context.Context,
	programID string,
	filters []programFilter,
	shardOffset int,
	handle venue.PoolBatchHandler,
	parse func(programAccount) (venue.Pool, bool),
) (int, error) {
	var totalScanned atomic.Int64

	_, err := s.rpc.StreamProgramAccountPubkeysSharded(ctx, programID, filters, shardOffset, func(ctx context.Context, pubkeys []string) error {
		count, err := s.streamProgramAccountPubkeys(ctx, pubkeys, handle, parse)
		if err != nil {
			return err
		}
		totalScanned.Add(int64(count))
		return nil
	})
	if err != nil {
		return int(totalScanned.Load()), err
	}

	return int(totalScanned.Load()), nil
}

func (s *Scanner) streamProgramAccountPubkeys(
	ctx context.Context,
	pubkeys []string,
	handle venue.PoolBatchHandler,
	parse func(programAccount) (venue.Pool, bool),
) (int, error) {
	totalScanned := 0

	for start := 0; start < len(pubkeys); start += persistBatchSize {
		end := min(start+persistBatchSize, len(pubkeys))
		accounts, err := s.rpc.GetAccounts(ctx, pubkeys[start:end])
		if err != nil {
			return totalScanned, err
		}

		count, err := s.streamProgramAccounts(ctx, accounts, handle, parse)
		if err != nil {
			return totalScanned, err
		}
		totalScanned += count
	}

	return totalScanned, nil
}

func (s *Scanner) streamProgramAccounts(
	ctx context.Context,
	accounts []programAccount,
	handle venue.PoolBatchHandler,
	parse func(programAccount) (venue.Pool, bool),
) (int, error) {
	totalScanned := 0

	for start := 0; start < len(accounts); start += persistBatchSize {
		end := min(start+persistBatchSize, len(accounts))
		pools := make([]venue.Pool, 0, end-start)

		for _, account := range accounts[start:end] {
			pool, ok := parse(account)
			if ok {
				pools = append(pools, pool)
			}
		}
		if len(pools) == 0 {
			continue
		}

		pools, err := s.fillVaultBalances(ctx, pools)
		if err != nil {
			return totalScanned, err
		}
		if handle != nil {
			if err := handle(ctx, pools); err != nil {
				return totalScanned, err
			}
		}
		totalScanned += len(pools)
	}

	return totalScanned, nil
}

func (s *Scanner) fillVaultBalances(ctx context.Context, pools []venue.Pool) ([]venue.Pool, error) {
	vaults := make([]string, 0, len(pools)*2)
	seen := make(map[string]struct{}, len(pools)*2)
	for _, pool := range pools {
		if pool.Vault0 != "" {
			if _, ok := seen[pool.Vault0]; !ok {
				seen[pool.Vault0] = struct{}{}
				vaults = append(vaults, pool.Vault0)
			}
		}
		if pool.Vault1 != "" {
			if _, ok := seen[pool.Vault1]; !ok {
				seen[pool.Vault1] = struct{}{}
				vaults = append(vaults, pool.Vault1)
			}
		}
	}
	if len(vaults) == 0 {
		return pools, nil
	}

	balances, err := s.rpc.GetTokenAccountAmounts(ctx, vaults)
	if err != nil {
		return nil, err
	}

	for i := range pools {
		if amount, ok := balances[pools[i].Vault0]; ok {
			pools[i].Reserve0 = amount
		}
		if amount, ok := balances[pools[i].Vault1]; ok {
			pools[i].Reserve1 = amount
		}
	}

	return pools, nil
}

type RPC struct {
	urls          []string
	client        *http.Client
	next          atomic.Uint64
	mu            sync.RWMutex
	disabledUntil map[string]time.Time
}

func NewRPC(urls []string) (*RPC, error) {
	clean := make([]string, 0, len(urls))
	for _, url := range urls {
		if strings.TrimSpace(url) != "" {
			clean = append(clean, strings.TrimSpace(url))
		}
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("solana rpc requires at least one url")
	}
	return &RPC{
		urls:          clean,
		disabledUntil: make(map[string]time.Time),
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        256,
				MaxIdleConnsPerHost: 64,
				MaxConnsPerHost:     64,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 90 * time.Second,
		},
	}, nil
}

func (r *RPC) endpoint() string {
	now := time.Now()
	for i := 0; i < len(r.urls); i++ {
		n := r.next.Add(1) - 1
		url := r.urls[int(n)%len(r.urls)]
		if !r.isEndpointDisabled(url, now) {
			return url
		}
	}

	n := r.next.Add(1) - 1
	return r.urls[int(n)%len(r.urls)]
}

func (r *RPC) isEndpointDisabled(url string, now time.Time) bool {
	r.mu.RLock()
	until, ok := r.disabledUntil[url]
	r.mu.RUnlock()
	return ok && now.Before(until)
}

func (r *RPC) disableEndpoint(url string, d time.Duration, reason error) {
	if url == "" || d <= 0 {
		return
	}

	until := time.Now().Add(d)
	r.mu.Lock()
	current, ok := r.disabledUntil[url]
	if !ok || until.After(current) {
		r.disabledUntil[url] = until
		log.Printf("Warning: temporarily disabling Solana RPC %s for %v: %v", url, d, reason)
	}
	r.mu.Unlock()
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (r *RPC) call(ctx context.Context, method string, params any, out any) error {
	reqBody, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}

	var lastErr error
	attempts := len(r.urls) * 3
	if attempts < 3 {
		attempts = 3
	}

	for i := 0; i < attempts; i++ {
		if method == "getProgramAccounts" && getProgramAccountsDelay > 0 {
			if err := wait(ctx, getProgramAccountsDelay); err != nil {
				return err
			}
		}

		endpoint := r.endpoint()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("rpc %s %s: %w", method, endpoint, err)
			r.disableEndpoint(endpoint, endpointCooldown(lastErr), lastErr)
			if err := waitBeforeRetry(ctx, i); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodySnippet := readBodySnippet(resp)
			lastErr = fmt.Errorf("rpc %s %s http status %d %s%s", method, endpoint, resp.StatusCode, http.StatusText(resp.StatusCode), bodySnippet)
			resp.Body.Close()
			r.disableEndpoint(endpoint, endpointCooldown(lastErr), lastErr)
			if !isRetryableRPCError(lastErr) {
				return lastErr
			}
			if err := waitBeforeRetry(ctx, i); err != nil {
				return err
			}
			continue
		}

		var rpcResp rpcResponse
		err = json.NewDecoder(resp.Body).Decode(&rpcResp)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			if err := waitBeforeRetry(ctx, i); err != nil {
				return err
			}
			continue
		}
		if rpcResp.Error != nil {
			lastErr = fmt.Errorf("rpc %s %s error %d: %s", method, endpoint, rpcResp.Error.Code, rpcResp.Error.Message)
			r.disableEndpoint(endpoint, endpointCooldown(lastErr), lastErr)
			if !isRetryableRPCError(lastErr) {
				return lastErr
			}
			if err := waitBeforeRetry(ctx, i); err != nil {
				return err
			}
			continue
		}

		return json.Unmarshal(rpcResp.Result, out)
	}

	return lastErr
}

func waitBeforeRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt%3+1) * 150 * time.Millisecond
	return wait(ctx, delay)
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readBodySnippet(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil || len(body) == 0 {
		return ""
	}

	return ": " + strings.TrimSpace(string(body))
}

func endpointCooldown(err error) time.Duration {
	if err == nil {
		return 0
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "400 bad request"),
		strings.Contains(msg, "403"),
		strings.Contains(msg, "forbidden"),
		strings.Contains(msg, "temporary redirect"),
		strings.Contains(msg, "307"),
		strings.Contains(msg, "excluded from account secondary indexes"),
		strings.Contains(msg, "method unavailable for key"):
		return 10 * time.Minute
	case strings.Contains(msg, "429"),
		strings.Contains(msg, "too many requests"),
		strings.Contains(msg, "rate limit"):
		return 30 * time.Second
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "eof"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "temporarily unavailable"),
		strings.Contains(msg, "500 internal server error"),
		strings.Contains(msg, "502 bad gateway"),
		strings.Contains(msg, "503 service unavailable"),
		strings.Contains(msg, "504 gateway timeout"):
		return 2 * time.Minute
	default:
		return 0
	}
}

func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "400 bad request") ||
		strings.Contains(msg, "307") ||
		strings.Contains(msg, "temporary redirect") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "administrative rules") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "excluded from account secondary indexes") ||
		strings.Contains(msg, "method unavailable for key") ||
		strings.Contains(msg, "500 internal server error") ||
		strings.Contains(msg, "502 bad gateway") ||
		strings.Contains(msg, "503 service unavailable") ||
		strings.Contains(msg, "504 gateway timeout")
}

type programFilter map[string]any

func memcmpFilter(offset int, bytes []byte) programFilter {
	return programFilter{"memcmp": map[string]any{"offset": offset, "bytes": base58Encode(bytes)}}
}

func memcmpStringFilter(offset int, bytes string) programFilter {
	return programFilter{"memcmp": map[string]any{"offset": offset, "bytes": bytes}}
}

func dataSizeFilter(size int) programFilter {
	return programFilter{"dataSize": size}
}

type programAccount struct {
	Pubkey string
	Data   []byte
}

type pubkeyBatchHandler func(ctx context.Context, pubkeys []string) error

type shardJob struct {
	prefix []byte
}

func (r *RPC) GetProgramAccounts(ctx context.Context, programID string, filters []programFilter) ([]programAccount, error) {
	pubkeys, err := r.GetProgramAccountPubkeys(ctx, programID, filters)
	if err != nil {
		return nil, err
	}

	return r.GetAccounts(ctx, pubkeys)
}

func (r *RPC) GetProgramAccountPubkeys(ctx context.Context, programID string, filters []programFilter) ([]string, error) {
	params := []any{
		programID,
		map[string]any{
			"encoding":  "base64",
			"filters":   filters,
			"dataSlice": map[string]int{"offset": 0, "length": 8},
		},
	}

	var result []struct {
		Pubkey string `json:"pubkey"`
	}
	if err := r.call(ctx, "getProgramAccounts", params, &result); err != nil {
		return nil, err
	}

	pubkeys := make([]string, 0, len(result))
	for _, item := range result {
		if item.Pubkey == "" {
			continue
		}
		pubkeys = append(pubkeys, item.Pubkey)
	}

	return pubkeys, nil
}

func (r *RPC) GetProgramAccountPubkeysSharded(
	ctx context.Context,
	programID string,
	filters []programFilter,
	shardOffset int,
) ([]string, error) {
	out := make([]string, 0)
	var mu sync.Mutex
	_, err := r.StreamProgramAccountPubkeysSharded(ctx, programID, filters, shardOffset, func(_ context.Context, pubkeys []string) error {
		mu.Lock()
		defer mu.Unlock()
		out = append(out, pubkeys...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RPC) StreamProgramAccountPubkeysSharded(
	ctx context.Context,
	programID string,
	filters []programFilter,
	shardOffset int,
	handle pubkeyBatchHandler,
) (int, error) {
	log.Printf(
		"Scanning Solana program %s with %d parallel sharded getProgramAccounts workers at account offset %d",
		programID,
		programShardConcurrency,
		shardOffset,
	)

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan shardJob, 65536)
	var jobWG sync.WaitGroup
	var workerWG sync.WaitGroup
	var total atomic.Int64
	var errMu sync.Mutex
	var firstErr error

	setErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	enqueue := func(prefix []byte) bool {
		if scanCtx.Err() != nil {
			return false
		}

		copied := append([]byte{}, prefix...)
		jobWG.Add(1)
		select {
		case jobs <- shardJob{prefix: copied}:
			return true
		case <-scanCtx.Done():
			jobWG.Done()
			return false
		}
	}

	worker := func() {
		defer workerWG.Done()

		for job := range jobs {
			if scanCtx.Err() != nil {
				jobWG.Done()
				continue
			}

			pubkeys, err := r.GetProgramAccountPubkeys(scanCtx, programID, appendShardFilter(filters, shardOffset, job.prefix))
			if err != nil {
				if isProgramScanLimitError(err) && len(job.prefix) < maxShardDepth {
					for i := 0; i < 256; i++ {
						childPrefix := append(append([]byte{}, job.prefix...), byte(i))
						if !enqueue(childPrefix) {
							break
						}
					}
				} else if isProgramScanLimitError(err) {
					setErr(fmt.Errorf("getProgramAccounts result limit for %s after %d-byte shard at offset %d", programID, len(job.prefix), shardOffset))
				} else if isRetryableRPCError(err) {
					log.Printf("Warning: skipping Solana shard program=%s offset=%d prefix=%x after RPC retries: %v", programID, shardOffset, job.prefix, err)
				} else {
					setErr(err)
				}
				jobWG.Done()
				continue
			}

			if len(pubkeys) > 0 {
				if handle != nil {
					if err := handle(scanCtx, pubkeys); err != nil {
						setErr(err)
					}
				}
				total.Add(int64(len(pubkeys)))
			}
			jobWG.Done()
		}
	}

	for i := 0; i < programShardConcurrency; i++ {
		workerWG.Add(1)
		go worker()
	}

	for i := 0; i < 256; i++ {
		if !enqueue([]byte{byte(i)}) {
			break
		}
	}

	jobWG.Wait()
	close(jobs)
	workerWG.Wait()

	errMu.Lock()
	err := firstErr
	errMu.Unlock()
	if err != nil {
		return int(total.Load()), err
	}
	return int(total.Load()), nil
}

func appendShardFilter(filters []programFilter, offset int, prefix []byte) []programFilter {
	out := make([]programFilter, 0, len(filters)+1)
	out = append(out, filters...)
	out = append(out, memcmpFilter(offset, prefix))
	return out
}

func isProgramScanLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "-32012") ||
		strings.Contains(msg, "scan aborted") ||
		strings.Contains(msg, "accumulated scan results exceeded the limit") ||
		strings.Contains(msg, "exceeded the limit")
}

func (r *RPC) GetAccounts(ctx context.Context, accounts []string) ([]programAccount, error) {
	out := make([]programAccount, 0, len(accounts))

	for start := 0; start < len(accounts); start += 100 {
		end := min(start+100, len(accounts))
		params := []any{
			accounts[start:end],
			map[string]any{"encoding": "base64"},
		}

		var result struct {
			Value []*struct {
				Data []string `json:"data"`
			} `json:"value"`
		}
		if err := r.call(ctx, "getMultipleAccounts", params, &result); err != nil {
			return nil, err
		}

		for i, item := range result.Value {
			if item == nil || len(item.Data) == 0 {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(item.Data[0])
			if err != nil {
				continue
			}
			out = append(out, programAccount{Pubkey: accounts[start+i], Data: data})
		}
	}

	return out, nil
}

func (r *RPC) GetTokenAccountAmounts(ctx context.Context, accounts []string) (map[string]*big.Int, error) {
	out := make(map[string]*big.Int, len(accounts))

	for start := 0; start < len(accounts); start += 100 {
		end := min(start+100, len(accounts))
		params := []any{
			accounts[start:end],
			map[string]any{"encoding": "base64"},
		}

		var result struct {
			Value []*struct {
				Data []string `json:"data"`
			} `json:"value"`
		}
		if err := r.call(ctx, "getMultipleAccounts", params, &result); err != nil {
			return nil, err
		}

		for i, item := range result.Value {
			if item == nil || len(item.Data) == 0 {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(item.Data[0])
			if err != nil || len(data) < 72 {
				continue
			}
			out[accounts[start+i]] = readU64(data, 64)
		}
	}

	return out, nil
}

func parseOrcaWhirlpool(id string, data []byte, chainKey chain.ChainKey, venueKey venue.VenueKey, programID string) (venue.Pool, bool) {
	if len(data) < orcaWhirlpoolAccountSize || !hasDiscriminator(data, "Whirlpool") {
		return venue.Pool{}, false
	}
	return venue.Pool{
		ID:           venue.PoolID(id),
		Address:      id,
		ChainKey:     chainKey,
		VenueKey:     venueKey,
		Kind:         venue.PoolKindCLMM,
		Token0:       venue.AssetID(pubkey(data, 101)),
		Token1:       venue.AssetID(pubkey(data, 181)),
		Reserve0:     big.NewInt(0),
		Reserve1:     big.NewInt(0),
		SqrtPriceX96: readU128(data, 65),
		Liquidity:    readU128(data, 49),
		Tick:         int64(int32(binary.LittleEndian.Uint32(data[81:85]))),
		Fee:          uint32(binary.LittleEndian.Uint16(data[45:47])),
		TickSpacing:  int32(binary.LittleEndian.Uint16(data[41:43])),
		ProgramID:    programID,
		Vault0:       pubkey(data, 133),
		Vault1:       pubkey(data, 213),
		Enabled:      true,
	}, true
}

func parseRaydiumCLMM(id string, data []byte, chainKey chain.ChainKey, venueKey venue.VenueKey, programID string) (venue.Pool, bool) {
	if len(data) < 273 || !hasDiscriminator(data, "PoolState") {
		return venue.Pool{}, false
	}
	return venue.Pool{
		ID:           venue.PoolID(id),
		Address:      id,
		ChainKey:     chainKey,
		VenueKey:     venueKey,
		Kind:         venue.PoolKindCLMM,
		Token0:       venue.AssetID(pubkey(data, 73)),
		Token1:       venue.AssetID(pubkey(data, 105)),
		Reserve0:     big.NewInt(0),
		Reserve1:     big.NewInt(0),
		SqrtPriceX96: readU128(data, 253),
		Liquidity:    readU128(data, 237),
		Tick:         int64(int32(binary.LittleEndian.Uint32(data[269:273]))),
		TickSpacing:  int32(binary.LittleEndian.Uint16(data[235:237])),
		ProgramID:    programID,
		Vault0:       pubkey(data, 137),
		Vault1:       pubkey(data, 169),
		Enabled:      true,
	}, true
}

func parseRaydiumCPMM(id string, data []byte, chainKey chain.ChainKey, venueKey venue.VenueKey, programID string) (venue.Pool, bool) {
	if len(data) < 232 || !hasDiscriminator(data, "PoolState") {
		return venue.Pool{}, false
	}
	return venue.Pool{
		ID:        venue.PoolID(id),
		Address:   id,
		ChainKey:  chainKey,
		VenueKey:  venueKey,
		Kind:      venue.PoolKindV2,
		Token0:    venue.AssetID(pubkey(data, 168)),
		Token1:    venue.AssetID(pubkey(data, 200)),
		Reserve0:  big.NewInt(0),
		Reserve1:  big.NewInt(0),
		ProgramID: programID,
		Vault0:    pubkey(data, 72),
		Vault1:    pubkey(data, 104),
		Enabled:   true,
	}, true
}

func parseRaydiumAMM(id string, data []byte, chainKey chain.ChainKey, venueKey venue.VenueKey, programID string) (venue.Pool, bool) {
	if len(data) < raydiumAMMAccountSize {
		return venue.Pool{}, false
	}
	return venue.Pool{
		ID:        venue.PoolID(id),
		Address:   id,
		ChainKey:  chainKey,
		VenueKey:  venueKey,
		Kind:      venue.PoolKindV2,
		Token0:    venue.AssetID(pubkey(data, 400)),
		Token1:    venue.AssetID(pubkey(data, 432)),
		Reserve0:  big.NewInt(0),
		Reserve1:  big.NewInt(0),
		ProgramID: programID,
		Vault0:    pubkey(data, 336),
		Vault1:    pubkey(data, 368),
		Enabled:   true,
	}, true
}

func parseMeteoraLbPair(id string, data []byte, chainKey chain.ChainKey, venueKey venue.VenueKey, programID string) (venue.Pool, bool) {
	if len(data) < 216 || !hasDiscriminator(data, "LbPair") {
		return venue.Pool{}, false
	}
	return venue.Pool{
		ID:          venue.PoolID(id),
		Address:     id,
		ChainKey:    chainKey,
		VenueKey:    venueKey,
		Kind:        venue.PoolKindCLMM,
		Token0:      venue.AssetID(pubkey(data, 88)),
		Token1:      venue.AssetID(pubkey(data, 120)),
		Reserve0:    big.NewInt(0),
		Reserve1:    big.NewInt(0),
		TickSpacing: int32(binary.LittleEndian.Uint16(data[80:82])),
		ProgramID:   programID,
		Vault0:      pubkey(data, 152),
		Vault1:      pubkey(data, 184),
		Enabled:     true,
	}, true
}

func solanaAssetMints(assetIDs []venue.AssetID) []string {
	seen := make(map[string]struct{}, len(assetIDs))
	out := make([]string, 0, len(assetIDs))
	for _, id := range assetIDs {
		mint := strings.TrimSpace(string(id))
		if mint == "" {
			continue
		}
		if _, ok := seen[mint]; ok {
			continue
		}
		seen[mint] = struct{}{}
		out = append(out, mint)
	}
	return out
}

func anchorDiscriminator(name string) []byte {
	sum := sha256.Sum256([]byte("account:" + name))
	return sum[:8]
}

func hasDiscriminator(data []byte, name string) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], anchorDiscriminator(name))
}

func pubkey(data []byte, offset int) string {
	if len(data) < offset+32 {
		return ""
	}
	return base58Encode(data[offset : offset+32])
}

func readU64(data []byte, offset int) *big.Int {
	if len(data) < offset+8 {
		return big.NewInt(0)
	}
	return new(big.Int).SetUint64(binary.LittleEndian.Uint64(data[offset : offset+8]))
}

func readU128(data []byte, offset int) *big.Int {
	if len(data) < offset+16 {
		return big.NewInt(0)
	}
	bytesLE := data[offset : offset+16]
	bytesBE := make([]byte, 16)
	for i := range bytesLE {
		bytesBE[len(bytesLE)-1-i] = bytesLE[i]
	}
	return new(big.Int).SetBytes(bytesBE)
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(input []byte) string {
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var out []byte

	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		out = append(out, base58Alphabet[mod.Int64()])
	}

	for _, b := range input {
		if b != 0 {
			break
		}
		out = append(out, base58Alphabet[0])
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return string(out)
}
