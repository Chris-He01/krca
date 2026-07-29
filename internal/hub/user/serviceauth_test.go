package user

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceAuthMiddlewareBypassesAuthWithDefaultIdentity(t *testing.T) {
	var gotUser string
	bypass := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = FromContext(r.Context()).ID
		w.WriteHeader(http.StatusOK)
	})
	auth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("auth chain should not be called when service token matches")
	})

	h := ServiceAuthMiddleware(ServiceAuthConfig{Token: "secret"}, bypass, auth)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req.Header.Set(defaultServiceAuthHeader, "secret")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rr.Code, http.StatusOK)
	}
	if gotUser != defaultServiceAuthUser {
		t.Fatalf("user=%q, want %q", gotUser, defaultServiceAuthUser)
	}
}

func TestServiceAuthMiddlewareFallsThroughOnBadToken(t *testing.T) {
	authCalled := false
	bypass := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("bypass chain should not be called when service token is wrong")
	})
	auth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalled = true
		w.WriteHeader(http.StatusUnauthorized)
	})

	h := ServiceAuthMiddleware(ServiceAuthConfig{Token: "secret"}, bypass, auth)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req.Header.Set(defaultServiceAuthHeader, "wrong")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if !authCalled {
		t.Fatal("expected auth chain to be called")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
