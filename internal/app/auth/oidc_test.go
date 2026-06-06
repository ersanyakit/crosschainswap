package auth

import (
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
