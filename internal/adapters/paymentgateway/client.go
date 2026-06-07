package paymentgateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "http://localhost:3001"

type Config struct {
	BaseURL    string
	APIKey     string
	APISecret  string
	MerchantID string
	DomainID   string
	ProductID  string
	Timeout    time.Duration
}

type Client struct {
	cfg  Config
	http *http.Client
}

type WalletAddress struct {
	ChainKey string
	Address  string
}

type walletCreateRequest struct {
	MerchantID string `json:"merchant_id,omitempty"`
	DomainID   string `json:"domain_id,omitempty"`
	ProductID  string `json:"product_id,omitempty"`
	UserID     string `json:"user_id"`
}

type walletCreateResponse struct {
	Ethereum  string `json:"ethereum"`
	Base      string `json:"base"`
	Chiliz    string `json:"chiliz"`
	Solana    string `json:"solana"`
	Avalanche string `json:"avalanche"`
	Unichain  string `json:"unichain"`
	Arbitrum  string `json:"arbitrum"`
	BNBChain  string `json:"bnbchain"`
	Bitcoin   string `json:"bitcoin"`
	Tron      string `json:"tron"`
}

func ConfigFromEnv() Config {
	return Config{
		BaseURL:    envOrDefault("PAYMENT_GATEWAY_BASE_URL", defaultBaseURL),
		APIKey:     strings.TrimSpace(os.Getenv("PAYMENT_GATEWAY_API_KEY")),
		APISecret:  strings.TrimSpace(os.Getenv("PAYMENT_GATEWAY_API_SECRET")),
		MerchantID: strings.TrimSpace(os.Getenv("PAYMENT_GATEWAY_MERCHANT_ID")),
		DomainID:   strings.TrimSpace(os.Getenv("PAYMENT_GATEWAY_DOMAIN_ID")),
		ProductID:  strings.TrimSpace(os.Getenv("PAYMENT_GATEWAY_PRODUCT_ID")),
		Timeout:    durationFromEnv("PAYMENT_GATEWAY_TIMEOUT", 10*time.Second),
	}
}

func NewClient(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.cfg.BaseURL != "" && c.cfg.MerchantID != "" && c.cfg.DomainID != "" && c.cfg.ProductID != ""
}

func (c *Client) CreateUserWallet(ctx context.Context, userID string) ([]WalletAddress, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("gateway wallet user_id is required")
	}
	if !c.Enabled() {
		return nil, fmt.Errorf("payment gateway wallet client is not configured")
	}
	payload := walletCreateRequest{
		MerchantID: c.cfg.MerchantID,
		DomainID:   c.cfg.DomainID,
		ProductID:  c.cfg.ProductID,
		UserID:     userID,
	}
	var response walletCreateResponse
	if err := c.postJSON(ctx, "/merchant.wallet.create", payload, &response); err != nil {
		return nil, err
	}
	return response.wallets(), nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint, err := c.url(path)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.sign(req, body)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("payment gateway %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}

func (c *Client) url(path string) (string, error) {
	base, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(rel).String(), nil
}

func (c *Client) sign(req *http.Request, body []byte) {
	if c.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", c.cfg.APIKey)
	}
	if c.cfg.APISecret == "" {
		return
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(c.cfg.APISecret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	req.Header.Set("X-API-Secret", c.cfg.APISecret)
	req.Header.Set("X-Gateway-Timestamp", timestamp)
	req.Header.Set("X-Gateway-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
}

func (r walletCreateResponse) wallets() []WalletAddress {
	candidates := []WalletAddress{
		{ChainKey: "ethereum", Address: r.Ethereum},
		{ChainKey: "base", Address: r.Base},
		{ChainKey: "chiliz", Address: r.Chiliz},
		{ChainKey: "solana", Address: r.Solana},
		{ChainKey: "avalanche", Address: r.Avalanche},
		{ChainKey: "unichain", Address: r.Unichain},
		{ChainKey: "arbitrum", Address: r.Arbitrum},
		{ChainKey: "binance_smart_chain", Address: r.BNBChain},
	}
	out := make([]WalletAddress, 0, len(candidates))
	for _, item := range candidates {
		item.Address = strings.TrimSpace(item.Address)
		if item.Address != "" {
			out = append(out, item)
		}
	}
	return out
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
