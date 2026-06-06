package auth

import (
	"net/url"
	"testing"
	"time"
)

func TestSignAndParseSession(t *testing.T) {
	service, err := NewOIDCService(t.Context(), Config{
		ProviderName:  "test",
		IssuerURL:     "",
		ClientID:      "client",
		ClientSecret:  "secret",
		RedirectURI:   "http://localhost/callback",
		SessionSecret: "session-secret",
		SessionTTL:    time.Hour,
		CookieSecure:  false,
		Scopes:        []string{"openid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.enabled = true

	raw, err := service.SignSession(Claims{Subject: "user-1", Email: "u@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ParseSession(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.Email != "u@example.com" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if _, err := service.ParseSession(raw + "x"); err == nil {
		t.Fatalf("expected tampered session to fail")
	}
}

func TestParseRoles(t *testing.T) {
	roles := parseRoles([]byte(`["admin"," trader ","admin"]`))
	if len(roles) != 2 || roles[0] != "admin" || roles[1] != "trader" {
		t.Fatalf("unexpected roles: %#v", roles)
	}
}

func TestConfigFromEnvReadsExplicitEndpoints(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "https://login.researchcave.com/")
	t.Setenv("OIDC_AUTH_URL", "https://login.researchcave.com/connect/authorize")
	t.Setenv("OIDC_TOKEN_URL", "https://login.researchcave.com/connect/token")
	t.Setenv("OIDC_USERINFO_URL", "https://login.researchcave.com/connect/userinfo")
	t.Setenv("OIDC_LOGOUT_URL", "https://login.researchcave.com/connect/logout")

	cfg := ConfigFromEnv()
	if cfg.IssuerURL != "https://login.researchcave.com/" {
		t.Fatalf("unexpected issuer url: %q", cfg.IssuerURL)
	}
	if cfg.AuthURL != "https://login.researchcave.com/connect/authorize" {
		t.Fatalf("unexpected auth url: %q", cfg.AuthURL)
	}
	if cfg.TokenURL != "https://login.researchcave.com/connect/token" {
		t.Fatalf("unexpected token url: %q", cfg.TokenURL)
	}
	if cfg.UserInfoURL != "https://login.researchcave.com/connect/userinfo" {
		t.Fatalf("unexpected userinfo url: %q", cfg.UserInfoURL)
	}
	if cfg.EndSessionURL != "https://login.researchcave.com/connect/logout" {
		t.Fatalf("unexpected logout url: %q", cfg.EndSessionURL)
	}
}

func TestEndSessionURL(t *testing.T) {
	service := &Service{
		cfg: Config{
			ClientID:      "kewlswap-exchange",
			EndSessionURL: "https://login.researchcave.com/connect/logout",
		},
		enabled: true,
	}

	raw := service.EndSessionURL("http://localhost:3001/")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.String(); got == "" {
		t.Fatal("expected end session url")
	}
	query := parsed.Query()
	if query.Get("client_id") != "kewlswap-exchange" {
		t.Fatalf("unexpected client_id: %q", query.Get("client_id"))
	}
	if query.Get("post_logout_redirect_uri") != "http://localhost:3001/" {
		t.Fatalf("unexpected post logout redirect: %q", query.Get("post_logout_redirect_uri"))
	}
}
