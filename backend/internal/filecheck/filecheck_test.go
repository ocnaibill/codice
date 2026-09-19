package filecheck_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/filecheck"
)

func TestValidate_AndHashFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(content), 0o644)
		return p
	}

	if err := filecheck.Validate(write("ok.pdf", "%PDF-1.4 ..."), ".pdf"); err != nil {
		t.Errorf("a real PDF header: %v", err)
	}
	var bad *filecheck.BadContent
	if err := filecheck.Validate(write("fake.pdf", "just text"), ".pdf"); !errors.As(err, &bad) {
		t.Errorf("text saved as PDF: %v", err)
	}
	if err := filecheck.Validate(write("x.exe", "MZ"), ".exe"); !errors.Is(err, filecheck.ErrUnsupported) {
		t.Errorf("unknown extension: %v", err)
	}
	if !filecheck.Supported[".epub"] || filecheck.Supported[".exe"] {
		t.Error("the supported set is wrong")
	}

	// Hashing depends on the bytes, not the name.
	a, sizeA, _ := filecheck.HashFile(write("a.bin", "same"))
	b, _, _ := filecheck.HashFile(write("b.bin", "same"))
	c, _, _ := filecheck.HashFile(write("c.bin", "different"))
	if a != b || a == c || sizeA != 4 || len(a) != 64 {
		t.Errorf("hashes: %s %s %s (size %d)", a, b, c, sizeA)
	}
	if _, _, err := filecheck.HashFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("hashing a missing file must fail")
	}
}
