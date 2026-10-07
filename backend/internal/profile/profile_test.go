package profile

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"Ana":                               "Ana",
		"  Ana   Maria  ":                   "Ana Maria",
		"Ana\nMaria\r\n":                    "Ana Maria",
		"Ana‮":                              "Ana",
		"Ana​Maria":                         "Ana Maria",
		"An\x00a":                           "An a",
		"bytes\xffinválidos":                "bytesinválidos",
		"":                                  "",
		"   \t\n ":                          "",
		"João da Conceição":                 "João da Conceição",
		"日本語の名前":                            "日本語の名前",
		strings.Repeat("ç", MaxDisplayName): strings.Repeat("ç", MaxDisplayName), // 60 characters, 120 bytes: it fits
	} {
		got, err := CleanDisplayName(in)
		if err != nil || got != want {
			t.Errorf("CleanDisplayName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestCleanDisplayName_ALongOneIsRefusedNotCut(t *testing.T) {
	for _, in := range []string{strings.Repeat("a", MaxDisplayName+1), strings.Repeat("ç", MaxDisplayName+1), strings.Repeat("日", 200)} {
		if got, err := CleanDisplayName(in); !errors.Is(err, ErrTooLong) || got != "" {
			t.Errorf("%d characters: %q, %v; want ErrTooLong", len([]rune(in)), got, err)
		}
	}
	// What counts is what is kept: the spaces that go away do not count.
	if _, err := CleanDisplayName("  " + strings.Repeat("a", MaxDisplayName) + "   "); err != nil {
		t.Errorf("a name of exactly the limit with spaces around it: %v", err)
	}
}
