package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// opdsNav is an entry that leads to another feed (a section of the catalog, an author, a series), rather than to a book.
type opdsNav struct {
	Title   string
	ID      string // what makes its id: "author-12"
	Href    string // from the root of the site, with its query
	Summary string
	Count   int // how many works it leads to; 0 leaves it unsaid
	// Navigation says it leads to a list of other feeds (the authors) and not to a list of books.
	Navigation bool
	// Cover is the picture of the entry (the cover of a series), "" for an entry that has none.
	Cover string
}

func opdsNavEntry(base string, n opdsNav) string {
	var b strings.Builder
	b.WriteString("  <entry>\n")
	fmt.Fprintf(&b, "    <title>%s</title>\n", html.EscapeString(n.Title))
	fmt.Fprintf(&b, "    <id>urn:uuid:codice-%s</id>\n", html.EscapeString(n.ID))
	fmt.Fprintf(&b, "    <updated>%s</updated>\n", time.Now().Format(time.RFC3339))
	summary := n.Summary
	if n.Count > 0 {
		count := fmt.Sprintf("%d obra", n.Count)
		if n.Count != 1 {
			count += "s"
		}
		if summary != "" {
			summary = count + ". " + summary
		} else {
			summary = count
		}
	}
	if summary != "" {
		fmt.Fprintf(&b, "    <content type=\"text\">%s</content>\n", html.EscapeString(summary))
	}
	kind := opdsAcquisition
	if n.Navigation {
		kind = opdsNavigation
	}
	link := opdsLink("subsection", base+n.Href, kind)
	if n.Count > 0 {
		link = strings.Replace(link, "/>", fmt.Sprintf(" thr:count=\"%d\"/>", n.Count), 1)
	}
	b.WriteString("  " + link)
	if n.Cover != "" {
		b.WriteString(opdsImageLinks(base, n.Cover))
	}
	b.WriteString("  </entry>\n")
	return b.String()
}

// opdsNavFeed writes a page of entries that lead to other feeds, with the links to walk the rest.
func (h *OPDSHandler) opdsNavFeed(w http.ResponseWriter, r *http.Request, id, title, self string, entries []opdsNav, total, page int) {
	base := h.baseURL(r)
	at := func(p int) string {
		if p > 1 {
			return base + self + "?page=" + strconv.Itoa(p)
		}
		return base + self
	}
	last := (total + opdsPageSize - 1) / opdsPageSize
	if last < 1 {
		last = 1
	}
	var b strings.Builder
	b.WriteString(opdsFeedOpen(id, title, time.Now()))
	b.WriteString(opdsLink("self", at(page), opdsNavigation))
	b.WriteString(opdsLink("start", base+"/opds/v1.2/catalog", opdsNavigation))
	b.WriteString(opdsLink("first", at(1), opdsNavigation))
	if page > 1 {
		b.WriteString(opdsLink("previous", at(page-1), opdsNavigation))
	}
	if page < last {
		b.WriteString(opdsLink("next", at(page+1), opdsNavigation))
	}
	b.WriteString(opdsLink("last", at(last), opdsNavigation))
	fmt.Fprintf(&b, "  <opensearch:totalResults>%d</opensearch:totalResults>\n", total)
	fmt.Fprintf(&b, "  <opensearch:itemsPerPage>%d</opensearch:itemsPerPage>\n", opdsPageSize)
	fmt.Fprintf(&b, "  <opensearch:startIndex>%d</opensearch:startIndex>\n", (page-1)*opdsPageSize+1)
	for _, n := range entries {
		b.WriteString(opdsNavEntry(base, n))
	}
	b.WriteString("</feed>\n")
	h.writeFeed(w, b.String())
}

// opdsIDParam is the number in the address, or false (and a 404) when it is not one.
func opdsIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "Not found", http.StatusNotFound)
		return 0, false
	}
	return id, true
}

// inProgress is the condition (the caller is $1) for a work they began, did not finish, and did not mark as finished: the same reading
// the library calls "in progress", in whatever edition or format.
const opdsInProgress = `
	EXISTS (SELECT 1 FROM reading_progress r JOIN files f2 ON f2.id = r.file_id JOIN editions e2 ON e2.id = f2.edition_id
	        WHERE e2.work_id = w.id AND r.user_id = $1::uuid AND f2.availability = 'available'
	          AND ` + "(r.position <> '' OR r.locator IS NOT NULL OR r.percent_complete > 0)" + ` AND r.completed_at IS NULL)
	AND NOT EXISTS (SELECT 1 FROM work_reading_state wrs WHERE wrs.work_id = w.id AND wrs.user_id = $1::uuid)`

