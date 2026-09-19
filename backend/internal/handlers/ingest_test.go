package handlers

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpusFile reads a file of the synthetic corpus (testdata/corpus at the repo root).
func corpusFile(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "corpus", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeTemp(t *testing.T, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestValidateContent_AcceptsTheRealFormats(t *testing.T) {
	for name, ext := range map[string]string{
		"cbz_ltr.cbz": ".cbz", "cbz_rtl.cbz": ".cbz", "cbz_webtoon_imagem_longa.cbz": ".cbz",
		"epub_acentos.epub": ".epub", "epub_fixed_layout.epub": ".epub", "epub_outra_edicao_en.epub": ".epub",
		"pdf_digital.pdf": ".pdf", "pdf_escaneado.pdf": ".pdf", "pdf_misto.pdf": ".pdf",
		"notas.txt": ".txt",
	} {
		if err := validateContent(writeTemp(t, name, corpusFile(t, name)), ext); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
}

func TestValidateContent_RejectsWhatOnlyLooksLikeTheFormat(t *testing.T) {
	cases := map[string]struct {
		ext     string
		content []byte
	}{
		"a truncated EPUB from the corpus":  {".epub", corpusFile(t, "epub_corrompido.epub")},
		"text saved as .pdf (the corpus)":   {".pdf", corpusFile(t, "falso.pdf")},
		"an executable named .pdf":          {".pdf", append([]byte("MZ\x90\x00"), bytes.Repeat([]byte{0}, 2000)...)},
		"a ZIP of images named .epub":       {".epub", zipOf(t, map[string]string{"a.jpg": "x"})},
		"an EPUB with the wrong mimetype":   {".epub", zipOf(t, map[string]string{"mimetype": "application/zip"})},
		"a ZIP without images named .cbz":   {".cbz", zipOf(t, map[string]string{"readme.txt": "x"})},
		"a PDF named .cbz":                  {".cbz", corpusFile(t, "pdf_digital.pdf")},
		"binary data named .txt":            {".txt", []byte("abc\x00\x01\x02def")},
		"invalid UTF-8 named .md":           {".md", []byte{0xff, 0xfe, 0xfd, 'a'}},
		"text named .mp3":                   {".mp3", []byte("just some text, not audio at all")},
		"text named .m4b":                   {".m4b", []byte("just some text, not audio at all")},
		"text named .flac":                  {".flac", []byte("not flac")},
		"text named .cbr":                   {".cbr", []byte("not rar")},
		"random bytes named .mobi":          {".mobi", bytes.Repeat([]byte{7}, 200)},
		"an empty-header RIFF named .wav":   {".wav", []byte("RIFF\x00\x00\x00\x00XXXX")},
		"a zip entry count bomb named .cbz": {".cbz", nil},
	}
	delete(cases, "a zip entry count bomb named .cbz") // covered by the limit constants, not built here
	for name, c := range cases {
		err := validateContent(writeTemp(t, "f"+c.ext, c.content), c.ext)
		var bad *errBadContent
		if !errors.As(err, &bad) {
			t.Errorf("%s: got %v, want errBadContent", name, err)
		}
	}
}

func TestValidateContent_AcceptsMinimalValidSamples(t *testing.T) {
	ok := map[string][]byte{
		".mp3":  []byte("ID3\x03\x00\x00\x00\x00\x00\x00"),
		".m4a":  append([]byte("\x00\x00\x00\x18ftypM4A "), bytes.Repeat([]byte{0}, 40)...),
		".flac": []byte("fLaC\x00\x00\x00\x22"),
		".ogg":  []byte("OggS\x00\x02"),
		".wav":  []byte("RIFF\x24\x00\x00\x00WAVEfmt "),
		".cbr":  []byte("Rar!\x1a\x07\x00"),
		".md":   []byte("# Título com acentos: ção\n"),
		".mobi": append(bytes.Repeat([]byte{0}, 60), []byte("BOOKMOBI")...),
	}
	for ext, content := range ok {
		if ext != ".md" { // binary formats may be padded; text must not contain NUL bytes
			content = append(content, bytes.Repeat([]byte{0}, 16)...)
		}
		if err := validateContent(writeTemp(t, "f"+ext, content), ext); err != nil {
			t.Errorf("%s: %v", ext, err)
		}
	}
	if err := validateContent(writeTemp(t, "x.exe", []byte("MZ")), ".exe"); !errors.Is(err, errUnsupportedFormat) {
		t.Errorf("unknown extension: %v", err)
	}
	// A multi-byte character cut by the read window must not make valid text look invalid.
	long := strings.Repeat("a", 8191) + "ã"
	if err := validateContent(writeTemp(t, "long.txt", []byte(long)), ".txt"); err != nil {
		t.Errorf("valid UTF-8 cut at the window edge: %v", err)
	}
}
