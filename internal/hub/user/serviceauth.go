package user

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	defaultServiceAuthHeader = "X-Knsight-Service-Token"
	defaultServiceAuthUser   = "knsight-component"
)

// ServiceAuthConfig controls machine-to-machine access that bypasses browser SSO.
// It is token-gated so an internal route can be callable without making the
// backend an unauthenticated public API.
type ServiceAuthConfig struct {
	Token  string
	Header string
	UserID string
}

func ServiceAuthConfigFromEnv() ServiceAuthConfig {
	return ServiceAuthConfig{
		Token:  strings.TrimSpace(os.Getenv("KNSIGHT_SERVICE_AUTH_TOKEN")),
		Header: firstNonEmpty(strings.TrimSpace(os.Getenv("KNSIGHT_SERVICE_AUTH_HEADER")), defaultServiceAuthHeader),
		UserID: firstNonEmpty(strings.TrimSpace(os.Getenv("KNSIGHT_SERVICE_AUTH_USER")), defaultServiceAuthUser),
	}
}

func (c ServiceAuthConfig) Enabled() bool {
	return c.Token != ""
}

func (c ServiceAuthConfig) headerName() string {
	return firstNonEmpty(c.Header, defaultServiceAuthHeader)
}

func (c ServiceAuthConfig) userID() string {
	return firstNonEmpty(c.UserID, defaultServiceAuthUser)
}

// ServiceAuthMiddleware lets trusted internal callers use a shared service token
// and skips the browser/AP auth chain when the token matches.
func ServiceAuthMiddleware(c ServiceAuthConfig, bypassNext http.Handler, authNext http.Handler) http.Handler {
	if !c.Enabled() {
		return authNext
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimSpace(r.Header.Get(c.headerName()))
		if constantTimeEqual(got, c.Token) {
			ctx := WithContext(r.Context(), &Info{ID: c.userID()})
			bypassNext.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		authNext.ServeHTTP(w, r)
	})
}

func LogServiceAuthConfig(c ServiceAuthConfig) {
	if c.Enabled() {
		log.Printf("auth: service token enabled (header=%s user=%s)", c.headerName(), c.userID())
	}
}

func constantTimeEqual(a, b string) bool {
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
