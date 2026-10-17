package sealed

import (
	"errors"
	"strings"
	"testing"
)

const master = "0123456789abcdef0123456789abcdef"

func TestSealed_ASecretComesBackForTheSamePurposeAndOnlyThen(t *testing.T) {
	s, err := Seal(master, "mail", "p4ssw0rd")
	if err != nil || strings.Contains(s, "p4ssw0rd") || !strings.HasPrefix(s, "v1:") {
		t.Fatalf("%q %v", s, err)
	}
	if got, err := Open(master, "mail", s); err != nil || got != "p4ssw0rd" {
		t.Errorf("%q %v", got, err)
	}
	if _, err := Open(master, "wiki", s); !errors.Is(err, ErrUnreadable) {
		t.Errorf("another purpose: %v", err)
	}
	if _, err := Open(master+"x", "mail", s); !errors.Is(err, ErrUnreadable) {
		t.Errorf("another master key: %v", err)
	}
	for _, bad := range []string{"", "v1:", "v1:!!!", "plain", "v1:" + strings.Repeat("A", 8)} {
		if _, err := Open(master, "mail", bad); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestSealed_WithoutAMasterKeyNothingIsKept(t *testing.T) {
	if _, err := Seal("short", "mail", "x"); !errors.Is(err, ErrNoMasterKey) {
		t.Errorf("%v", err)
	}
	if _, err := Open("", "mail", "v1:AAAA"); !errors.Is(err, ErrNoMasterKey) {
		t.Errorf("%v", err)
	}
}

func TestSealed_TwoSealsOfTheSameSecretDiffer(t *testing.T) {
	a, _ := Seal(master, "mail", "x")
	b, _ := Seal(master, "mail", "x")
	if a == b {
		t.Error("the same nonce twice")
	}
}
