package rest

import "testing"

func TestSanitizeOIDCRedirect(t *testing.T) {
	t.Setenv("OIDC_ALLOWED_REDIRECT_ORIGINS", "https://exchange.example")

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "relative", raw: "/", want: "/"},
		{name: "localhost", raw: "http://localhost:3002/", want: "http://localhost:3002/"},
		{name: "allowed origin", raw: "https://exchange.example/terminal", want: "https://exchange.example/terminal"},
		{name: "protocol relative rejected", raw: "//evil.example", want: ""},
		{name: "external rejected", raw: "https://evil.example", want: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeOIDCRedirect(tt.raw); got != tt.want {
				t.Fatalf("sanitizeOIDCRedirect(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDecodeMarketSymbol(t *testing.T) {
	if got := decodeMarketSymbol("PEPPER%2FUSD"); got != "PEPPER/USD" {
		t.Fatalf("decodeMarketSymbol encoded = %q, want PEPPER/USD", got)
	}
	if got := decodeMarketSymbol("PEPPER/USD"); got != "PEPPER/USD" {
		t.Fatalf("decodeMarketSymbol raw = %q, want PEPPER/USD", got)
	}
}

func TestSanitizeOIDCPostLogoutRedirect(t *testing.T) {
	t.Setenv("OIDC_POST_LOGOUT_REDIRECT_URL", "http://localhost:3002/")
	if got := sanitizeOIDCPostLogoutRedirect(""); got != "http://localhost:3002/" {
		t.Fatalf("sanitizeOIDCPostLogoutRedirect default = %q, want localhost frontend", got)
	}
	if got := sanitizeOIDCPostLogoutRedirect("https://evil.example"); got != "" {
		t.Fatalf("sanitizeOIDCPostLogoutRedirect external = %q, want empty", got)
	}
}
