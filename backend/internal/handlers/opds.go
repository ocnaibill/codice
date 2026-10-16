package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/people"

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

const (
	opdsNavigation  = "application/atom+xml;profile=opds-catalog;kind=navigation"
	opdsAcquisition = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	opdsPageSize    = 30 // a page of a feed: small enough for an e-reader to draw at once
)

// opdsFeedOpen is the start of every feed. The three prefixes are declared in all of them, so that no element can use one that is
// not (a feed with an undeclared prefix is not well formed, and a strict client refuses it).
func opdsFeedOpen(id, title string, now time.Time) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom"` + "\n")
	b.WriteString(`      xmlns:dc="http://purl.org/dc/terms/"` + "\n")
	b.WriteString(`      xmlns:opds="http://opds-spec.org/2010/catalog"` + "\n")
	b.WriteString(`      xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/"` + "\n")
	b.WriteString(`      xmlns:thr="http://purl.org/syndication/thread/1.0">` + "\n")
	fmt.Fprintf(&b, "  <id>%s</id>\n  <title>%s</title>\n  <updated>%s</updated>\n", html.EscapeString(id), html.EscapeString(title), now.Format(time.RFC3339))
	return b.String()
}

func opdsLink(rel, href, typ string) string {
	if rel != "" {
		rel = fmt.Sprintf(` rel="%s"`, html.EscapeString(rel))
	}
	return fmt.Sprintf("  <link%s href=\"%s\" type=\"%s\"/>\n", rel, html.EscapeString(href), html.EscapeString(typ))
}

func (h *OPDSHandler) writeFeed(w http.ResponseWriter, feed string) {
	w.Header().Set("Content-Type", "application/atom+xml;charset=utf-8")
	w.Write([]byte(feed))
}

func (h *OPDSHandler) RootCatalog(w http.ResponseWriter, r *http.Request) {
	base := h.baseURL(r)
	now := time.Now()

	var b strings.Builder
	b.WriteString(opdsFeedOpen("urn:uuid:codice-catalog", "Códice", now))
	b.WriteString(opdsLink("self", base+"/opds/v1.2/catalog", opdsNavigation))
	b.WriteString(opdsLink("start", base+"/opds/v1.2/catalog", opdsNavigation))
	b.WriteString(opdsLink("search", base+"/opds/v1.2/search?q={searchTerms}", opdsAcquisition))
	for _, n := range []opdsNav{
		{Title: "Continuar lendo", ID: "reading", Href: "/opds/v1.2/reading", Summary: "O que você começou e ainda não terminou."},
		{Title: "Adicionados recentemente", ID: "recent", Href: "/opds/v1.2/recent"},
		{Title: "Favoritos", ID: "favorites", Href: "/opds/v1.2/favorites", Summary: "As obras que você marcou."},
		{Title: "Minhas listas", ID: "lists", Href: "/opds/v1.2/lists", Navigation: true, Summary: "Ler depois e as listas que você fez."},
		{Title: "Séries", ID: "series", Href: "/opds/v1.2/series", Navigation: true},
		{Title: "Autores", ID: "authors", Href: "/opds/v1.2/authors", Navigation: true},
		{Title: "Categorias", ID: "categories", Href: "/opds/v1.2/categories", Navigation: true},
	} {
		b.WriteString(opdsNavEntry(base, n))
	}
	b.WriteString("</feed>\n")
	h.writeFeed(w, b.String())
}

// opdsFile is one file of a work, as an acquisition link.
type opdsFile struct {
	Format string
	Href   string
	Size   int64
}

// opdsWork is one entry of a feed.
type opdsWork struct {
	ID       int
	Title    string
	Author   string
	Cover    string
	Summary  string
	Language string
	Updated  time.Time
	Files    []opdsFile
}

// opdsMime is what a client is told a file is: the type of its format.
func opdsMime(format string) string {
	switch strings.ToLower(format) {
	case "epub":
		return "application/epub+zip"
	case "pdf":
		return "application/pdf"
	case "mobi":
		return "application/x-mobipocket-ebook"
	case "cbz":
		return "application/x-cbz"
	case "cbr":
		return "application/x-cbr"
	case "txt":
		return "text/plain"
	case "md":
		return "text/markdown"
	case "mp3":
		return "audio/mpeg"
	case "m4a", "m4b":
		return "audio/mp4"
	case "ogg":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	}
	return "application/octet-stream"
}

// opdsFormatOrder puts the formats an e-reader opens best first: the EPUB before the PDF, the PDF before the rest.
func opdsFormatOrder(format string) int {
	for i, f := range []string{"epub", "mobi", "pdf", "cbz", "cbr", "txt", "md"} {
		if strings.EqualFold(format, f) {
			return i
		}
	}
	return 99
}

