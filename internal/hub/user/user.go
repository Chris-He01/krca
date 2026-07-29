package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// Info holds the current user identity.
type Info struct {
	ID     string `json:"id"` // e.g. "lvbo" from knsight_user_id cookie
	Issuer string `json:"issuer,omitempty"`
}

// IsVisitor returns true if the user is not authenticated.
func (u *Info) IsVisitor() bool {
	return u.ID == "" || u.ID == "visitor"
}

const defaultUserID = "visitor"

type ctxKey struct{}

// FromContext extracts user info from context. Returns visitor if not set.
func FromContext(ctx context.Context) *Info {
	if u, ok := ctx.Value(ctxKey{}).(*Info); ok {
		return u
	}
	return &Info{ID: defaultUserID}
}

// WithContext stores user info in context.
func WithContext(ctx context.Context, u *Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

const (
	// CookieName is the SSO cookie used for user identification.
	CookieName = "knsight_user_id"
	// UserIDQueryParam is the fallback query parameter for trusted internal callers.
	UserIDQueryParam = "user_id"
)

var validUserID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// DefaultSSOURL is the default SSO login endpoint.
const DefaultSSOURL = ""

// Middleware extracts user identity from cookie, then the user_id query
// parameter, and injects it into context. If both are missing, the user
// defaults to "visitor".
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := requestUserID(r)
		ctx := WithContext(r.Context(), &Info{ID: uid})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SSOMiddleware wraps Middleware with SSO redirect enforcement.
// When ssoRequired=true and the user has no valid cookie:
//   - API requests (/v1/*) get a 401 JSON with a redirect URL
//   - Page requests get a 302 redirect to SSO
//
// When ssoRequired=false, unauthenticated users pass through as "visitor" (same as Middleware).
func SSOMiddleware(ssoRequired bool, ssoURL string, next http.Handler) http.Handler {
	if !ssoRequired {
		return Middleware(next)
	}
	if ssoURL == "" {
		ssoURL = DefaultSSOURL
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip SSO for health checks, static assets and component-to-
		// component tool endpoints (e.g. /v1/tools/ck/*). See the
		// matching note in isPublicPath (accessproxy.go) for the rationale.
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/healthz" ||
			strings.HasPrefix(r.URL.Path, "/_next/") || strings.HasPrefix(r.URL.Path, "/favicon") ||
			strings.HasPrefix(r.URL.Path, "/v1/tools/") {
			uid := defaultUserID
			ctx := WithContext(r.Context(), &Info{ID: uid})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		uid := requestUserID(r)

		if uid == defaultUserID {
			// No valid cookie — enforce SSO
			currentURL := requestURL(r)
			redirectURL := ssoURL + "?service=" + url.QueryEscape(currentURL)

			if isAPIRequest(r) {
				// API: return 401 JSON with redirect URL
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":    "authentication required",
					"redirect": redirectURL,
				})
				return
			}
			// Page: HTTP 302 redirect to SSO
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		ctx := WithContext(r.Context(), &Info{ID: uid})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestUserID(r *http.Request) string {
	if c, err := r.Cookie(CookieName); err == nil {
		if uid := strings.TrimSpace(c.Value); validUserID.MatchString(uid) {
			return uid
		}
	}
	if uid := strings.TrimSpace(r.URL.Query().Get(UserIDQueryParam)); validUserID.MatchString(uid) {
		return uid
	}
	return defaultUserID
}

// IsSSORequired reads the KNSIGHT_SSO_REQUIRED env var. Returns false if not set.
func IsSSORequired() bool {
	v := os.Getenv("KNSIGHT_SSO_REQUIRED")
	return v == "true" || v == "1"
}

// GetSSOURL reads KNSIGHT_SSO_URL env var, falling back to DefaultSSOURL.
func GetSSOURL() string {
	if v := os.Getenv("KNSIGHT_SSO_URL"); v != "" {
		return v
	}
	return DefaultSSOURL
}

func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/v1/")
}

func requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	return scheme + "://" + r.Host + r.RequestURI
}

// UserScopePrefix returns the skill/memory scope prefix for a user.
// e.g. "user/lvbo"
func UserScopePrefix(userID string) string {
	return "user/" + userID
}
