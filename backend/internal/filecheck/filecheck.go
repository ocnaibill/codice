// Package filecheck decides whether a file really is what its extension says
// and hashes it. It is shared by uploads, bulk import and the scanner of
// referenced libraries, so they all apply the same rules (RF-008).
package filecheck

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Supported is the set of file extensions the library accepts.
var Supported = map[string]bool{
	".pdf": true, ".epub": true,
	".cbz": true, ".cbr": true,
	".txt": true, ".md": true,
	".mobi": true, ".azw": true, ".azw3": true,
	".mp3": true, ".m4a": true, ".m4b": true,
	".flac": true, ".ogg": true, ".wav": true,
}

// ErrUnsupported: the extension is not one the library accepts.
var ErrUnsupported = errors.New("unsupported file format")

// BadContent means the bytes do not match the extension the file claims, or the file is one that is not taken. Msg, when
// there is one, is the whole sentence (the file may be a valid one, as an EPUB with DRM is).
type BadContent struct{ Ext, Why, Msg string }

func (e *BadContent) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("the content is not a valid %s file (%s)", e.Ext, e.Why)
}

// Limits that keep a hostile archive from exhausting the server (RNF-007).
const (
	maxZipEntries      = 100000
	maxZipUncompressed = 32 << 30 // 32 GiB declared
)

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}

// Validate checks that the bytes really are the format the extension
// claims: magic numbers for binary formats, structure for ZIP-based ones, valid
// UTF-8 for text. It is the first line of defence; extractors still run with
// their own limits.
func Validate(path, ext string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 8192)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	bad := func(why string) error { return &BadContent{Ext: ext, Why: why} }

	switch ext {
	case ".pdf":
		if !bytes.Contains(head[:min(len(head), 1024)], []byte("%PDF-")) {
			return bad("no PDF header")
		}
	case ".epub":
		return validateZip(path, ext, func(zr *zip.ReadCloser) error {
			for _, zf := range zr.File {
				if zf.Name == "mimetype" {
					rc, err := zf.Open()
					if err != nil {
						return bad("unreadable mimetype")
					}
					defer rc.Close()
					b, _ := io.ReadAll(io.LimitReader(rc, 64))
					if strings.TrimSpace(string(b)) != "application/epub+zip" {
						return bad("wrong mimetype")
					}
					return epubProtection(zr, ext)
				}
			}
			return bad("no mimetype entry")
		})
	case ".cbz":
		return validateZip(path, ext, func(zr *zip.ReadCloser) error {
			for _, zf := range zr.File {
				if !zf.FileInfo().IsDir() && imageExts[strings.ToLower(filepath.Ext(zf.Name))] {
					return nil
				}
			}
			return bad("no images inside")
		})
	case ".cbr":
		if !bytes.HasPrefix(head, []byte("Rar!")) {
			return bad("no RAR signature")
		}
	case ".mobi", ".azw", ".azw3":
		if len(head) < 68 || !bytes.Equal(head[60:68], []byte("BOOKMOBI")) {
			return bad("no MOBI signature")
		}
	case ".txt", ".md":
		if bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(trimIncompleteRune(head)) {
			return bad("not UTF-8 text; save it as UTF-8")
		}
	case ".mp3":
		if !(bytes.HasPrefix(head, []byte("ID3")) || (len(head) > 1 && head[0] == 0xFF && head[1]&0xE0 == 0xE0)) {
			return bad("no MP3 frame or ID3 tag")
		}
	case ".m4a", ".m4b":
		if len(head) < 12 || !bytes.Equal(head[4:8], []byte("ftyp")) {
			return bad("no MP4 ftyp box")
		}
	case ".flac":
		if !bytes.HasPrefix(head, []byte("fLaC")) {
			return bad("no FLAC signature")
		}
	case ".ogg":
		if !bytes.HasPrefix(head, []byte("OggS")) {
			return bad("no Ogg signature")
		}
	case ".wav":
		if len(head) < 12 || !bytes.Equal(head[:4], []byte("RIFF")) || !bytes.Equal(head[8:12], []byte("WAVE")) {
			return bad("no RIFF/WAVE header")
		}
	default:
		return ErrUnsupported
	}
	return nil
}

// fontObfuscation are the two algorithms that META-INF/encryption.xml names for what is not DRM: the obfuscation of an
// embedded font (IDPF and Adobe), which publishers use in ordinary books.
var fontObfuscation = map[string]bool{
	"http://www.idpf.org/2008/embedding": true,
	"http://ns.adobe.com/pdf/enc#RC":     true,
}

// epubProtection refuses an EPUB whose content is encrypted: any algorithm in META-INF/encryption.xml other than the
// obfuscation of fonts (Adobe ADEPT, Kobo, B&N and the like). Codice neither reads nor removes DRM, and without this the
// ciphertext would be read as text and indexed. A file that only has a rights or sinf file, or an encryption.xml that
// cannot be read, is not proof of anything and is taken.
func epubProtection(zr *zip.ReadCloser, ext string) error {
	for _, zf := range zr.File {
		if !strings.EqualFold(zf.Name, "META-INF/encryption.xml") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return nil
		}
		defer rc.Close()
		dec := xml.NewDecoder(io.LimitReader(rc, 1<<20))
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil
			}
			el, ok := tok.(xml.StartElement)
			if !ok || el.Name.Local != "EncryptionMethod" {
				continue
			}
			for _, a := range el.Attr {
				if a.Name.Local == "Algorithm" && !fontObfuscation[strings.TrimSpace(a.Value)] {
					return &BadContent{Ext: ext, Why: "protected by DRM", Msg: fmt.Sprintf(
						"this %s is protected by DRM (%s), which Codice neither reads nor removes", ext, strings.TrimSpace(a.Value))}
				}
			}
		}
	}
	return nil
}

// trimIncompleteRune drops a partial multi-byte character cut by the read window.
func trimIncompleteRune(b []byte) []byte {
	for i := 0; i < 3 && len(b) > 0; i++ {
		if utf8.Valid(b) {
			return b
		}
		b = b[:len(b)-1]
	}
	return b
}

func validateZip(path, ext string, check func(*zip.ReadCloser) error) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return &BadContent{Ext: ext, Why: "not a readable ZIP archive"}
	}
	defer zr.Close()
	if len(zr.File) > maxZipEntries {
		return &BadContent{Ext: ext, Why: "too many entries"}
	}
	var total uint64
	for _, zf := range zr.File {
		total += zf.UncompressedSize64
		if total > maxZipUncompressed {
			return &BadContent{Ext: ext, Why: "declared size is unreasonably large"}
		}
	}
	return check(zr)
}

// HashFile returns the SHA-256 (hex) and the size of a file, reading it once.
func HashFile(path string) (sum string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
