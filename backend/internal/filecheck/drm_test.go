package filecheck_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/filecheck"
)

// epubWith writes an EPUB that has what the test gives besides the mimetype.
func epubWith(t *testing.T, extra map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	w.Write([]byte("application/epub+zip"))
	for name, body := range extra {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	p := filepath.Join(t.TempDir(), "livro.epub")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func encryption(methods ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container">`)
	for _, m := range methods {
		b.WriteString(`<EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#"><EncryptionMethod Algorithm="` + m + `"/>` +
			`<CipherData><CipherReference URI="OEBPS/x"/></CipherData></EncryptedData>`)
	}
	b.WriteString(`</encryption>`)
	return b.String()
}

func TestValidate_AnEPUBWithDRMIsRefusedAndSaysWhy(t *testing.T) {
	for name, algorithm := range map[string]string{
		"Adobe ADEPT (AES-128)": "http://www.w3.org/2001/04/xmlenc#aes128-cbc",
		"AES-256":               "http://www.w3.org/2001/04/xmlenc#aes256-cbc",
		"some other scheme":     "http://example.com/our-own-drm",
	} {
		err := filecheck.Validate(epubWith(t, map[string]string{"META-INF/encryption.xml": encryption(algorithm)}), ".epub")
		var bad *filecheck.BadContent
		if !errors.As(err, &bad) {
			t.Errorf("%s: %v, want a refusal", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "protected by DRM") || !strings.Contains(err.Error(), algorithm) || strings.Contains(err.Error(), "not a valid") {
			t.Errorf("%s: the message does not say what it is: %q", name, err)
		}
	}
}

func TestValidate_AnEPUBThatIsNotProvedToHaveDRMIsTaken(t *testing.T) {
	obfuscation := encryption("http://www.idpf.org/2008/embedding", "http://ns.adobe.com/pdf/enc#RC")
	for name, extra := range map[string]map[string]string{
		"no encryption file":                   {},
		"obfuscated fonts, as publishers do":   {"META-INF/encryption.xml": obfuscation},
		"a sinf file alone":                    {"META-INF/sinf.xml": "<fairplay/>"},
		"a rights file alone":                  {"META-INF/rights.xml": "<rights/>"},
		"an encryption file that is not XML":   {"META-INF/encryption.xml": "\x00\x01 not xml <<<"},
		"an empty encryption file":             {"META-INF/encryption.xml": ""},
		"an encryption element with no method": {"META-INF/encryption.xml": `<encryption><EncryptedData/></encryption>`},
	} {
		if err := filecheck.Validate(epubWith(t, extra), ".epub"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestValidate_OneEncryptedChapterAmongObfuscatedFontsIsDRM(t *testing.T) {
	mixed := encryption("http://www.idpf.org/2008/embedding", "http://www.w3.org/2001/04/xmlenc#aes128-cbc")
	if err := filecheck.Validate(epubWith(t, map[string]string{"META-INF/encryption.xml": mixed}), ".epub"); err == nil {
		t.Error("an encrypted chapter next to obfuscated fonts was taken")
	}
}

func TestValidate_TheEncryptionFileIsFoundWhateverTheCase(t *testing.T) {
	file := epubWith(t, map[string]string{"META-INF/Encryption.XML": encryption("http://www.w3.org/2001/04/xmlenc#aes128-cbc")})
	if err := filecheck.Validate(file, ".epub"); err == nil {
		t.Error("a differently cased encryption file was missed")
	}
}

func TestValidate_TheAlgorithmInTheMessageHasNoPaddingOfTheFile(t *testing.T) {
	err := filecheck.Validate(epubWith(t, map[string]string{"META-INF/encryption.xml": encryption("  http://example.com/drm \n")}), ".epub")
	if err == nil || !strings.Contains(err.Error(), "(http://example.com/drm)") {
		t.Errorf("%v", err)
	}
}