// opdsCoverMime is the type of a cover by its extension (a placeholder is an SVG, a cover is almost always a JPEG).
func opdsCoverMime(cover string) string {
	switch strings.ToLower(path.Ext(strings.SplitN(cover, "?", 2)[0])) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	}
	return "image/jpeg"
}

// opdsPage reads one page of the works that `condition` keeps (a piece of SQL over `w`, with the placeholders $1.. of `args`), newest
// first, with all the files of each, and how many works there are in all.
func (h *OPDSHandler) opdsPage(ctx context.Context, userID, condition, order string, args []any, page int) ([]opdsWork, int, error) {
	if order == "" {
		order = "w.id DESC"
	}
	var total int
	if err := h.DB.QueryRowContext(ctx, `SELECT count(*) FROM works w WHERE w.retired_at IS NULL AND `+condition, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := len(args)+1, len(args)+2
	rows, err := h.DB.QueryContext(ctx, `
		SELECT w.id, w.original_title, `+authorLabelFor(people.OrderFor(ctx, h.DB, userID).Effective)+`, COALESCE(wp.cover_url, ''),
		       w.created_at, COALESCE(left(w.description, 1200), ''),
		       COALESCE((SELECT e.language FROM editions e WHERE e.id = wp.edition_id), '')
		`+catalogFrom+`
		WHERE w.retired_at IS NULL AND `+condition+`
		ORDER BY `+order+`, w.id DESC LIMIT $`+strconv.Itoa(limit)+` OFFSET $`+strconv.Itoa(offset), append(append([]any{}, args...), opdsPageSize, (page-1)*opdsPageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var works []opdsWork
	byID := map[int]*opdsWork{}
	var ids []int64
	for rows.Next() {
		var wk opdsWork
		var created sql.NullTime
		if err := rows.Scan(&wk.ID, &wk.Title, &wk.Author, &wk.Cover, &created, &wk.Summary, &wk.Language); err != nil {
			return nil, 0, err
		}
		if created.Valid {
			wk.Updated = created.Time
		} else {
			wk.Updated = time.Now()
		}
		works = append(works, wk)
		ids = append(ids, int64(wk.ID))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range works {
		byID[works[i].ID] = &works[i]
	}
	if len(ids) == 0 {
		return works, total, nil
	}
	files, err := h.DB.QueryContext(ctx, `
		SELECT e.work_id, f.id, COALESCE(f.format, ''), COALESCE(f.size_bytes, 0), COALESCE(l.mode, ''), COALESCE(l.path, '')
		FROM editions e
		JOIN files f ON f.edition_id = e.id
		LEFT JOIN LATERAL (SELECT mode, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1) l ON TRUE
		WHERE e.work_id = ANY($1) AND f.availability = 'available'
		ORDER BY e.work_id, e.is_primary DESC, f.id`, pq.Array(ids))
	if err != nil {
		return nil, 0, err
	}
	defer files.Close()
	for files.Next() {
		var workID int
		var fileID sql.NullInt64
		var format, mode, rel string
		var size int64
		if err := files.Scan(&workID, &fileID.Int64, &format, &size, &mode, &rel); err != nil {
			return nil, 0, err
		}
		fileID.Valid = true
		if href := fileHref(fileID, mode, rel); href != "" {
			if wk := byID[workID]; wk != nil {
				wk.Files = append(wk.Files, opdsFile{Format: format, Href: href, Size: size})
			}
		}
	}
	for i := range works {
		fs := works[i].Files
		for a := 1; a < len(fs); a++ { // stable by format: the primary file stays first among files of the same format
			for b := a; b > 0 && opdsFormatOrder(fs[b].Format) < opdsFormatOrder(fs[b-1].Format); b-- {
				fs[b], fs[b-1] = fs[b-1], fs[b]
			}
		}
	}
	return works, total, files.Err()
}

func opdsEntry(base string, wk opdsWork) string {
	var b strings.Builder
	b.WriteString("  <entry>\n")
	fmt.Fprintf(&b, "    <title>%s</title>\n", html.EscapeString(wk.Title))
	fmt.Fprintf(&b, "    <id>urn:uuid:codice-work-%d</id>\n", wk.ID)
	fmt.Fprintf(&b, "    <updated>%s</updated>\n", wk.Updated.Format(time.RFC3339))
	if wk.Author != "" {
		fmt.Fprintf(&b, "    <author><name>%s</name></author>\n", html.EscapeString(wk.Author))
	}
	fmt.Fprintf(&b, "    <dc:identifier>%d</dc:identifier>\n", wk.ID)
	if wk.Language != "" {
		fmt.Fprintf(&b, "    <dc:language>%s</dc:language>\n", html.EscapeString(wk.Language))
	}
	if s := strings.TrimSpace(wk.Summary); s != "" {
		fmt.Fprintf(&b, "    <content type=\"text\">%s</content>\n", html.EscapeString(s))
	}
	for _, f := range wk.Files {
		link := opdsLink("http://opds-spec.org/acquisition", base+f.Href, opdsMime(f.Format))
		if f.Size > 0 {
			link = strings.Replace(link, "/>", fmt.Sprintf(" length=\"%d\"/>", f.Size), 1)
		}
		b.WriteString("  " + link)
	}
	cover := wk.Cover
	if cover == "" {
		cover = "/covers/placeholder.svg"
	}
	if !strings.Contains(cover, "://") {
		cover = base + cover
	}
	b.WriteString("  " + opdsLink("http://opds-spec.org/image", cover, opdsCoverMime(cover)))
	// the same cover reduced, for a small screen on a home network (a cover that is not a picture the server reduces comes whole)
	if opdsCoverMime(cover) != "image/svg+xml" && strings.HasPrefix(cover, base+"/covers/") {
		sep := "?"
		if strings.Contains(cover, "?") {
			sep = "&"
		}
		b.WriteString("  " + opdsLink("http://opds-spec.org/image/thumbnail", cover+sep+"w=240", "image/jpeg"))
	}
	b.WriteString("  </entry>\n")
	return b.String()
}

// opdsPageNumber is the page asked for: from 1, and a nonsense one is the first.
func opdsPageNumber(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 || n > 100000 {
		return 1
	}
	return n
}

// opdsAcquisitionFeed writes a page of works with the links to walk the others (first, previous, next, last) and the totals a client
// shows ("1 to 30 of 412").
func (h *OPDSHandler) opdsAcquisitionFeed(w http.ResponseWriter, r *http.Request, id, title, self string, query url.Values, works []opdsWork, total, page int, sections ...opdsNav) {
	base := h.baseURL(r)
	at := func(p int) string {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		if p > 1 {
			q.Set("page", strconv.Itoa(p))
		}
		if len(q) == 0 {
			return base + self
		}
		return base + self + "?" + q.Encode()
	}
	last := (total + opdsPageSize - 1) / opdsPageSize
	if last < 1 {
		last = 1
	}
	var b strings.Builder
	b.WriteString(opdsFeedOpen(id, title, time.Now()))
	b.WriteString(opdsLink("self", at(page), opdsAcquisition))
	b.WriteString(opdsLink("start", base+"/opds/v1.2/catalog", opdsNavigation))
	b.WriteString(opdsLink("first", at(1), opdsAcquisition))
	if page > 1 {
		b.WriteString(opdsLink("previous", at(page-1), opdsAcquisition))
	}
	if page < last {
		b.WriteString(opdsLink("next", at(page+1), opdsAcquisition))
	}
	b.WriteString(opdsLink("last", at(last), opdsAcquisition))
	fmt.Fprintf(&b, "  <opensearch:totalResults>%d</opensearch:totalResults>\n", total)
	fmt.Fprintf(&b, "  <opensearch:itemsPerPage>%d</opensearch:itemsPerPage>\n", opdsPageSize)
	fmt.Fprintf(&b, "  <opensearch:startIndex>%d</opensearch:startIndex>\n", (page-1)*opdsPageSize+1)
	if page == 1 { // the sub-sections (the categories inside a category) come before the works, on the first page only
		for _, n := range sections {
			b.WriteString(opdsNavEntry(base, n))
		}
	}
	for _, wk := range works {
		b.WriteString(opdsEntry(base, wk))
	}
	b.WriteString("</feed>\n")
	h.writeFeed(w, b.String())
}

func (h *OPDSHandler) RecentFeed(w http.ResponseWriter, r *http.Request) {
	page := opdsPageNumber(r)
	works, total, err := h.opdsPage(r.Context(), currentUserID(r), "TRUE", "", nil, page)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, "urn:uuid:codice-recent", "Adicionados recentemente", "/opds/v1.2/recent", nil, works, total, page)
}

func (h *OPDSHandler) SearchFeed(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing search query", http.StatusBadRequest)
		return
	}
	page := opdsPageNumber(r)
	works, total, err := h.opdsPage(r.Context(), currentUserID(r), "("+titleMatches("$1")+" OR "+authorMatches("$1")+")", "", []any{catalogSearchPattern(query)}, page)
	if err != nil {
		http.Error(w, "Error searching", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, "urn:uuid:codice-search", "Resultados da busca", "/opds/v1.2/search", url.Values{"q": {query}}, works, total, page)
}
