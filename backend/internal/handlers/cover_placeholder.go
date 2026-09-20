package handlers

import (
	_ "embed"
	"net/http"
)

//go:embed cover_placeholder.svg
var coverPlaceholder []byte

// CoverPlaceholder serves the picture shown for a work that has no cover. The catalog points
// to /covers/placeholder.svg for such works; without this the address answered 404 on any
// installation where nobody had put a file there.
func CoverPlaceholder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=604800, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A plain picture: nothing in it may run, whatever loads it.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Write(coverPlaceholder)
}
