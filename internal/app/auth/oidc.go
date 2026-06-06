package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	DefaultProviderName = "RESEARCHCAVE"
	DefaultClientID     = "kewlswap-exchange"
	DefaultRedirectURI  = "http://localhost:8080/auth/oidc/callback"
)

var (
	ErrDisabled       = errors.New("oidc is not configured")
	ErrInvalidSession = errors.New("invalid oidc session")
)

type Config struct {
	ProviderName  string
	IssuerURL     string
	AuthURL       string
	TokenURL      string
	UserInfoURL   string
	EndSessionURL string
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	Scopes        []string
	SessionSecret string
	SessionTTL    time.Duration
	CookieSecure  bool
}

type Service struct {
	cfg      Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   oauth2.Config
	enabled  bool
}

type Claims struct {
	Subject   string    `json:"sub"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	Roles     []string  `json:"roles,omitempty"`
	Issuer    string    `json:"iss,omitempty"`
	Audience  []string  `json:"aud,omitempty"`
	ExpiresAt time.Time `json:"exp"`
}

type tokenClaims struct {
	Subject string          `json:"sub"`
	Email   string          `json:"email"`
	Name    string          `json:"name"`
	Roles   json.RawMessage `json:"roles"`
}

func ConfigFromEnv() Config {
	return Config{
		ProviderName:  envOrDefault("OIDC_PROVIDER_NAME", DefaultProviderName),
		IssuerURL:     strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")),
		AuthURL:       strings.TrimSpace(os.Getenv("OIDC_AUTH_URL")),
		TokenURL:      strings.TrimSpace(os.Getenv("OIDC_TOKEN_URL")),
		UserInfoURL:   strings.TrimSpace(os.Getenv("OIDC_USERINFO_URL")),
		EndSessionURL: envOrDefault("OIDC_LOGOUT_URL", strings.TrimSpace(os.Getenv("OIDC_END_SESSION_URL"))),
		ClientID:      envOrDefault("OIDC_CLIENT_ID", DefaultClientID),
		ClientSecret:  strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET")),
		RedirectURI:   envOrDefault("OIDC_REDIRECT_URI", DefaultRedirectURI),
		Scopes:        scopesFromEnv("OIDC_SCOPES", []string{oidc.ScopeOpenID, "profile", "email", "roles"}),
		SessionSecret: envOrDefault("OIDC_SESSION_SECRET", strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET"))),
		SessionTTL:    durationFromEnv("OIDC_SESSION_TTL", 12*time.Hour),
		CookieSecure:  boolFromEnv("OIDC_COOKIE_SECURE", false),
	}
}

func NewOIDCService(ctx context.Context, cfg Config) (*Service, error) {
	cfg.ProviderName = strings.TrimSpace(cfg.ProviderName)
	cfg.IssuerURL = strings.TrimSpace(cfg.IssuerURL)
	cfg.AuthURL = strings.TrimSpace(cfg.AuthURL)
	cfg.TokenURL = strings.TrimSpace(cfg.TokenURL)
	cfg.UserInfoURL = strings.TrimSpace(cfg.UserInfoURL)
	cfg.EndSessionURL = strings.TrimSpace(cfg.EndSessionURL)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)
	cfg.RedirectURI = strings.TrimSpace(cfg.RedirectURI)
	cfg.SessionSecret = strings.TrimSpace(cfg.SessionSecret)
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.ProviderName == "" {
		cfg.ProviderName = DefaultProviderName
	}
	if cfg.ClientID == "" {
		cfg.ClientID = DefaultClientID
	}
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = DefaultRedirectURI
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{oidc.ScopeOpenID, "profile", "email", "roles"}
	}
	if cfg.IssuerURL == "" || cfg.ClientSecret == "" || cfg.SessionSecret == "" {
		return &Service{cfg: cfg}, nil
	}
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	if cfg.EndSessionURL == "" {
		var metadata struct {
			EndSessionEndpoint string `json:"end_session_endpoint"`
		}
		if err := provider.Claims(&metadata); err == nil {
			cfg.EndSessionURL = strings.TrimSpace(metadata.EndSessionEndpoint)
		}
	}
	endpoint := provider.Endpoint()
	if cfg.AuthURL != "" {
		endpoint.AuthURL = cfg.AuthURL
	}
	if cfg.TokenURL != "" {
		endpoint.TokenURL = cfg.TokenURL
	}
	out := &Service{
		cfg:      cfg,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth2: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURI,
			Endpoint:     endpoint,
			Scopes:       cfg.Scopes,
		},
		enabled: true,
	}
	return out, nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.enabled
}

func (s *Service) ProviderName() string {
	if s == nil || s.cfg.ProviderName == "" {
		return DefaultProviderName
	}
	return s.cfg.ProviderName
}

func (s *Service) CookieSecure() bool {
	return s != nil && s.cfg.CookieSecure
}

func (s *Service) SessionTTL() time.Duration {
	if s == nil || s.cfg.SessionTTL <= 0 {
		return 12 * time.Hour
	}
	return s.cfg.SessionTTL
}

func (s *Service) AuthCodeURL(state string) (string, error) {
	if !s.Enabled() {
		return "", ErrDisabled
	}
	return s.oauth2.AuthCodeURL(state, oauth2.AccessTypeOnline), nil
}

func (s *Service) EndSessionURL(postLogoutRedirect string) string {
	if !s.Enabled() || s.cfg.EndSessionURL == "" {
		return ""
	}
	parsed, err := url.Parse(s.cfg.EndSessionURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	query := parsed.Query()
	if s.cfg.ClientID != "" {
		query.Set("client_id", s.cfg.ClientID)
	}
	if postLogoutRedirect = strings.TrimSpace(postLogoutRedirect); postLogoutRedirect != "" {
		query.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (s *Service) Exchange(ctx context.Context, code string) (*Claims, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("%w: authorization code is required", ErrInvalidSession)
	}
	token, err := s.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("%w: id_token is missing", ErrInvalidSession)
	}
	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, err
	}
	var raw tokenClaims
	if err := idToken.Claims(&raw); err != nil {
		return nil, err
	}
	claims := &Claims{
		Subject:   raw.Subject,
		Email:     raw.Email,
		Name:      raw.Name,
		Roles:     parseRoles(raw.Roles),
		Issuer:    idToken.Issuer,
		Audience:  idToken.Audience,
		ExpiresAt: time.Now().Add(s.cfg.SessionTTL).UTC(),
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: subject is missing", ErrInvalidSession)
	}
	return claims, nil
}

func (s *Service) SignSession(claims Claims) (string, error) {
	if !s.Enabled() {
		return "", ErrDisabled
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("%w: subject is missing", ErrInvalidSession)
	}
	if claims.ExpiresAt.IsZero() {
		claims.ExpiresAt = time.Now().Add(s.cfg.SessionTTL).UTC()
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := hmacSign([]byte(s.cfg.SessionSecret), []byte(encodedPayload))
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *Service) ParseSession(value string) (*Claims, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	payload, sig, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || payload == "" || sig == "" {
		return nil, ErrInvalidSession
	}
	expected := hmacSign([]byte(s.cfg.SessionSecret), []byte(payload))
	actual, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, ErrInvalidSession
	}
	if !hmac.Equal(actual, expected) {
		return nil, ErrInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrInvalidSession
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, ErrInvalidSession
	}
	if claims.Subject == "" || time.Now().After(claims.ExpiresAt) {
		return nil, ErrInvalidSession
	}
	return &claims, nil
}

func NewState() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hmacSign(secret []byte, payload []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return mac.Sum(nil)
}

func parseRoles(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var roles []string
	if err := json.Unmarshal(raw, &roles); err == nil {
		return compactStrings(roles)
	}
	var role string
	if err := json.Unmarshal(raw, &role); err == nil {
		return compactStrings(strings.Split(role, ","))
	}
	return nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func scopesFromEnv(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	return compactStrings(strings.Split(raw, ","))
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	out, err := time.ParseDuration(raw)
	if err != nil || out <= 0 {
		return fallback
	}
	return out
}

func boolFromEnv(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
