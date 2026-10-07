package handlers

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// storedNameMax is how long the name of a stored file may be, in bytes. File systems stop at 255 bytes, and the
	// organizer and the backup add to the name later, so there is room left.
	storedNameMax = 200
	// storedPrefix is what ingest puts before the name: 12 hex digits and an underscore.
	storedPrefix = 13
	// titleMax is the longest title the catalog holds, in characters (the column is varchar(255)).
	titleMax = 255
)

// cleanFilename turns the name a client sent into one that is safe to show and to store: only the last part of the path
// (a browser or a script may send a whole path, with / or \), no control characters, line breaks, NUL, zero-width marks
// or direction overrides (the ones that make "livro‮txt.exe" look like something else), no invalid UTF-8, one space
// for any run of spaces. It never returns an empty stem: ".txt" becomes "arquivo.txt".
func cleanFilename(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	ext := filepath.Ext(name)
	stem := strings.Trim(strings.TrimSuffix(name, ext), " .")
	if stem == "" {
		stem = "arquivo"
	}
	return stem + ext
}

// storedFilename is the cleaned name cut so that the stored name (prefix, name) fits in storedNameMax bytes, never in the
// middle of a character and never losing the extension.
func storedFilename(clean string) string {
	ext := filepath.Ext(clean)
	stem := strings.TrimSuffix(clean, ext)
	room := storedNameMax - storedPrefix - len(ext)
	if room < 1 {
		room = 1
	}
	return cutBytes(stem, room) + ext
}

// titleOf is the cleaned name as the first title of a work, cut to what the catalog holds.
func titleOf(clean string) string {
	if utf8.RuneCountInString(clean) <= titleMax {
		return clean
	}
	return string([]rune(clean)[:titleMax])
}

// cutBytes cuts s to at most n bytes without breaking a character.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return strings.TrimRight(s[:n], " .")
}
