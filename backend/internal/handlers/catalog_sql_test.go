package handlers

import "testing"

func TestFilesURL_EscapesEachSegmentButKeepsTheFolders(t *testing.T) {
	cases := map[string]string{
		"a1_Duna.epub":                       "/files/a1_Duna.epub",
		"Frank Herbert/Duna/Duna.epub":       "/files/Frank%20Herbert/Duna/Duna.epub",
		"Autor/Obra/Português — Aleph/O.pdf": "/files/Autor/Obra/Portugu%C3%AAs%20%E2%80%94%20Aleph/O.pdf",
		"A/50% #1?/x.epub":                   "/files/A/50%25%20%231%3F/x.epub",
	}
	for in, want := range cases {
		if got := filesURL(in); got != want {
			t.Errorf("filesURL(%q) = %q, want %q", in, got, want)
		}
	}
}
