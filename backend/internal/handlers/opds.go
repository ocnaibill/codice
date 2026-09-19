package handlers

import (
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
)

type OPDSHandler struct {
	DB   *sql.DB
	Auth appMiddleware.Authenticator
}

// OpdsAuth authenticates OPDS clients with an app token (HTTP Basic, the token
// as the password) or a session bearer token. The account password is not
// accepted (DEC-071).
func (h *OPDSHandler) OpdsAuth(next http.Handler) http.Handler {
	return h.Auth.WithBasic(next)
}

func (h *OPDSHandler) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func (h *OPDSHandler) RootCatalog(w http.ResponseWriter, r *http.Request) {
	base := h.baseURL(r)
	now := time.Now().Format(time.RFC3339)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"` + "\n")
	b.WriteString(`      xmlns:dc="http://purl.org/dc/terms/"` + "\n")
	b.WriteString(`      xmlns:opds="http://opds-spec.org/2010/catalog">` + "\n")
	b.WriteString(fmt.Sprintf("  <id>urn:uuid:codice-catalog</id>\n"))
	b.WriteString(fmt.Sprintf("  <title>Codice Catalog</title>\n"))
	b.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", now))
	b.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"%s/opds/v1.2/catalog\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", base))
	b.WriteString(fmt.Sprintf("  <link rel=\"start\" href=\"%s/opds/v1.2/catalog\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", base))
	b.WriteString(fmt.Sprintf("  <link rel=\"search\" href=\"%s/opds/v1.2/search?q={searchTerms}\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", base))
	b.WriteString(fmt.Sprintf("  <entry>\n"))
	b.WriteString(fmt.Sprintf("    <title>Recently Added</title>\n"))
	b.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-recent</id>\n"))
	b.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", now))
	b.WriteString(fmt.Sprintf("    <link rel=\"subsection\" href=\"%s/opds/v1.2/recent\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", base))
	b.WriteString(fmt.Sprintf("  </entry>\n"))
	b.WriteString(fmt.Sprintf("</feed>\n"))

	w.Header().Set("Content-Type", "application/atom+xml;charset=utf-8")
	w.Write([]byte(b.String()))
}

func (h *OPDSHandler) RecentFeed(w http.ResponseWriter, r *http.Request) {
	base := h.baseURL(r)
	now := time.Now().Format(time.RFC3339)

	rows, err := h.DB.Query(`
		SELECT w.id, w.original_title, ` + authorLabel + `, COALESCE(wp.cover_url, ''),
		       COALESCE(wp.file_format, ''), COALESCE(wp.file_path, ''), w.created_at
		` + catalogFrom + `
		WHERE w.retired_at IS NULL
		ORDER BY w.id DESC LIMIT 50
	`)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries strings.Builder
	for rows.Next() {
		var id int
		var title, author, coverURL, format, filePath string
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &title, &author, &coverURL, &format, &filePath, &createdAt); err != nil {
			continue
		}
		updated := now
		if createdAt.Valid {
			updated = createdAt.Time.Format(time.RFC3339)
		}
		acqType := "application/pdf"
		switch format {
		case "epub":
			acqType = "application/epub+zip"
		case "cbz", "cbr":
			acqType = "application/x-cbz"
		case "txt":
			acqType = "text/plain"
		case "md":
			acqType = "text/markdown"
		case "mp3", "m4a", "m4b", "ogg", "wav", "flac":
			acqType = "audio/mpeg"
		}

		filename := filepath.Base(filePath)
		escapedFilename := strings.TrimPrefix(filesURL(filename), "/files/")

		if coverURL == "" {
			coverURL = "/covers/placeholder.svg"
		}

		entries.WriteString(fmt.Sprintf("  <entry>\n"))
		entries.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		entries.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-work-%d</id>\n", id))
		entries.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updated))
		entries.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		entries.WriteString(fmt.Sprintf("    <dc:identifier>%d</dc:identifier>\n", id))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s/files/%s\" type=\"%s\"/>\n", base, escapedFilename, acqType))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/image\" href=\"%s%s\" type=\"image/jpeg\"/>\n", base, coverURL))
		entries.WriteString(fmt.Sprintf("  </entry>\n"))
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"` + "\n")
	b.WriteString(`      xmlns:dc="http://purl.org/dc/terms/"` + "\n")
	b.WriteString(`      xmlns:opds="http://opds-spec.org/2010/catalog">` + "\n")
	b.WriteString(fmt.Sprintf("  <id>urn:uuid:codice-recent</id>\n"))
	b.WriteString(fmt.Sprintf("  <title>Recently Added</title>\n"))
	b.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", now))
	b.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"%s/opds/v1.2/recent\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", base))
	b.WriteString(fmt.Sprintf("  <link rel=\"start\" href=\"%s/opds/v1.2/catalog\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", base))
	b.WriteString(entries.String())
	b.WriteString(fmt.Sprintf("</feed>\n"))

	w.Header().Set("Content-Type", "application/atom+xml;charset=utf-8")
	w.Write([]byte(b.String()))
}