// opdsLastRead orders the works the caller read the most recently first.
const opdsLastRead = `(SELECT max(GREATEST(COALESCE(r.last_opened_at, '-infinity'::timestamptz), r.updated_at))
	FROM reading_progress r JOIN files f2 ON f2.id = r.file_id JOIN editions e2 ON e2.id = f2.edition_id
	WHERE e2.work_id = w.id AND r.user_id = $1::uuid) DESC NULLS LAST`

// ReadingFeed is what the caller has begun and not finished, the one read last first.
func (h *OPDSHandler) ReadingFeed(w http.ResponseWriter, r *http.Request) {
	page := opdsPageNumber(r)
	user := currentUserID(r)
	works, total, err := h.opdsPage(r.Context(), user, opdsInProgress, opdsLastRead, []any{user}, page)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, "urn:uuid:codice-reading", "Continuar lendo", "/opds/v1.2/reading", nil, works, total, page)
}

// FavoritesFeed is the works the caller marked, the last marked first.
func (h *OPDSHandler) FavoritesFeed(w http.ResponseWriter, r *http.Request) {
	page := opdsPageNumber(r)
	user := currentUserID(r)
	works, total, err := h.opdsPage(r.Context(), user,
		`EXISTS (SELECT 1 FROM favorites fv WHERE fv.work_id = w.id AND fv.user_id = $1::uuid)`,
		`(SELECT fv.created_at FROM favorites fv WHERE fv.work_id = w.id AND fv.user_id = $1::uuid) DESC`, []any{user}, page)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, "urn:uuid:codice-favorites", "Favoritos", "/opds/v1.2/favorites", nil, works, total, page)
}

