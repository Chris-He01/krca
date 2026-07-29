package user

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestInboundJWTMiddlewareAuthenticatesUser(t *testing.T) {
	cfg := InboundJWTConfig{Secret: "secret"}
	token := signInboundJWTForTest(t, cfg.Secret, "test-user", "local-dev", time.Now().Add(time.Hour))

	var gotUser string
	var gotIssuer string
	bypass := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := FromContext(r.Context())
		gotUser = u.ID
		gotIssuer = u.Issuer
		w.WriteHeader(http.StatusOK)
	})
	auth := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("existing auth chain should be bypassed")
	})

	h := InboundJWTMiddleware(cfg, bypass, auth)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req.Header.Set(InboundTokenHeader, token)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if gotUser != "test-user" {
		t.Fatalf("user=%q, want test-user", gotUser)
	}
	if gotIssuer != "local-dev" {
		t.Fatalf("issuer=%q, want local-dev", gotIssuer)
	}
}

func TestInboundJWTConfigFromEnvIsDisabledWithoutSecret(t *testing.T) {
	t.Setenv("KNSIGHT_INBOUND_JWT_SECRET", "")

	cfg := InboundJWTConfigFromEnv()

	if cfg.Secret != "" || cfg.Enabled() {
		t.Fatal("expected inbound JWT auth to be disabled")
	}
}

func TestInboundJWTConfigFromEnvOverridesDefaultSecret(t *testing.T) {
	t.Setenv("KNSIGHT_INBOUND_JWT_SECRET", "override")

	cfg := InboundJWTConfigFromEnv()

	if cfg.Secret != "override" {
		t.Fatal("expected environment override")
	}
}

func TestInboundJWTMiddlewareFallsThroughWithoutHeader(t *testing.T) {
	authCalled := false
	h := InboundJWTMiddleware(
		InboundJWTConfig{Secret: "secret"},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("bypass chain should not be called")
		}),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			authCalled = true
			w.WriteHeader(http.StatusUnauthorized)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !authCalled {
		t.Fatal("expected existing auth chain to be called")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestInboundJWTMiddlewareRejectsInvalidTokenWithoutFallback(t *testing.T) {
	authCalled := false
	h := InboundJWTMiddleware(
		InboundJWTConfig{Secret: "secret"},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("bypass chain should not be called")
		}),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			authCalled = true
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set(InboundTokenHeader, "not-a-jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if authCalled {
		t.Fatal("invalid inbound JWT must not fall back to another identity")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestInboundJWTMiddlewareRejectsExpiredToken(t *testing.T) {
	cfg := InboundJWTConfig{Secret: "secret"}
	token := signInboundJWTForTest(t, cfg.Secret, "test-user", "local-dev", time.Now().Add(-time.Minute))

	h := InboundJWTMiddleware(cfg, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("expired token reached application handler")
	}), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("expired token fell back to auth chain")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set(InboundTokenHeader, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestInboundJWTMiddlewareRejectsMissingUser(t *testing.T) {
	cfg := InboundJWTConfig{Secret: "secret"}
	token := signInboundJWTForTest(t, cfg.Secret, "", "local-dev", time.Now().Add(time.Hour))

	h := InboundJWTMiddleware(cfg, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("token without user reached application handler")
	}), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("token without user fell back to auth chain")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set(InboundTokenHeader, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusForbidden)
	}
}

func signInboundJWTForTest(t *testing.T, secret, username, issuer string, expires time.Time) string {
	t.Helper()
	claims := struct {
		User string `json:"user"`
		jwt.RegisteredClaims
	}{
		User: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