func (h *OPDSHandler) SearchFeed(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing search query", http.StatusBadRequest)
		return
	}
	base := h.baseURL(r)
	now := time.Now().Format(time.RFC3339)

	rows, err := h.DB.Query(`
		SELECT w.id, w.original_title, ` + authorLabel + `, COALESCE(wp.cover_url, ''),
		       COALESCE(wp.file_format, ''), COALESCE(wp.file_path, ''), w.created_at
		` + catalogFrom + `
		WHERE w.retired_at IS NULL
		  AND (LOWER(w.original_title) LIKE LOWER($1) OR ` + authorMatches("$1") + `)
		ORDER BY w.id DESC LIMIT 50
	`, "%"+query+"%")
	if err != nil {
		http.Error(w, "Error searching", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries strings.Builder
	count := 0
	for rows.Next() {
		var id int
		var title, author, coverURL, format, filePath string
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &title, &author, &coverURL, &format, &filePath, &createdAt); err != nil {
			continue
		}
		count++
		updated := now
		if createdAt.Valid {
			updated = createdAt.Time.Format(time.RFC3339)
		}
		acqType := "application/pdf"
		switch format {
		case "epub":
			acqType = "application/epub+zip"
		case "cbz", "cbr":
			acqType = "application/x-cbz"
		case "txt":
			acqType = "text/plain"
		case "md":
			acqType = "text/markdown"
		}

		filename := filepath.Base(filePath)
		escapedFilename := strings.TrimPrefix(filesURL(filename), "/files/")

		if coverURL == "" {
			coverURL = "/covers/placeholder.svg"
		}

		entries.WriteString(fmt.Sprintf("  <entry>\n"))
		entries.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		entries.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-work-%d</id>\n", id))
		entries.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updated))
		entries.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		entries.WriteString(fmt.Sprintf("    <dc:identifier>%d</dc:identifier>\n", id))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s/files/%s\" type=\"%s\"/>\n", base, escapedFilename, acqType))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/image\" href=\"%s%s\" type=\"image/jpeg\"/>\n", base, coverURL))
		entries.WriteString(fmt.Sprintf("  </entry>\n"))
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"` + "\n")
	b.WriteString(`      xmlns:dc="http://purl.org/dc/terms/"` + "\n")
	b.WriteString(`      xmlns:opds="http://opds-spec.org/2010/catalog">` + "\n")
	b.WriteString(fmt.Sprintf("  <id>urn:uuid:codice-search</id>\n"))
	b.WriteString(fmt.Sprintf("  <title>Search Results</title>\n"))
	b.WriteString(fmt.Sprintf("  <updated>%s</updated>\n", now))
	b.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"%s/opds/v1.2/search?q=%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", base, url.QueryEscape(query)))
	b.WriteString(fmt.Sprintf("  <link rel=\"start\" href=\"%s/opds/v1.2/catalog\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", base))
	b.WriteString(fmt.Sprintf("  <opensearch:totalResults>%d</opensearch:totalResults>\n", count))
	b.WriteString(entries.String())
	b.WriteString(fmt.Sprintf("</feed>\n"))

	w.Header().Set("Content-Type", "application/atom+xml;charset=utf-8")
	w.Write([]byte(b.String()))
}