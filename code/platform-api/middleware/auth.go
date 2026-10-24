package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// Principal is the authenticated caller.
type Principal struct {
	Subject string
	Groups  []string
}

// TokenVerifier turns a raw bearer token into a Principal.
//
// It is the seam for OIDC: wrap go-oidc's *oidc.IDTokenVerifier, call
// Verify(ctx, raw), then map the "sub" and "groups" claims onto a Principal.
// StaticTokens below is a simple implementation for labs and tests.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*Principal, error)
}

// TokenVerifierFunc adapts a function to TokenVerifier.
type TokenVerifierFunc func(ctx context.Context, rawToken string) (*Principal, error)

// Verify implements TokenVerifier.
func (f TokenVerifierFunc) Verify(ctx context.Context, raw string) (*Principal, error) {
	return f(ctx, raw)
}

// ErrInvalidToken is returned by verifiers for unknown or expired tokens.
var ErrInvalidToken = errors.New("invalid token")

// StaticTokens is a TokenVerifier backed by a fixed token→principal table.
// Tokens are compared in constant time.
type StaticTokens map[string]Principal

// Verify implements TokenVerifier.
func (s StaticTokens) Verify(_ context.Context, raw string) (*Principal, error) {
	var match *Principal
	for tok, p := range s {
		if subtle.ConstantTimeCompare([]byte(tok), []byte(raw)) == 1 {
			p := p
			match = &p
		}
	}
	if match == nil {
		return nil, ErrInvalidToken
	}
	return match, nil
}

// AuthConfig configures the Auth middleware.
type AuthConfig struct {
	Verifier TokenVerifier
	// PublicPaths are served without authentication (exact match, or prefix
	// match when the entry ends in "/").
	PublicPaths []string
}

func (c AuthConfig) isPublic(path string) bool {
	for _, p := range c.PublicPaths {
		if path == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
			return true
		}
	}
	return false
}

// Auth requires a valid "Authorization: Bearer <token>" header on every
// non-public path. A nil Verifier disables authentication (local dev only).
func Auth(cfg AuthConfig) Middleware {
	return func(next http.Handler) http.Handler {
		if cfg.Verifier == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions || cfg.isPublic(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="platform-api"`)
				WriteError(w, r, http.StatusUnauthorized, "Unauthorized", "missing bearer token")
				return
			}
			p, err := cfg.Verifier.Verify(r.Context(), strings.TrimSpace(token))
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="platform-api", error="invalid_token"`)
				WriteError(w, r, http.StatusUnauthorized, "Unauthorized", "invalid bearer token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}
