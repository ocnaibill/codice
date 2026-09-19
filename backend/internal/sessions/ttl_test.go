package sessions

import (
	"testing"
	"time"
)

func TestTTL(t *testing.T) {
	cases := map[string]time.Duration{
		"":       7 * 24 * time.Hour, // default
		"24":     24 * time.Hour,
		"1":      time.Hour,
		"0":      7 * 24 * time.Hour, // invalid values fall back to the default
		"-5":     7 * 24 * time.Hour,
		"abc":    7 * 24 * time.Hour,
		"12.5":   7 * 24 * time.Hour,
		" 48 ":   7 * 24 * time.Hour,
		"168":    7 * 24 * time.Hour,
		"720":    720 * time.Hour,
		"999999": 7 * 24 * time.Hour, // absurd values are refused rather than trusted
	}
	for env, want := range cases {
		t.Setenv("JWT_EXPIRATION_HOURS", env)
		if got := TTL(); got != want {
			t.Errorf("JWT_EXPIRATION_HOURS=%q: TTL() = %v, want %v", env, got, want)
		}
	}
}
