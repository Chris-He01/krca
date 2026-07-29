package user

import (
	"testing"
)

func TestResolveAuthMode(t *testing.T) {
	cases := []struct {
		name        string
		configMode  string
		ssoRequired bool
		envMode     string
		want        AuthMode
	}{
		{"empty defaults to disabled", "", false, "", AuthModeDisabled},
		{"legacy sso_required => cookie", "", true, "", AuthModeCookie},
		{"explicit accessproxy", "accessproxy", false, "", AuthModeAccessProxy},
		{"explicit auto", "auto", false, "", AuthModeAuto},
		{"alias sso", "sso", false, "", AuthModeCookie},
		{"explicit disabled wins over sso_required", "disabled", true, "", AuthModeDisabled},
		{"env override beats config", "cookie", false, "accessproxy", AuthModeAccessProxy},
		{"env disabled beats config", "accessproxy", false, "off", AuthModeDisabled},
		{"unknown mode falls back to legacy sso_required path", "weird", true, "", AuthModeCookie},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KNSIGHT_AUTH_MODE", tc.envMode)
			got := ResolveAuthMode(tc.configMode, tc.ssoRequired)
			if got != tc.want {
				t.Fatalf("ResolveAuthMode(%q, %v) [env=%q] = %q, want %q",
					tc.configMode, tc.ssoRequired, tc.envMode, got, tc.want)
			}
		})
	}
}

func TestIsAuthEnabled(t *testing.T) {
	t.Run("nil config defaults true", func(t *testing.T) {
		t.Setenv("KNSIGHT_AUTH_ENABLED", "")
		if !IsAuthEnabled(nil) {
			t.Fatal("expected default true")
		}
	})
	t.Run("explicit false", func(t *testing.T) {
		t.Setenv("KNSIGHT_AUTH_ENABLED", "")
		f := false
		if IsAuthEnabled(&f) {
			t.Fatal("expected false")
		}
	})
	t.Run("env false overrides config true", func(t *testing.T) {
		t.Setenv("KNSIGHT_AUTH_ENABLED", "false")
		tr := true
		if IsAuthEnabled(&tr) {
			t.Fatal("expected env to win")
		}
	})
	t.Run("env true overrides config false", func(t *testing.T) {
		t.Setenv("KNSIGHT_AUTH_ENABLED", "1")
		f := false
		if !IsAuthEnabled(&f) {
			t.Fatal("expected env to win")
		}
	})
}

func TestSetFromAPSeedsCacheBypassingHalo(t *testing.T) {
	c := NewProfileCache(0, "")
	c.SetFromAP("zhangsan01", "张三", "https://example.com/a.jpg")

	// GetProfile should return the seeded entry without making an HTTP call.
	// We pass a nil *http.Request — fetchProfile would panic when trying to
	// call r.Cookies(), so a successful return implies the cache was hit.
	got := c.GetProfile("zhangsan01", nil)
	if got.ID != "zhangsan01" || got.DisplayName != "张三" || got.AvatarURL != "https://example.com/a.jpg" {
		t.Fatalf("unexpected profile: %+v", got)
	}
}

func TestSetFromAPIgnoresVisitor(t *testing.T) {
	c := NewProfileCache(0, "")
	c.SetFromAP("visitor", "Visitor", "")
	c.SetFromAP("", "Anonymous", "")
	if len(c.entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(c.entries))
	}
}
