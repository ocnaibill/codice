// Package backup makes and restores a Códice instance's operational data (RF-034,
// RNF-010, DEC-064): the database and the list of files, optionally the files
// themselves, in one package that is verified before anything is overwritten.
//
// A package is a tar (optionally encrypted with age, with a passphrase the operator
// keeps off the server) holding, in this order:
//
//	database.dump   the database, from pg_dump, without credentials or single-use links
//	files.json      every stored file: path, size, sha256, taken from the same snapshot
//	storage/...     the managed files and covers, only with --include-files
//	manifest.json   what the package contains and the sha256 of every member, written last
//
// Nothing here needs a service: it talks to PostgreSQL and the storage directory.
package backup

import (
	"errors"
	"time"
)

// FormatVersion is the layout of the package itself.
const FormatVersion = 1

const (
	MemberManifest = "manifest.json"
	MemberDump     = "database.dump"
	MemberFiles    = "files.json"
	MemberStorage  = "storage/"
)

// credentialTables hold sessions, tokens and single-use links, and the record of sign-ins. Their data is left out
// of every package: a backup travels and lingers, and a restored instance must not
// revive a session, an app token or a link that was revoked after the backup. The record of sign-ins is a log of
// addresses kept for a time the owner sets; a package that outlived it would break that promise.
var credentialTables = []string{"sessions", "app_tokens", "invitations", "password_resets", "link_tickets", "login_events"}

// derivedTables hold what is made from the files: the text extracted for search, and the fingerprint of it that
// finds the same text in another file (#38). Their data is left
// out of every package, because it is large, can be made again and is worth nothing without the files
// it came from. A restored instance asks for it to be extracted anew (see afterRestore), so search is
// back when that job has run, and a restore is as quick as the database without it.
//
// The one text that is NOT here is what OCR read (ocr_pages, #24, with the language each file was read in, ocr_files): the native text is made again in seconds, but
// reading a scanned library again takes hours, and what it found is only text. It travels in the package, and the
// text of a file is made from those pages when it is extracted anew, so a restore needs no image to be read again.
var derivedTables = []string{"document_segments", "text_extractions", "document_segment_embeddings", "text_embedding_status", "text_fingerprints"}

var (
	ErrEncrypted       = errors.New("the package is encrypted: give the passphrase (CODICE_BACKUP_PASSPHRASE or --passphrase-file)")
	ErrWrongPassphrase = errors.New("the passphrase does not open this package")
	ErrCorrupt         = errors.New("the package is corrupt or incomplete")
	ErrNewerSchema     = errors.New("the package comes from a newer Códice than this one: update before restoring")
	ErrNotEmpty        = errors.New("the target database already holds data")
	ErrBusy            = errors.New("other connections are using the target database: stop the API and the worker first")
	ErrNoTool          = errors.New("pg_dump and pg_restore are needed (install the PostgreSQL client tools)")
	// ErrToolTooNew: a pg_dump newer than the server writes commands the server does not
	// know, so the package would not restore on it. Use the client of the same major version
	// (or set PG_DUMP to one).
	ErrToolTooNew = errors.New("pg_dump is newer than the database server, so the package might not restore on it")
	// ErrServerTooOld: a package made on a newer PostgreSQL cannot be loaded into an older one.
	ErrServerTooOld = errors.New("the package was made on a newer PostgreSQL than the target server runs")
)

// Member describes one file inside the package.
type Member struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Counts are a few facts about the data, taken from the snapshot, so a restored copy
// can be checked against them.
type Counts struct {
	Users int `json:"users"`
	Works int `json:"works"`
	Files int `json:"files"`
	Notes int `json:"notes"`
	// OCRPages are the pages OCR has read (#24). Absent in a package from before OCR, which is the same as none.
	OCRPages int `json:"ocrPages,omitempty"`
}

// Manifest says what a package holds. It is the last member, so a truncated package
// has none and is refused.
type Manifest struct {
	Format        int               `json:"format"`
	Tool          string            `json:"tool"`
	CreatedAt     time.Time         `json:"createdAt"`
	SchemaVersion int64             `json:"schemaVersion"`
	ServerVersion string            `json:"serverVersion"`
	IncludesFiles bool              `json:"includesFiles"`
	Counts        Counts            `json:"counts"`
	Files         FilesSummary      `json:"files"`
	Members       map[string]Member `json:"members"`
}

// FilesSummary counts the stored files and what became of them when the package was made.
type FilesSummary struct {
	Total    int   `json:"total"`
	Bytes    int64 `json:"bytes"`
	Included int   `json:"included"`
	Missing  int   `json:"missing"` // recorded in the database but not on disk when the package was made
}

// FileEntry is one stored file, from the database snapshot.
type FileEntry struct {
	FileID int64  `json:"fileId"`
	Title  string `json:"title,omitempty"`
	Mode   string `json:"mode"` // managed or referenced
	Root   string `json:"root,omitempty"`
	Path   string `json:"path"`  // where the location says it is
	Bytes  string `json:"bytes"` // where the bytes actually are (differs when trashed); relative to root
	State  string `json:"state"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

// LastBackup is what the interface shows about the latest package made here (UI-19).
type LastBackup struct {
	At            time.Time `json:"at"`
	Bytes         int64     `json:"bytes"`
	Files         int       `json:"files"`
	IncludesFiles bool      `json:"includesFiles"`
	Encrypted     bool      `json:"encrypted"`
	// Name and Path say where the package is, when the command knew: a package sent to standard
	// output has no address of its own, unless the operator said it with --as-path. Path is the
	// owner's to see; the interface sends only the Name to the administrators.
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// Verified is the last time a package was checked with verify-backup (UI-19): which one, when, and
// whether it was only read end to end or also rehearsed as a restore (--deep).
type Verified struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
	Deep bool      `json:"deep"`
}
