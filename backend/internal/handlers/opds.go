package handlers

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
)

type OPDSHandler struct {
	DB *sql.DB
}

func (h *OPDSHandler) OpdsAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Codice OPDS"`)
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(authHeader, "Basic ") {
			payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
			if err != nil {
				http.Error(w, "Invalid Basic Auth", http.StatusUnauthorized)
				return
			}
			parts := strings.SplitN(string(payload), ":", 2)
			if len(parts) != 2 {
				http.Error(w, "Invalid Basic Auth", http.StatusUnauthorized)
				return
			}
			var userID string
			err = h.DB.QueryRow("SELECT id FROM users WHERE username = $1", parts[0]).Scan(&userID)
			if err != nil {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(authHeader, "Bearer ") {
			appMiddleware.AuthMiddleware(next).ServeHTTP(w, r)
			return
		}
		http.Error(w, "Unsupported authorization method", http.StatusUnauthorized)
	})
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
		SELECT w.id, w.original_title, COALESCE(p.name, 'Unknown Author'), COALESCE(e.cover_url, ''),
		       COALESCE(w.format, ''), w.created_at
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
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
		var title, author, coverURL, format string
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &title, &author, &coverURL, &format, &createdAt); err != nil {
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
		entries.WriteString(fmt.Sprintf("  <entry>\n"))
		entries.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		entries.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-work-%d</id>\n", id))
		entries.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updated))
		entries.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		entries.WriteString(fmt.Sprintf("    <dc:identifier>%d</dc:identifier>\n", id))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s/files/%%s\" type=\"%s\"/>\n", base, acqType))
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
		SELECT w.id, w.original_title, COALESCE(p.name, 'Unknown Author'), COALESCE(e.cover_url, ''),
		       COALESCE(w.format, ''), w.created_at
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		WHERE LOWER(w.original_title) LIKE LOWER($1) OR LOWER(COALESCE(p.name, '')) LIKE LOWER($1)
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
		var title, author, coverURL, format string
		var createdAt sql.NullTime
		if err := rows.Scan(&id, &title, &author, &coverURL, &format, &createdAt); err != nil {
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
		entries.WriteString(fmt.Sprintf("  <entry>\n"))
		entries.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		entries.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-work-%d</id>\n", id))
		entries.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updated))
		entries.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		entries.WriteString(fmt.Sprintf("    <dc:identifier>%d</dc:identifier>\n", id))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s/files/%%s\" type=\"%s\"/>\n", base, acqType))
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
	b.WriteString(fmt.Sprintf("  <link rel=\"self\" href=\"%s/opds/v1.2/search?q=%s\" type=\"application/atom+xml;profile=opds-catalog;kind=acquisition\"/>\n", base, query))
	b.WriteString(fmt.Sprintf("  <link rel=\"start\" href=\"%s/opds/v1.2/catalog\" type=\"application/atom+xml;profile=opds-catalog;kind=navigation\"/>\n", base))
	b.WriteString(fmt.Sprintf("  <opensearch:totalResults>%d</opensearch:totalResults>\n", count))
	b.WriteString(entries.String())
	b.WriteString(fmt.Sprintf("</feed>\n"))

	w.Header().Set("Content-Type", "application/atom+xml;charset=utf-8")
	w.Write([]byte(b.String()))
}