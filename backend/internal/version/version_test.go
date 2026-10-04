package version

import "testing"

func TestSourceURL(t *testing.T) {
	good := map[string]string{
		"":                                    DefaultSourceURL,
		"   ":                                 DefaultSourceURL,
		"https://git.example.com/me/codice":   "https://git.example.com/me/codice",
		" http://192.168.1.5:3000/me/codice ": "http://192.168.1.5:3000/me/codice",
		"https://git.example.com/me/codice?ref=x": "https://git.example.com/me/codice?ref=x",
	}
	for in, want := range good {
		if got, err := SourceURL(in); err != nil || got != want {
			t.Errorf("SourceURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"git.example.com/me/codice", "ftp://git.example.com/x", "javascript:alert(1)", "https://", "https:///x",
		"https://user:pw@git.example.com/x", "https://git.example.com/x#frag", "data:text/html,<script>", "//git.example.com",
	} {
		if got, err := SourceURL(in); err == nil {
			t.Errorf("SourceURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestVersionDefaultsToDev(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q: a build without -ldflags must say dev", Version)
	}
}
