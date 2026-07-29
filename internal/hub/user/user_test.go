package user

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSOMiddlewareAllowsVisitorWhenSSONotRequired(t *testing.T) {
	var gotUser string
	h := SSOMiddleware(false, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = FromContext(r.Context()).ID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if gotUser != defaultUserID {
		t.Fatalf("user=%q, want %q", gotUser, defaultUserID)
	}
}

func TestSSOMiddlewareUsesCookieWhenSSONotRequired(t *testing.T) {
	var gotUser string
	h := SSOMiddleware(false, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = FromContext(r.Context()).ID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "hehongge"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if gotUser != "hehongge" {
		t.Fatalf("user=%q, want hehongge", gotUser)
	}
}

func TestSSOMiddlewareUsesUserIDQueryParam(t *testing.T) {
	var gotUser string
	h := SSOMiddleware(false, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = FromContext(r.Context()).ID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat?user_id=hehongge", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if gotUser != "hehongge" {
		t.Fatalf("user=%q, want hehongge", gotUser)
	}
}

func TestSSOMiddlewareCookieTakesPriorityOverUserIDQueryParam(t *testing.T) {
	var gotUser string
	h := SSOMiddleware(false, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = FromContext(r.Context()).ID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat?user_id=query-user", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "cookie-user"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if gotUser != "cookie-user" {
		t.Fatalf("user=%q, want cookie-user", gotUser)
	}
}

func TestSSOMiddlewareRejectsUnsafeUserIDQueryParam(t *testing.T) {
	var gotUser string
	h := SSOMiddleware(false, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = FromContext(r.Context()).ID
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat?user_id=../../admin", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotUser != defaultUserID {
		t.Fatalf("user=%q, want %q", gotUser, defaultUserID)
	}
}
