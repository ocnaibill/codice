package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// jsonLimit caps the two JSON members: a package is untrusted input until verified.
const jsonLimit = 1 << 30

var ageMagic = []byte("age-encryption.org/")

// ReadOptions controls how a package is read.
type ReadOptions struct {
	Passphrase string
	TmpDir     string
	// StageDir, when set, receives the storage/ members as they are read (they are
	// only kept if the whole package verifies). When empty they are read and discarded.
	StageDir string
}

// Package is a package that has been read completely and checked against its manifest.
type Package struct {
	Manifest  Manifest
	Files     []FileEntry
	DumpPath  string // the database dump, extracted to a temporary file
	Encrypted bool
	StageDir  string
}

// Close removes the temporary dump.
func (p *Package) Close() {
	if p != nil && p.DumpPath != "" {
		os.Remove(p.DumpPath)
	}
}

func validMember(name string) bool {
	switch name {
	case MemberManifest, MemberDump, MemberFiles:
		return true
	}
	if !strings.HasPrefix(name, MemberStorage) || path.Clean(name) != name {
		return false
	}
	_, ok := storage.SafeRel(strings.TrimPrefix(name, MemberStorage))
	return ok
}

// Read reads a whole package and verifies it against its manifest before returning:
// every member's size and sha256, that nothing is missing or extra, the format and the
// schema version. Nothing outside the temporary dump (and StageDir) is written, and a
// member with an unsafe name, a link or a duplicate refuses the package.
func Read(in io.Reader, o ReadOptions) (pkg *Package, err error) {
	br := bufio.NewReaderSize(in, 1<<20)
	encrypted := false
	if head, _ := br.Peek(len(ageMagic)); bytes.Equal(head, ageMagic) {
		encrypted = true
	}
	var src io.Reader = br
	if encrypted {
		if o.Passphrase == "" {
			return nil, ErrEncrypted
		}
		id, ierr := age.NewScryptIdentity(o.Passphrase)
		if ierr != nil {
			return nil, ierr
		}
		id.SetMaxWorkFactor(22)
		dec, derr := age.Decrypt(br, id)
		if derr != nil {
			var noMatch *age.NoIdentityMatchError
			if errors.As(derr, &noMatch) {
				return nil, ErrWrongPassphrase
			}
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, derr)
		}
		src = dec
	}

	pkg = &Package{Encrypted: encrypted, StageDir: o.StageDir}
	defer func() {
		if err != nil {
			pkg.Close()
			pkg = nil
		}
	}()

	tr := tar.NewReader(src)
	seen := map[string]Member{}
	var manifestRaw, filesRaw []byte
	for {
		h, herr := tr.Next()
		if herr == io.EOF {
			break
		}
		if herr != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, herr)
		}
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%w: %q is not a regular file", ErrCorrupt, h.Name)
		}
		if !validMember(h.Name) {
			return nil, fmt.Errorf("%w: unexpected member %q", ErrCorrupt, h.Name)
		}
		if _, dup := seen[h.Name]; dup {
			return nil, fmt.Errorf("%w: %q appears twice", ErrCorrupt, h.Name)
		}

		hash := sha256.New()
		var dst io.Writer = io.Discard
		var closeDst func() error
		switch {
		case h.Name == MemberManifest || h.Name == MemberFiles:
			if h.Size > jsonLimit {
				return nil, fmt.Errorf("%w: %s is too large", ErrCorrupt, h.Name)
			}
			dst = &bytes.Buffer{}
		case h.Name == MemberDump:
			f, ferr := os.CreateTemp(o.TmpDir, "codice-restore-*.dump")
			if ferr != nil {
				return nil, ferr
			}
			pkg.DumpPath = f.Name()
			dst, closeDst = f, f.Close
		case o.StageDir != "":
			rel := strings.TrimPrefix(h.Name, MemberStorage)
			full := filepath.Join(o.StageDir, filepath.FromSlash(rel))
			if merr := os.MkdirAll(filepath.Dir(full), 0o755); merr != nil {
				return nil, merr
			}
			f, ferr := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if ferr != nil {
				return nil, ferr
			}
			dst, closeDst = f, f.Close
		}
		n, cerr := io.Copy(io.MultiWriter(dst, hash), tr)
		if closeDst != nil {
			closeDst()
		}
		if cerr != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, cerr)
		}
		seen[h.Name] = Member{Size: n, SHA256: hex.EncodeToString(hash.Sum(nil))}
		if b, ok := dst.(*bytes.Buffer); ok {
			if h.Name == MemberManifest {
				manifestRaw = b.Bytes()
			} else {
				filesRaw = b.Bytes()
			}
		}
	}

	if manifestRaw == nil {
		return nil, fmt.Errorf("%w: no manifest (the package is incomplete)", ErrCorrupt)
	}
	if jerr := json.Unmarshal(manifestRaw, &pkg.Manifest); jerr != nil {
		return nil, fmt.Errorf("%w: the manifest is not readable", ErrCorrupt)
	}
	m := pkg.Manifest
	if m.Format != FormatVersion {
		return nil, fmt.Errorf("%w: unknown package format %d", ErrCorrupt, m.Format)
	}
	if m.SchemaVersion > database.LatestVersion() {
		return nil, ErrNewerSchema
	}
	delete(seen, MemberManifest)
	for name, want := range m.Members {
		got, ok := seen[name]
		switch {
		case !ok:
			return nil, fmt.Errorf("%w: %q is missing", ErrCorrupt, name)
		case got != want:
			return nil, fmt.Errorf("%w: %q does not match its recorded checksum", ErrCorrupt, name)
		}
		delete(seen, name)
	}
	for name := range seen {
		return nil, fmt.Errorf("%w: %q is not in the manifest", ErrCorrupt, name)
	}
	if _, ok := m.Members[MemberDump]; !ok || pkg.DumpPath == "" {
		return nil, fmt.Errorf("%w: no database dump", ErrCorrupt)
	}
	if filesRaw == nil {
		return nil, fmt.Errorf("%w: no list of files", ErrCorrupt)
	}
	if jerr := json.Unmarshal(filesRaw, &pkg.Files); jerr != nil {
		return nil, fmt.Errorf("%w: the list of files is not readable", ErrCorrupt)
	}
	if len(pkg.Files) != m.Files.Total {
		return nil, fmt.Errorf("%w: the list of files disagrees with the manifest", ErrCorrupt)
	}
	return pkg, nil
}
