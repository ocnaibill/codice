// Package textenc brings the text files that are not UTF-8 to UTF-8 when they enter the library (issue #171).
//
// The extraction and the reader read UTF-8 only, so a file in Windows-1252 (the old .txt) or in UTF-16 with a BOM (the
// "Unicode" of Notepad) would be refused, or read as garbage. Converting at the door keeps everything after it as it is.
// What it takes for sure is converted; what it cannot tell is refused, because a wrong guess shows wrong letters and
// says nothing.
package textenc

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// The names an owner reads ("converted from Windows-1252").
const (
	Windows1252 = "Windows-1252"
	UTF16LE     = "UTF-16 LE"
	UTF16BE     = "UTF-16 BE"
)

// ErrUnknown: the file is not UTF-8, and it is not an encoding that is taken for sure.
var ErrUnknown = errors.New("not UTF-8, and not Windows-1252 or UTF-16 with a byte order mark")

// undefined1252 are the bytes that Windows-1252 leaves without a character: a text that has one is not in it.
var undefined1252 = [256]bool{0x81: true, 0x8d: true, 0x8f: true, 0x90: true, 0x9d: true}

// Detect says how the file is encoded: "" when it is UTF-8 already (nothing to do), or one of the names above. It reads the
// whole file once, in blocks. ErrUnknown when it cannot tell: binary data (a NUL), a damaged UTF-8 (valid sequences and
// invalid bytes together, which as Windows-1252 would turn "ç" into "Ã§"), or bytes that Windows-1252 does not have.
func Detect(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)

	head, _ := r.Peek(4)
	switch {
	case len(head) >= 4 && head[0] == 0xff && head[1] == 0xfe && head[2] == 0 && head[3] == 0:
		return "", ErrUnknown // UTF-32 LE: not taken
	case len(head) >= 2 && head[0] == 0xff && head[1] == 0xfe:
		return UTF16LE, nil
	case len(head) >= 2 && head[0] == 0xfe && head[1] == 0xff:
		return UTF16BE, nil
	}

	var multibyte, invalid int
	bad1252 := false
	for {
		buf, _ := r.Peek(utf8.UTFMax)
		if len(buf) == 0 {
			break
		}
		if buf[0] == 0 {
			return "", ErrUnknown
		}
		c, size := utf8.DecodeRune(buf)
		switch {
		case c == utf8.RuneError && size == 1:
			invalid++
			if undefined1252[buf[0]] || (buf[0] < 0x20) {
				bad1252 = true
			}
		case size > 1:
			multibyte++
		case buf[0] < 0x20 && buf[0] != '\t' && buf[0] != '\n' && buf[0] != '\r' && buf[0] != '\f':
			bad1252 = true // a control character: not a text anybody wrote in Windows-1252
		}
		r.Discard(size)
	}
	switch {
	case invalid == 0:
		return "", nil
	case multibyte > 0 || bad1252:
		return "", ErrUnknown
	}
	return Windows1252, nil
}

// Convert rewrites the file at path as UTF-8 when Detect says it is in another encoding, and answers with the name of the
// encoding it came from ("" when nothing was done). The new bytes replace the file at once (a rename), and the old ones
// are gone only then.
func Convert(path string) (string, error) {
	from, err := Detect(path)
	if err != nil || from == "" {
		return "", err
	}
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	var decoder transform.Transformer
	switch from {
	case Windows1252:
		decoder = charmap.Windows1252.NewDecoder()
	case UTF16LE:
		decoder = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	case UTF16BE:
		decoder = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
	}
	out, err := os.CreateTemp(filepath.Dir(path), "conv-*")
	if err != nil {
		return "", err
	}
	outPath := out.Name()
	defer func() {
		out.Close()
		os.Remove(outPath) // nothing is left when it failed; after the rename there is no such file
	}()
	if _, err := io.Copy(out, transform.NewReader(src, decoder)); err != nil {
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(outPath, path); err != nil {
		return "", err
	}
	return from, nil
}