// opdsNavPage reads one page of a navigation list: `query` selects id, name and how many works, ordered, and takes limit and offset as
// its last two placeholders; `count` says how many there are.
func (h *OPDSHandler) opdsNavPage(ctx context.Context, count, query string, args []any, page int, href func(id int64) string, idPrefix string) ([]opdsNav, int, error) {
	var total int
	if err := h.DB.QueryRowContext(ctx, count, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	all := append(append([]any{}, args...), opdsPageSize, (page-1)*opdsPageSize)
	rows, err := h.DB.QueryContext(ctx, query, all...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []opdsNav
	for rows.Next() {
		var id int64
		var name string
		var n int
		if err := rows.Scan(&id, &name, &n); err != nil {
			return nil, 0, err
		}
		out = append(out, opdsNav{Title: name, ID: idPrefix + strconv.FormatInt(id, 10), Href: href(id), Count: n})
	}
	return out, total, rows.Err()
}

// AuthorsFeed lists the people who wrote something in the library, by the name as the caller prefers to read it.
func (h *OPDSHandler) AuthorsFeed(w http.ResponseWriter, r *http.Request) {
	page := opdsPageNumber(r)
	label := `p.name`
	if people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective == people.FamilyFirst {
		label = `COALESCE(p.family_name || COALESCE(', ' || p.given_name, ''), p.name)`
	}
	from := ` FROM person p
		JOIN work_contributors c ON c.person_id = p.id AND c.role = 'author'
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL`
	entries, total, err := h.opdsNavPage(r.Context(),
		`SELECT count(DISTINCT p.id)`+from,
		`SELECT p.id, `+label+`, count(DISTINCT w.id)`+from+` GROUP BY p.id ORDER BY unaccent(lower(`+label+`)), p.id LIMIT $1 OFFSET $2`,
		nil, page, func(id int64) string { return "/opds/v1.2/authors/" + strconv.FormatInt(id, 10) }, "author-")
	if err != nil {
		http.Error(w, "Error fetching authors", http.StatusInternalServerError)
		return
	}
	h.opdsNavFeed(w, r, "urn:uuid:codice-authors", "Autores", "/opds/v1.2/authors", entries, total, page)
}

// AuthorFeed is the works of one author.
func (h *OPDSHandler) AuthorFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := opdsIDParam(w, r)
	if !ok {
		return
	}
	var name string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT name FROM person WHERE id = $1`, id).Scan(&name); err != nil {
		http.Error(w, "Author not found", http.StatusNotFound)
		return
	}
	page := opdsPageNumber(r)
	works, total, err := h.opdsPage(r.Context(), currentUserID(r),
		`EXISTS (SELECT 1 FROM work_contributors c WHERE c.work_id = w.id AND c.role = 'author' AND c.person_id = $1)`,
		`w.original_title`, []any{id}, page)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, fmt.Sprintf("urn:uuid:codice-author-%d", id), name, fmt.Sprintf("/opds/v1.2/authors/%d", id), nil, works, total, page)
}

// SeriesFeed lists the official collections (the series) that have works in them.
func (h *OPDSHandler) SeriesFeed(w http.ResponseWriter, r *http.Request) {
	page := opdsPageNumber(r)
	from := ` FROM collections c
		JOIN collection_works cw ON cw.collection_id = c.id
		JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
		WHERE c.kind = 'official' AND c.retired_at IS NULL`
	entries, total, err := h.opdsNavPage(r.Context(),
		`SELECT count(DISTINCT c.id)`+from,
		`SELECT c.id, c.name, count(DISTINCT w.id)`+from+` GROUP BY c.id ORDER BY unaccent(lower(c.name)), c.id LIMIT $1 OFFSET $2`,
		nil, page, func(id int64) string { return "/opds/v1.2/series/" + strconv.FormatInt(id, 10) }, "series-")
	if err != nil {
		http.Error(w, "Error fetching series", http.StatusInternalServerError)
		return
	}
	h.seriesCovers(r.Context(), entries)
	h.opdsNavFeed(w, r, "urn:uuid:codice-series", "Séries", "/opds/v1.2/series", entries, total, page)
}

// seriesCovers gives each series of a page its cover (DEC-201): the one the staff chose, or the cover of its first work that has one. A series
// with none keeps no picture, and a failure here only leaves the pictures out.
func (h *OPDSHandler) seriesCovers(ctx context.Context, entries []opdsNav) {
	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		if id, err := strconv.ParseInt(strings.TrimPrefix(e.ID, "series-"), 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	rows, err := h.DB.QueryContext(ctx, `
		SELECT c.id, COALESCE(NULLIF(c.cover_url, ''), (array_agg(wp.cover_url ORDER BY cw.position NULLS LAST, w.original_title, w.id)
		                                                 FILTER (WHERE wp.cover_url IS NOT NULL AND wp.cover_url <> ''))[1], '')
		FROM collections c
		JOIN collection_works cw ON cw.collection_id = c.id
		JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
		LEFT JOIN work_primary wp ON wp.work_id = w.id
		WHERE c.id = ANY($1)
		GROUP BY c.id, c.cover_url`, pq.Array(ids))
	if err != nil {
		log.Println("Error fetching the covers of the series:", err)
		return
	}
	defer rows.Close()
	covers := map[string]string{}
	for rows.Next() {
		var id int64
		var cover string
		if rows.Scan(&id, &cover) == nil && cover != "" {
			covers["series-"+strconv.FormatInt(id, 10)] = cover
		}
	}
	for i := range entries {
		entries[i].Cover = covers[entries[i].ID]
	}
}

// collectionWorksFeed is the works of a collection, in the order the collection gives them (the number in the series, then the order
// they came in).
func (h *OPDSHandler) collectionWorksFeed(w http.ResponseWriter, r *http.Request, id int64, name, self string) {
	page := opdsPageNumber(r)
	works, total, err := h.opdsPage(r.Context(), currentUserID(r),
		`w.id IN (SELECT cw.work_id FROM collection_works cw WHERE cw.collection_id = $1)`,
		`(SELECT cw.position FROM collection_works cw WHERE cw.collection_id = $1 AND cw.work_id = w.id) ASC NULLS LAST, `+
			`(SELECT cw.added_at FROM collection_works cw WHERE cw.collection_id = $1 AND cw.work_id = w.id) ASC`, []any{id}, page)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, fmt.Sprintf("urn:uuid:codice-collection-%d", id), name, self, nil, works, total, page)
}

// SeriesWorksFeed is one series.
func (h *OPDSHandler) SeriesWorksFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := opdsIDParam(w, r)
	if !ok {
		return
	}
	var name string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT name FROM collections WHERE id = $1 AND kind = 'official' AND retired_at IS NULL`, id).Scan(&name); err != nil {
		http.Error(w, "Series not found", http.StatusNotFound)
		return
	}
	h.collectionWorksFeed(w, r, id, name, fmt.Sprintf("/opds/v1.2/series/%d", id))
}

// ListsFeed lists the caller's own lists, "Ler depois" first.
func (h *OPDSHandler) ListsFeed(w http.ResponseWriter, r *http.Request) {
	page := opdsPageNumber(r)
	user := currentUserID(r)
	from := ` FROM collections c LEFT JOIN collection_works cw ON cw.collection_id = c.id
		LEFT JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
		WHERE c.kind = 'personal' AND c.owner_id = $1::uuid AND c.retired_at IS NULL`
	entries, total, err := h.opdsNavPage(r.Context(),
		`SELECT count(DISTINCT c.id)`+from,
		`SELECT c.id, c.name, count(DISTINCT w.id)`+from+` GROUP BY c.id ORDER BY (c.system_key IS NULL), unaccent(lower(c.name)), c.id LIMIT $2 OFFSET $3`,
		[]any{user}, page, func(id int64) string { return "/opds/v1.2/lists/" + strconv.FormatInt(id, 10) }, "list-")
	if err != nil {
		http.Error(w, "Error fetching lists", http.StatusInternalServerError)
		return
	}
	h.opdsNavFeed(w, r, "urn:uuid:codice-lists", "Minhas listas", "/opds/v1.2/lists", entries, total, page)
}

