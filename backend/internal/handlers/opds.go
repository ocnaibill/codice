package handlers

import (
	"database/sql"
	"fmt"
	"github.com/ocnaibill/codice/backend/internal/people"
	"html"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
)

type OPDSHandler struct {
	DB   *sql.DB
	Auth appMiddleware.Authenticator
	// PublicURL is the address people reach the app at from outside (CODICE_PUBLIC_URL,
	// already checked by middleware.ParsePublicURL). When set, every link of the catalog
	// uses it as it is. When empty, the links are built from the request.
	PublicURL string
	// TrustedProxies are the proxies whose X-Forwarded-Proto is believed (see RequestScheme).
	TrustedProxies []netip.Prefix
}

// OpdsAuth authenticates OPDS clients with an app token (HTTP Basic, the token
// as the password) or a session bearer token. The account password is not
// accepted (DEC-071).
func (h *OPDSHandler) OpdsAuth(next http.Handler) http.Handler {
	return h.Auth.WithBasic(next)
}

// baseURL is what every link of the catalog starts with. A client follows these links as
// written, and it is not a browser: nothing upgrades an http:// link to https:// for it, so
// behind a proxy that ends the TLS a link that says http:// sends the token of the app,
// and the book, in the clear (#80). The configured public address wins; without one, the
// scheme the proxy reports (when the proxy is a trusted one) and the host of the request.
func (h *OPDSHandler) baseURL(r *http.Request) string {
	if h.PublicURL != "" {
		return h.PublicURL
	}
	return fmt.Sprintf("%s://%s", appMiddleware.RequestScheme(r, h.TrustedProxies), r.Host)
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
		SELECT w.id, w.original_title, ` + authorLabelFor(people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective) + `, COALESCE(wp.cover_url, ''),
		       COALESCE(wp.file_format, ''), COALESCE(wp.file_path, ''), w.created_at, wp.file_id, COALESCE(wp.file_mode, '')
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
		var fileID sql.NullInt64
		var mode string
		if err := rows.Scan(&id, &title, &author, &coverURL, &format, &filePath, &createdAt, &fileID, &mode); err != nil {
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

		href := fileHref(fileID, mode, filePath)

		if coverURL == "" {
			coverURL = "/covers/placeholder.svg"
		}

		entries.WriteString(fmt.Sprintf("  <entry>\n"))
		entries.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		entries.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-work-%d</id>\n", id))
		entries.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updated))
		entries.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		entries.WriteString(fmt.Sprintf("    <dc:identifier>%d</dc:identifier>\n", id))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s%s\" type=\"%s\"/>\n", base, href, acqType))
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
		SELECT w.id, w.original_title, `+authorLabelFor(people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective)+`, COALESCE(wp.cover_url, ''),
		       COALESCE(wp.file_format, ''), COALESCE(wp.file_path, ''), w.created_at, wp.file_id, COALESCE(wp.file_mode, '')
		`+catalogFrom+`
		WHERE w.retired_at IS NULL
		  AND (LOWER(w.original_title) LIKE LOWER($1) OR `+authorMatches("$1")+`)
		ORDER BY w.id DESC LIMIT 50
	`, catalogSearchPattern(query))
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
		var fileID sql.NullInt64
		var mode string
		if err := rows.Scan(&id, &title, &author, &coverURL, &format, &filePath, &createdAt, &fileID, &mode); err != nil {
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

		href := fileHref(fileID, mode, filePath)

		if coverURL == "" {
			coverURL = "/covers/placeholder.svg"
		}

		entries.WriteString(fmt.Sprintf("  <entry>\n"))
		entries.WriteString(fmt.Sprintf("    <title>%s</title>\n", html.EscapeString(title)))
		entries.WriteString(fmt.Sprintf("    <id>urn:uuid:codice-work-%d</id>\n", id))
		entries.WriteString(fmt.Sprintf("    <updated>%s</updated>\n", updated))
		entries.WriteString(fmt.Sprintf("    <author><name>%s</name></author>\n", html.EscapeString(author)))
		entries.WriteString(fmt.Sprintf("    <dc:identifier>%d</dc:identifier>\n", id))
		entries.WriteString(fmt.Sprintf("    <link rel=\"http://opds-spec.org/acquisition\" href=\"%s%s\" type=\"%s\"/>\n", base, href, acqType))
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
