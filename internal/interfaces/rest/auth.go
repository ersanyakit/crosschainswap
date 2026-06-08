package rest

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	appauth "exchange/internal/app/auth"

	"github.com/gofiber/fiber/v3"
)

const (
	oidcStateCookie    = "exchange_oidc_state"
	oidcSessionCookie  = "exchange_session"
	oidcRedirectCookie = "exchange_oidc_redirect"
	authClaimsLocal    = "auth.claims"
)

func (s *Server) oidcStatus(c fiber.Ctx) error {
	return okJSON(c, fiber.Map{
		"enabled":  s.auth != nil && s.auth.Enabled(),
		"provider": s.authProviderName(),
	})
}

func (s *Server) oidcLogin(c fiber.Ctx) error {
	if s.auth == nil || !s.auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(errorResponse{Error: appauth.ErrDisabled.Error()})
	}
	state, err := appauth.NewState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errorResponse{Error: err.Error()})
	}
	authURL, err := s.auth.AuthCodeURL(state)
	if err != nil {
		return authError(c, err)
	}
	s.setCookie(c, oidcStateCookie, state, 10*time.Minute)
	if redirectURL := sanitizeOIDCRedirect(c.Query("redirect")); redirectURL != "" {
		s.setCookie(c, oidcRedirectCookie, redirectURL, 10*time.Minute)
	}
	return c.Redirect().To(authURL)
}

func (s *Server) oidcCallback(c fiber.Ctx) error {
	if s.auth == nil || !s.auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(errorResponse{Error: appauth.ErrDisabled.Error()})
	}
	expectedState := c.Cookies(oidcStateCookie)
	actualState := strings.TrimSpace(c.Query("state"))
	if expectedState == "" || actualState == "" || expectedState != actualState {
		s.clearCookie(c, oidcStateCookie)
		return c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: "invalid oidc state"})
	}
	claims, err := s.auth.Exchange(c.Context(), c.Query("code"))
	if err != nil {
		s.clearCookie(c, oidcStateCookie)
		return authError(c, err)
	}
	session, err := s.auth.SignSession(*claims)
	if err != nil {
		s.clearCookie(c, oidcStateCookie)
		return authError(c, err)
	}
	s.clearCookie(c, oidcStateCookie)
	s.setCookie(c, oidcSessionCookie, session, s.auth.SessionTTL())
	redirectURL := sanitizeOIDCRedirect(c.Cookies(oidcRedirectCookie))
	s.clearCookie(c, oidcRedirectCookie)
	if redirectURL != "" {
		return c.Redirect().To(redirectURL)
	}
	return okJSON(c, fiber.Map{"authenticated": true, "user": claims})
}

func (s *Server) authMe(c fiber.Ctx) error {
	claims, err := s.requireUser(c)
	if err != nil {
		return err
	}
	if claims == nil {
		return okJSON(c, fiber.Map{"authenticated": false, "enabled": false})
	}
	return okJSON(c, fiber.Map{"authenticated": true, "user": claims})
}

func (s *Server) authLogout(c fiber.Ctx) error {
	redirectURL := sanitizeOIDCPostLogoutRedirect(c.Query("redirect"))
	s.clearCookie(c, oidcSessionCookie)
	s.clearCookie(c, oidcStateCookie)
	s.clearCookie(c, oidcRedirectCookie)
	response := fiber.Map{"authenticated": false}
	if s.auth != nil && s.auth.Enabled() {
		if logoutURL := s.auth.EndSessionURL(redirectURL); logoutURL != "" {
			response["logout_url"] = logoutURL
		}
	}
	return okJSON(c, response)
}

func (s *Server) requirePathUser(c fiber.Ctx) (string, error) {
	userID := strings.TrimSpace(c.Params("user_id"))
	claims, err := s.requireUser(c)
	if err != nil {
		return "", err
	}
	if claims == nil {
		return userID, nil
	}
	if userID != "" && userID != claims.Subject {
		return "", c.Status(fiber.StatusForbidden).JSON(errorResponse{Error: "user_id does not match authenticated subject"})
	}
	return claims.Subject, nil
}

func (s *Server) requireOrderOwner(c fiber.Ctx, userID string) error {
	claims, err := s.requireUser(c)
	if err != nil {
		return err
	}
	if claims == nil {
		return nil
	}
	if userID != claims.Subject {
		return c.Status(fiber.StatusForbidden).JSON(errorResponse{Error: "order does not belong to authenticated subject"})
	}
	return nil
}

func (s *Server) requireUser(c fiber.Ctx) (*appauth.Claims, error) {
	if s.auth == nil || !s.auth.Enabled() {
		return nil, nil
	}
	if cached, ok := c.Locals(authClaimsLocal).(*appauth.Claims); ok && cached != nil {
		return cached, nil
	}
	claims, err := s.auth.ParseSession(c.Cookies(oidcSessionCookie))
	if err != nil {
		return nil, c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: "authentication required"})
	}
	c.Locals(authClaimsLocal, claims)
	return claims, nil
}

func (s *Server) setCookie(c fiber.Ctx, name string, value string, ttl time.Duration) {
	maxAge := int(ttl.Seconds())
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  time.Now().Add(ttl),
		Secure:   s.auth != nil && s.auth.CookieSecure(),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

func (s *Server) clearCookie(c fiber.Ctx, name string) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   s.auth != nil && s.auth.CookieSecure(),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

func sanitizeOIDCRedirect(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OIDC_POST_LOGIN_REDIRECT_URL"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("APP_URL"))
	}
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return raw
	}
	for _, allowed := range strings.Split(os.Getenv("OIDC_ALLOWED_REDIRECT_ORIGINS"), ",") {
		allowedURL, err := url.Parse(strings.TrimSpace(allowed))
		if err == nil && strings.EqualFold(allowedURL.Scheme, parsed.Scheme) && strings.EqualFold(allowedURL.Host, parsed.Host) {
			return raw
		}
	}
	return ""
}

func sanitizeOIDCPostLogoutRedirect(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OIDC_POST_LOGOUT_REDIRECT_URL"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OIDC_POST_LOGIN_REDIRECT_URL"))
	}
	return sanitizeOIDCRedirect(raw)
}

func (s *Server) authProviderName() string {
	if s.auth == nil {
		return appauth.DefaultProviderName
	}
	return s.auth.ProviderName()
}

func authError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, appauth.ErrDisabled):
		return c.Status(fiber.StatusServiceUnavailable).JSON(errorResponse{Error: err.Error()})
	case errors.Is(err, appauth.ErrInvalidSession):
		return c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: err.Error()})
	default:
		return c.Status(fiber.StatusBadGateway).JSON(errorResponse{Error: err.Error()})
	}
}