// ListWorksFeed is one list of the caller's. A list of someone else is not found.
func (h *OPDSHandler) ListWorksFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := opdsIDParam(w, r)
	if !ok {
		return
	}
	var name string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT name FROM collections WHERE id = $1 AND kind = 'personal' AND owner_id = $2::uuid AND retired_at IS NULL`, id, currentUserID(r)).Scan(&name)
	if err != nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}
	h.collectionWorksFeed(w, r, id, name, fmt.Sprintf("/opds/v1.2/lists/%d", id))
}

// categoryCounts lists the categories under `parent` (the top ones when it is 0) that have works somewhere beneath them, with how many.
func (h *OPDSHandler) categoryCounts(ctx context.Context, parent int64) ([]opdsNav, error) {
	var parentArg sql.NullInt64
	if parent > 0 {
		parentArg = sql.NullInt64{Int64: parent, Valid: true}
	}
	rows, err := h.DB.QueryContext(ctx, `
		WITH RECURSIVE tree(root, id) AS (
			SELECT id, id FROM categories WHERE parent_id IS NOT DISTINCT FROM $1
			UNION ALL
			SELECT t.root, c.id FROM categories c JOIN tree t ON c.parent_id = t.id)
		SELECT c.id, c.name, (SELECT count(DISTINCT wc.work_id) FROM tree t
		                      JOIN work_categories wc ON wc.category_id = t.id
		                      JOIN works w ON w.id = wc.work_id AND w.retired_at IS NULL
		                      WHERE t.root = c.id)
		FROM categories c WHERE c.parent_id IS NOT DISTINCT FROM $1
		ORDER BY unaccent(lower(c.name)), c.id`, parentArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []opdsNav
	for rows.Next() {
		var id int64
		var name string
		var n int
		if err := rows.Scan(&id, &name, &n); err != nil {
			return nil, err
		}
		if n > 0 {
			out = append(out, opdsNav{Title: name, ID: "category-" + strconv.FormatInt(id, 10), Href: "/opds/v1.2/categories/" + strconv.FormatInt(id, 10), Count: n})
		}
	}
	return out, rows.Err()
}

// CategoriesFeed lists the top categories (those with something in them).
func (h *OPDSHandler) CategoriesFeed(w http.ResponseWriter, r *http.Request) {
	entries, err := h.categoryCounts(r.Context(), 0)
	if err != nil {
		http.Error(w, "Error fetching categories", http.StatusInternalServerError)
		return
	}
	page := opdsPageNumber(r)
	from := (page - 1) * opdsPageSize
	if from > len(entries) {
		from = len(entries)
	}
	to := from + opdsPageSize
	if to > len(entries) {
		to = len(entries)
	}
	h.opdsNavFeed(w, r, "urn:uuid:codice-categories", "Categorias", "/opds/v1.2/categories", entries[from:to], len(entries), page)
}

// CategoryFeed is one category: the categories inside it first (on the first page), then the works of it and of everything beneath it.
func (h *OPDSHandler) CategoryFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := opdsIDParam(w, r)
	if !ok {
		return
	}
	var name string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT name FROM categories WHERE id = $1`, id).Scan(&name); err != nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	children, err := h.categoryCounts(r.Context(), id)
	if err != nil {
		http.Error(w, "Error fetching categories", http.StatusInternalServerError)
		return
	}
	page := opdsPageNumber(r)
	works, total, err := h.opdsPage(r.Context(), currentUserID(r),
		`w.id IN (SELECT wc.work_id FROM work_categories wc WHERE wc.category_id IN (
			WITH RECURSIVE down AS (SELECT id FROM categories WHERE id = $1 UNION ALL SELECT c.id FROM categories c JOIN down d ON c.parent_id = d.id)
			SELECT id FROM down))`,
		`w.original_title`, []any{id}, page)
	if err != nil {
		http.Error(w, "Error fetching works", http.StatusInternalServerError)
		return
	}
	h.opdsAcquisitionFeed(w, r, fmt.Sprintf("urn:uuid:codice-category-%d", id), name, fmt.Sprintf("/opds/v1.2/categories/%d", id), url.Values{}, works, total, page, children...)
}
