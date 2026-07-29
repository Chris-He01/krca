package user

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// AccessProxyTokenHeader is the request header AccessProxy uses to forward
// the identity-passthrough JWT to upstream services.
const AccessProxyTokenHeader = "X-Identity-Token"

// DefaultJwksURL is the public JWKS endpoint published by AccessProxy for
// verifying identity-passthrough tokens.
const DefaultJwksURL = ""

type AccessProxyPrincipal struct {
	Username string
	Name     string
	Avatar   string
}

type AccessProxyClient interface {
	VerifyToken(token, remoteAddr string) (*AccessProxyPrincipal, error)
}

type hmacAccessProxyClient struct {
	secret []byte
}

func (c *hmacAccessProxyClient) VerifyToken(rawToken, _ string) (*AccessProxyPrincipal, error) {
	claims := struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		Avatar   string `json:"avatar"`
		jwt.RegisteredClaims
	}{}
	token, err := jwt.ParseWithClaims(rawToken, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %q", token.Method.Alg())
		}
		return c.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid identity token")
	}
	if claims.Username == "" {
		claims.Username = claims.Subject
	}
	return &AccessProxyPrincipal{Username: claims.Username, Name: claims.Name, Avatar: claims.Avatar}, nil
}

// AccessProxyConfig configures the generic identity-token client.
//
// JwksURL is the only required field; defaults to DefaultJwksURL when empty.
// VerifyIss should be true only when this service is AccessProxy's first hop
// (no intermediate gateway/proxy). TrustedHosts restricts accepted token
// `host` claims; when empty, any host claim is allowed.
type AccessProxyConfig struct {
	JwksURL          string
	PublicHost       string
	VerifyIss        bool
	TrustedHosts     []string
	TrustedUpstreams []int64
}

// NewAccessProxyClient initializes a identity-token client. A non-nil error usually
// means JWKS bootstrap failed (e.g. DNS or network issue) — callers may
// choose to fail fast or fall back depending on deployment requirements.
func NewAccessProxyClient(_ AccessProxyConfig) (AccessProxyClient, error) {
	secret := strings.TrimSpace(os.Getenv("KNSIGHT_IDENTITY_TOKEN_SECRET"))
	if secret == "" {
		return nil, fmt.Errorf("KNSIGHT_IDENTITY_TOKEN_SECRET is required for accessproxy mode")
	}
	return &hmacAccessProxyClient{secret: []byte(secret)}, nil
}

// AccessProxyMiddleware verifies the AccessProxy identity-passthrough token
// from the X-Identity-Token header and injects user identity into the
// request context. When a profileCache is provided, the verified principal's
// name and avatar are seeded into it so /v1/user/me skips the profile lookup.
//
// allowFallback controls behavior when the token is missing or invalid:
//   - true (auto mode): the request continues with no user populated, and
//     the next handler (typically the cookie middleware) gets a chance.
//   - false (strict mode): the request is rejected with 401. AccessProxy
//     itself handles the SSO redirect on the browser-side refresh.
func AccessProxyMiddleware(client AccessProxyClient, profileCache *ProfileCache, allowFallback bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health/static endpoints (matches SSOMiddleware behavior).
		if isPublicPath(r.URL.Path) {
			ctx := WithContext(r.Context(), &Info{ID: defaultUserID})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		token := r.Header.Get(AccessProxyTokenHeader)
		if token == "" {
			if allowFallback {
				next.ServeHTTP(w, r)
				return
			}
			writeAccessProxyUnauthorized(w, r, "ap-token missing")
			return
		}

		principal, err := client.VerifyToken(token, r.RemoteAddr)
		if err != nil {
			log.Printf("[user/ap] verify token failed (path=%s remote=%s): %v", r.URL.Path, r.RemoteAddr, err)
			if allowFallback {
				next.ServeHTTP(w, r)
				return
			}
			writeAccessProxyUnauthorized(w, r, "ap-token invalid")
			return
		}
		if principal == nil || principal.Username == "" {
			if allowFallback {
				next.ServeHTTP(w, r)
				return
			}
			writeAccessProxyUnauthorized(w, r, "ap-token empty")
			return
		}

		if profileCache != nil {
			profileCache.SetFromAP(principal.Username, principal.Name, principal.Avatar)
		}

		ctx := WithContext(r.Context(), &Info{ID: principal.Username})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeAccessProxyUnauthorized(w http.ResponseWriter, r *http.Request, reason string) {
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":  "authentication required (refresh page to re-login via AccessProxy)",
			"reason": reason,
		})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("登录状态已失效，请刷新页面重新登录。\nAuthentication required: please refresh the page so AccessProxy can redirect you to SSO.\n"))
}

func isPublicPath(p string) bool {
	if p == "/healthz" || p == "/api/healthz" {
		return true
	}
	// /v1/tools/* are component-to-component HTTP tools (e.g. CK gateway
	// proxy). They are authenticated upstream by gateway-side Ks-Auth-*
	// headers held by the hub itself, so we deliberately do not layer an
	// extra browser-SSO/AccessProxy check on top — that would block agents
	// and trusted service callers that arrive without an
	// X-Identity-Token. Anyone inside the cluster network can reach them.
	if strings.HasPrefix(p, "/v1/tools/") {
		return true
	}
	return strings.HasPrefix(p, "/_next/") || strings.HasPrefix(p, "/favicon")
}

// AuthMode is the authentication backend selector.
//   - AuthModeDisabled: no enforcement; all requests pass through as visitor.
//   - AuthModeCookie:   legacy knsight_user_id cookie + SSO redirect.
//   - AuthModeAccessProxy: AccessProxy identity-passthrough JWT (strict).
//   - AuthModeAuto: try AccessProxy first; fall back to cookie when token is absent or invalid.
type AuthMode string

const (
	AuthModeDisabled    AuthMode = "disabled"
	AuthModeCookie      AuthMode = "cookie"
	AuthModeAccessProxy AuthMode = "accessproxy"
	AuthModeAuto        AuthMode = "auto"
)

// ResolveAuthMode normalizes user-supplied mode strings and applies env
// overrides. Order of precedence: env KNSIGHT_AUTH_MODE > configMode > legacy
// SSORequired toggle > disabled. Returns one of the AuthMode* constants.
func ResolveAuthMode(configMode string, ssoRequired bool) AuthMode {
	if v := os.Getenv("KNSIGHT_AUTH_MODE"); v != "" {
		configMode = v
	}
	switch strings.ToLower(strings.TrimSpace(configMode)) {
	case string(AuthModeAccessProxy), "ap":
		return AuthModeAccessProxy
	case string(AuthModeAuto):
		return AuthModeAuto
	case string(AuthModeCookie), "sso":
		return AuthModeCookie
	case string(AuthModeDisabled), "off", "none", "false":
		return AuthModeDisabled
	}
	// Back-compat: legacy `sso_required: true` implies cookie mode.
	if ssoRequired {
		return AuthModeCookie
	}
	return AuthModeDisabled
}

// IsAuthEnabled reports whether the master auth toggle is on.
// Order of precedence: env KNSIGHT_AUTH_ENABLED > configEnabled.
// When set to "0" / "false", auth enforcement is bypassed regardless of mode.
func IsAuthEnabled(configEnabled *bool) bool {
	if v := os.Getenv("KNSIGHT_AUTH_ENABLED"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "0", "false", "no", "off":
			return false
		case "1", "true", "yes", "on":
			return true
		}
	}
	if configEnabled == nil {
		return true // default on; mode controls actual behavior
	}
	return *configEnabled
}
