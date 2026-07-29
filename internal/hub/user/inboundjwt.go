package user

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const (
	InboundTokenHeader = "X-Profile-Token"
)

// InboundJWTConfig controls inbound authentication with a Halo HS256 JWT.
type InboundJWTConfig struct {
	Secret string
}

func InboundJWTConfigFromEnv() InboundJWTConfig {
	return InboundJWTConfig{
		Secret: strings.TrimSpace(os.Getenv("KNSIGHT_INBOUND_JWT_SECRET")),
	}
}

func (c InboundJWTConfig) Enabled() bool {
	return c.Secret != ""
}

// InboundJWTMiddleware authenticates requests carrying X-Profile-Token and bypasses
// the browser/AP authentication chain after successful verification. Requests
// without the header continue through the existing authentication chain.
func InboundJWTMiddleware(c InboundJWTConfig, bypassNext http.Handler, authNext http.Handler) http.Handler {
	if !c.Enabled() {
		return authNext
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := strings.TrimSpace(r.Header.Get(InboundTokenHeader))
		if rawToken == "" {
			authNext.ServeHTTP(w, r)
			return
		}

		payload := struct {
			User string `json:"user"`
			jwt.RegisteredClaims
		}{}
		token, err := jwt.ParseWithClaims(rawToken, &payload, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method %q", token.Method.Alg())
			}
			return []byte(c.Secret), nil
		})
		if err != nil || !token.Valid {
			log.Printf("[user/inbound-jwt] verification failed (path=%s): %v", r.URL.Path, err)
			writeInboundJWTForbidden(w)
			return
		}
		if strings.TrimSpace(payload.User) == "" {
			log.Printf("[user/inbound-jwt] verification failed (path=%s): empty user", r.URL.Path)
			writeInboundJWTForbidden(w)
			return
		}

		ctx := WithContext(r.Context(), &Info{
			ID:     strings.TrimSpace(payload.User),
			Issuer: payload.Issuer,
		})
		bypassNext.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeInboundJWTForbidden(w http.ResponseWriter) {
	http.Error(w, "Invalid Token.", http.StatusForbidden)
}

func LogInboundJWTConfig(c InboundJWTConfig) {
	if c.Enabled() {
		log.Printf("auth: inbound jwt enabled (header=%s)", InboundTokenHeader)
	}
}
