package user

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Profile holds enriched user information from the profile API.
type Profile struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Email       string `json:"email,omitempty"`
}

// ProfileAPIBaseURL can point to an optional user-profile service.
var ProfileAPIBaseURL = strings.TrimSpace(os.Getenv("KNSIGHT_PROFILE_API_URL"))

type profileUser struct {
	AdUserID  string   `json:"adUserId"`
	WxUserID  string   `json:"wxUserId"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Avatar    string   `json:"avatar"`
	DeptNames []string `json:"deptNames"`
}

// ProfileCache caches user profiles to avoid repeated API calls.
type ProfileCache struct {
	mu           sync.RWMutex
	entries      map[string]profileEntry
	client       *http.Client
	ttl          time.Duration
	profileToken string // X-Profile-Token for service-to-service auth
}

type profileEntry struct {
	profile   Profile
	fetchedAt time.Time
}

// NewProfileCache creates a new profile cache with the given TTL.
// If profileToken is non-empty, it will be used as X-Profile-Token for profile API auth
// instead of forwarding cookies from the incoming request.
func NewProfileCache(ttl time.Duration, profileToken string) *ProfileCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &ProfileCache{
		entries: make(map[string]profileEntry),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		ttl:          ttl,
		profileToken: profileToken,
	}
}

// SetFromAP seeds the cache with profile data taken directly from a
// verified AccessProxy identity-passthrough token. The JWT already carries
// the display name and avatar URL, so this avoids a profile-service round-trip on the
// hot path of every request.
func (c *ProfileCache) SetFromAP(username, name, avatar string) {
	if username == "" || username == defaultUserID {
		return
	}
	p := Profile{
		ID:          username,
		DisplayName: name,
		AvatarURL:   avatar,
	}
	if p.DisplayName == "" {
		p.DisplayName = username
	}
	c.mu.Lock()
	c.entries[username] = profileEntry{profile: p, fetchedAt: time.Now()}
	c.mu.Unlock()
}

// GetProfile returns the profile for the given user ID.
// It fetches from the profile API on cache miss and caches the result.
// The incoming HTTP request is used to forward cookies for auth.
func (c *ProfileCache) GetProfile(userID string, r *http.Request) Profile {
	if userID == "" || userID == "visitor" {
		return Profile{ID: userID, DisplayName: "Visitor"}
	}

	// Check cache
	c.mu.RLock()
	if entry, ok := c.entries[userID]; ok && time.Since(entry.fetchedAt) < c.ttl {
		c.mu.RUnlock()
		return entry.profile
	}
	c.mu.RUnlock()

	// Fetch from profile API, forwarding cookies from the incoming request
	profile := c.fetchProfile(userID, r)

	// Store in cache
	c.mu.Lock()
	c.entries[userID] = profileEntry{profile: profile, fetchedAt: time.Now()}
	c.mu.Unlock()

	return profile
}

func (c *ProfileCache) fetchProfile(userID string, incomingReq *http.Request) Profile {
	fallback := Profile{ID: userID, DisplayName: userID}

	if ProfileAPIBaseURL == "" {
		return fallback
	}
	apiURL := fmt.Sprintf("%s?username=%s", ProfileAPIBaseURL, userID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fallback
	}

	// Set browser-like User-Agent to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	// Use X-Profile-Token if configured (service-to-service auth), otherwise forward cookies
	if c.profileToken != "" {
		req.Header.Set("X-Profile-Token", c.profileToken)
	} else if incomingReq != nil {
		for _, cookie := range incomingReq.Cookies() {
			req.AddCookie(cookie)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[user/profile] profile request error: %v", err)
		return fallback
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[user/profile] read body error: %v", err)
		return fallback
	}
	log.Printf("[user/profile] profile response status=%d body=%s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return fallback
	}

	var users []profileUser
	if err := json.Unmarshal(body, &users); err != nil {
		log.Printf("[user/profile] json decode error: %v", err)
		return fallback
	}

	// Find exact match by adUserId
	var matched *profileUser
	for i := range users {
		if users[i].AdUserID == userID {
			matched = &users[i]
			break
		}
	}
	if matched == nil {
		return fallback
	}

	profile := Profile{
		ID:    userID,
		Email: matched.Email,
	}
	if matched.Name != "" {
		profile.DisplayName = matched.Name
	} else {
		profile.DisplayName = userID
	}
	if matched.Avatar != "" {
		profile.AvatarURL = matched.Avatar
	}

	return profile
}
