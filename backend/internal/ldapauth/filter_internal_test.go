package ldapauth

import "testing"

// The filter is built from what a person types at the login form, so this is the
// place where an injection would happen. Values must always be escaped.
func TestUserFilter_TheNameCanOnlyBeAValue(t *testing.T) {
	cases := map[string]string{
		"ana":          "(uid=ana)",
		"*":            `(uid=\2a)`,
		"a*":           `(uid=a\2a)`,
		"*)(uid=*":     `(uid=\2a\29\28uid=\2a)`,
		"ana)(|(uid=*": `(uid=ana\29\28|\28uid=\2a)`,
		`a\b`:          `(uid=a\5cb)`,
		"a\x00b":       `(uid=a\00b)`,
		"jo\u00e3o":    "(uid=jo\\c3\\a3o)",
	}
	for in, want := range cases {
		if got := userFilter("(uid={username})", in); got != want {
			t.Errorf("userFilter(%q) = %s, want %s", in, got, want)
		}
	}
	// The same care applies inside a larger configured filter.
	if got := userFilter("(&(objectClass=person)(uid={username}))", "x)(uid=*"); got != `(&(objectClass=person)(uid=x\29\28uid=\2a))` {
		t.Errorf("compound filter = %s", got)
	}
	if got := subjectFilter("entryUUID", "a*)"); got != `(entryUUID=a\2a\29)` {
		t.Errorf("subject filter = %s", got)
	}
}
