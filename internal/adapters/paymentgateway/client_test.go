package paymentgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigFromEnvReadsSecretKeyAlias(t *testing.T) {
	t.Setenv("PAYMENT_GATEWAY_API_SECRET", "")
	t.Setenv("PAYMENT_GATEWAY_API_SECRET_KEY", "")
	t.Setenv("PAYMENT_GATEWAY_SECRET_KEY", "alias-secret")

	cfg := ConfigFromEnv()
	if cfg.APISecret != "alias-secret" {
		t.Fatalf("APISecret = %q, want alias-secret", cfg.APISecret)
	}
}

func TestConfigFromEnvPrefersAPISecret(t *testing.T) {
	t.Setenv("PAYMENT_GATEWAY_API_SECRET", "api-secret")
	t.Setenv("PAYMENT_GATEWAY_SECRET_KEY", "alias-secret")

	cfg := ConfigFromEnv()
	if cfg.APISecret != "api-secret" {
		t.Fatalf("APISecret = %q, want api-secret", cfg.APISecret)
	}
}

func TestClientEnabledRequiresWalletCreateScope(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "http://localhost:3001",
		MerchantID: "merchant-1",
	})

	if client.Enabled() {
		t.Fatal("client should not be enabled without domain id")
	}

	client = NewClient(Config{
		BaseURL:    "http://localhost:3001",
		MerchantID: "550e8400-e29b-41d4-a716-446655440000",
		DomainID:   "550e8400-e29b-41d4-a716-446655440001",
	})
	if !client.Enabled() {
		t.Fatal("client should be enabled with merchant and domain ids")
	}
}

func TestCreateUserWalletRejectsInvalidScopeBeforeHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "missing domain",
			cfg: Config{
				BaseURL:    server.URL,
				MerchantID: "550e8400-e29b-41d4-a716-446655440000",
				ProductID:  "exchange",
			},
			wantErr: "domain_id is required",
		},
		{
			name: "invalid merchant uuid",
			cfg: Config{
				BaseURL:    server.URL,
				MerchantID: "not-a-uuid",
				DomainID:   "550e8400-e29b-41d4-a716-446655440001",
				ProductID:  "exchange",
			},
			wantErr: "merchant_id must be a UUID",
		},
		{
			name: "invalid domain uuid",
			cfg: Config{
				BaseURL:    server.URL,
				MerchantID: "550e8400-e29b-41d4-a716-446655440000",
				DomainID:   "not-a-uuid",
				ProductID:  "exchange",
			},
			wantErr: "domain_id must be a UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)
			_, err := client.CreateUserWallet(context.Background(), "user-1")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CreateUserWallet error = %v, want %q", err, tt.wantErr)
			}
		})
	}

	if calls != 0 {
		t.Fatal("CreateUserWallet should validate scope before calling the gateway")
	}
}

func TestCreateUserWalletPostsGatewayScope(t *testing.T) {
	var captured walletCreateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/merchant.wallet.create" {
			t.Fatalf("path = %s, want /merchant.wallet.create", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{"ethereum":"0xabc","solana":"sol"}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:    server.URL,
		MerchantID: "550e8400-e29b-41d4-a716-446655440000",
		DomainID:   "550e8400-e29b-41d4-a716-446655440001",
		ProductID:  "exchange",
	})

	wallets, err := client.CreateUserWallet(context.Background(), " user-1 ")
	if err != nil {
		t.Fatalf("CreateUserWallet: %v", err)
	}
	if captured.MerchantID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("merchant_id = %q", captured.MerchantID)
	}
	if captured.DomainID != "550e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("domain_id = %q", captured.DomainID)
	}
	if captured.ProductID != captured.DomainID {
		t.Fatalf("product_id = %q", captured.ProductID)
	}
	if captured.UserID != "user-1" {
		t.Fatalf("user_id = %q", captured.UserID)
	}
	if len(wallets) != 2 {
		t.Fatalf("wallets = %v, want two populated addresses", wallets)
	}
}
