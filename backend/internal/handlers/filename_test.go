package handlers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanFilename(t *testing.T) {
	for in, want := range map[string]string{
		"Duna.epub":                    "Duna.epub",
		"../../../etc/passwd.txt":      "passwd.txt",
		`C:\Users\Ana\Livros\Duna.pdf`: "Duna.pdf",
		"linha1\nlinha2\r\n.txt":       "linha1 linha2.txt",
		"livro\u202etxt.exe\u0000.txt": "livro txt.exe.txt",
		"zero\u200bwidth\ufeff.pdf":    "zero width.pdf",
		"muitos    espaços   .epub":    "muitos espaços.epub",
		".txt":                         "arquivo.txt",
		"...pdf":                       "arquivo.pdf",
		"  .md":                        "arquivo.md",
		"sem extensão":                 "sem extensão",
		"bytes\xff\xfeinválidos.txt":   "bytesinválidos.txt",
		"Coração, ação e emoção (2ª ed).pdf": "Coração, ação e emoção (2ª ed).pdf",
		"日本語の本.epub":                         "日本語の本.epub",
	} {
		if got := cleanFilename(in); got != want {
			t.Errorf("cleanFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoredFilename_FitsAndKeepsTheExtension(t *testing.T) {
	for name, clean := range map[string]string{
		"ascii, 300 characters": strings.Repeat("n", 300) + ".txt",
		"accents, 2 bytes each": strings.Repeat("ç", 200) + ".epub",
		"japanese, 3 bytes":     strings.Repeat("本", 200) + ".pdf",
		"emoji, 4 bytes":        strings.Repeat("🙂", 100) + ".cbz",
		"a long extension":      "livro." + strings.Repeat("x", 300),
	} {
		got := storedFilename(clean)
		if storedPrefix+len(got) > storedNameMax && !strings.Contains(name, "extension") {
			t.Errorf("%s: %d bytes with the prefix, want at most %d", name, storedPrefix+len(got), storedNameMax)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: cut in the middle of a character: %q", name, got)
		}
		if !strings.Contains(name, "extension") && !strings.HasSuffix(got, clean[strings.LastIndex(clean, "."):]) {
			t.Errorf("%s: lost the extension: %q", name, got)
		}
	}
	if got := storedFilename("curto.pdf"); got != "curto.pdf" {
		t.Errorf("a short name changed: %q", got)
	}
	// What is cut must not leave a stem ending in a space or a dot.
	if got := storedFilename(strings.Repeat("a", 170) + " . b" + strings.Repeat("c", 40) + ".txt"); strings.Contains(got, " .txt") || strings.Contains(got, "..") {
		t.Errorf("dangling separator: %q", got)
	}
}

func TestTitleOf_CutsAtWhatTheColumnHolds(t *testing.T) {
	long := strings.Repeat("ç", 400) + ".epub"
	if got := titleOf(long); utf8.RuneCountInString(got) != titleMax {
		t.Errorf("%d characters, want %d", utf8.RuneCountInString(got), titleMax)
	}
	if got := titleOf("Duna.epub"); got != "Duna.epub" {
		t.Errorf("a short title changed: %q", got)
	}
	for n, want := range map[int]int{254: 254, 255: 255, 256: 255} {
		if got := titleOf(strings.Repeat("ç", n)); utf8.RuneCountInString(got) != want {
			t.Errorf("%d characters became %d, want %d", n, utf8.RuneCountInString(got), want)
		}
	}
}
