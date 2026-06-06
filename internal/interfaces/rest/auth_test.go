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
		{name: "localhost", raw: "http://localhost:3001/", want: "http://localhost:3001/"},
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
