package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"knsight-go/internal/hub/user"
)

// TokenManager manages per-user tokens for MCP services with in-memory cache.
type TokenManager struct {
	mu           sync.RWMutex
	cache        map[string]*cachedToken // key: "username:mcpName"
	signTokenURL string                  // optional provider token endpoint
	masterKey    string                  // Authorization Bearer token for sign-token API
	ttl          time.Duration           // token validity; default = 2/3 of configured timeout
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

func NewTokenManager(signTokenURL, masterKey string, ttl time.Duration) *TokenManager {
	return &TokenManager{
		cache:        make(map[string]*cachedToken),
		signTokenURL: signTokenURL,
		masterKey:    masterKey,
		ttl:          ttl,
	}
}

// GetToken returns a cached or freshly signed token for the given user.
func (tm *TokenManager) GetToken(ctx context.Context, username string) (string, error) {
	cacheKey := username

	// Check cache
	tm.mu.RLock()
	if ct, ok := tm.cache[cacheKey]; ok && time.Now().Before(ct.expiresAt) {
		tm.mu.RUnlock()
		return ct.token, nil
	}
	tm.mu.RUnlock()

	// Sign new token
	token, err := tm.signToken(ctx, username)
	if err != nil {
		return "", err
	}

	// Cache it
	tm.mu.Lock()
	tm.cache[cacheKey] = &cachedToken{
		token:     token,
		expiresAt: time.Now().Add(tm.ttl),
	}
	tm.mu.Unlock()

	log.Printf("[token] signed token for user=%s ttl=%s", username, tm.ttl)
	return token, nil
}

// HeaderFunc returns a transport.HTTPHeaderFunc that injects per-user Authorization.
// Falls back to masterKey if user is not available or sign fails.
func (tm *TokenManager) HeaderFunc(masterKey string) func(context.Context) map[string]string {
	return func(ctx context.Context) map[string]string {
		u := user.FromContext(ctx)
		if u.IsVisitor() {
			return map[string]string{"Authorization": "Bearer " + masterKey}
		}
		token, err := tm.GetToken(ctx, u.ID)
		if err != nil {
			log.Printf("[token] sign failed for user=%s, using master key: %v", u.ID, err)
			return map[string]string{"Authorization": "Bearer " + masterKey}
		}
		return map[string]string{"Authorization": "Bearer " + token}
	}
}

func (tm *TokenManager) signToken(ctx context.Context, username string) (string, error) {
	expiresAt := time.Now().Add(tm.ttl).Format("2006-01-02 15:04:05")
	body, _ := json.Marshal(map[string]string{
		"username":   username,
		"expires_at": expiresAt,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", tm.signTokenURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("sign-token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tm.masterKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sign-token call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sign-token returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"` // JWT token
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("sign-token parse: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("sign-token error: %s", result.Message)
	}
	return result.Data, nil
}
