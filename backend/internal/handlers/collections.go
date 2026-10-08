package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// CollectionsHandler reads the collections (#184, DEC-130): the official ones of the library, which everybody sees, and
// the personal ones, which only their owner does.
type CollectionsHandler struct{ DB *sql.DB }

// Collection is a collection as the lists show it.
type Collection struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	WorkCount      int    `json:"workCount"`
	CompletedCount int    `json:"completedCount"`
	CoverURL       string `json:"coverUrl"`
	Retired        bool   `json:"retired,omitempty"`
}

// CollectionWork is one place of a collection, in its order. A work that is not available (in the trash, or deleted for good,
// which only a personal list still shows) has no ID to open, and says what it was by the label it kept.
type CollectionWork struct {
	EntryID   int64    `json:"entryId"`
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	CoverURL  string   `json:"coverUrl"`
	Position  *float64 `json:"position"`
	Completed bool     `json:"completed"`
	Available bool     `json:"available"`
}

// visibleCollection is the condition for a collection (alias c) the caller ($1) may see: an official one that is not
// retired, or one of their own. The retired ones are for the staff, who ask for them apart.
const visibleCollection = `((c.kind = 'official' AND c.retired_at IS NULL) OR c.owner_id = $1::uuid)`

// A collection born from the series of a work, that no work has now, is not worth a card: it is shown once a person has
// made it theirs (renamed or made it by hand), or while it has works in it.
const shownCollection = `(c.origin = 'manual' OR c.edited_at IS NOT NULL OR EXISTS (
	SELECT 1 FROM collection_works cw JOIN works w ON w.id = cw.work_id WHERE cw.collection_id = c.id AND w.retired_at IS NULL))`

// List answers GET /collections?page=&limit=&kind=: the official collections, by name, or with kind=personal the caller's own.
func (h *CollectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	page, limit := 1, 50
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}

	// The official ones, unless the person asks for their own (what the kind says otherwise is not a kind, and gets the official
	// ones). The retired ones are listed apart, to restore them: the staff's for the official, the person's for their own.
	retired := r.URL.Query().Get("retired") == "true"
	where := `c.kind = 'official' AND c.retired_at IS NULL AND ` + shownCollection + ` AND $1::text IS NOT NULL`
	switch {
	case r.URL.Query().Get("kind") == "personal" && retired:
		where = `c.kind = 'personal' AND c.owner_id = $1::uuid AND c.retired_at IS NOT NULL`
	case r.URL.Query().Get("kind") == "personal":
		where = `c.kind = 'personal' AND c.owner_id = $1::uuid AND c.retired_at IS NULL`
	case retired && isStaffRequest(r):
		where = `c.kind = 'official' AND c.retired_at IS NOT NULL AND $1::text IS NOT NULL`
	}

	var total int
	if err := h.DB.QueryRow(`SELECT count(*) FROM collections c WHERE `+where, userID).Scan(&total); err != nil {
		log.Println("Error counting collections:", err)
		http.Error(w, "Error fetching collections", http.StatusInternalServerError)
		return
	}
	rows, err := h.DB.Query(`
		SELECT c.id, c.kind, c.name, c.retired_at IS NOT NULL,
		       COALESCE(s.works, 0), COALESCE(s.completed, 0), COALESCE(s.cover, '')
		FROM collections c
		LEFT JOIN LATERAL (
			SELECT count(*) AS works,
			       count(*) FILTER (WHERE rp.completed_at IS NOT NULL) AS completed,
			       (array_agg(wp.cover_url ORDER BY cw.position NULLS LAST, w.original_title, w.id)
			         FILTER (WHERE wp.cover_url IS NOT NULL AND wp.cover_url <> ''))[1] AS cover
			FROM collection_works cw
			JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
			LEFT JOIN work_primary wp ON wp.work_id = w.id
			LEFT JOIN reading_progress rp ON rp.file_id = wp.file_id AND rp.user_id = $1::uuid
			WHERE cw.collection_id = c.id
		) s ON TRUE
		WHERE `+where+`
		ORDER BY lower(c.name), c.id
		LIMIT $2 OFFSET $3`, userID, limit, (page-1)*limit)
	if err != nil {
		log.Println("Error fetching collections:", err)
		http.Error(w, "Error fetching collections", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []Collection{}
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.Retired, &c.WorkCount, &c.CompletedCount, &c.CoverURL); err != nil {
			log.Println("Error scanning collection:", err)
			continue
		}
		if c.CoverURL == "" {
			c.CoverURL = "/covers/placeholder.svg"
		}
		items = append(items, c)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data": items, "total": total, "page": page, "limit": limit, "totalPages": (total + limit - 1) / limit,
	})
}

// Get answers GET /collections/{id}: the collection and its works, in order (by number, those without one last by title).
// What the caller may not see is not found.
func (h *CollectionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}

	// The staff can open a retired collection too.
	visible := visibleCollection
	if isStaffRequest(r) {
		visible = `(c.kind = 'official' OR c.owner_id = $1::uuid)`
	}
	var c Collection
	err = h.DB.QueryRow(`SELECT c.id, c.kind, c.name, c.retired_at IS NOT NULL FROM collections c WHERE c.id = $2 AND `+visible, userID, id).
		Scan(&c.ID, &c.Kind, &c.Name, &c.Retired)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error fetching a collection:", err)
		http.Error(w, "Error fetching the collection", http.StatusInternalServerError)
		return
	}

	order := people.OrderFor(r.Context(), h.DB, userID).Effective
	authorNames := "au.names"
	if order == people.FamilyFirst {
		authorNames = "au.names_family"
	}
	// Every place of the collection. An official one shows the works that are available; a personal one also shows the places
	// of works that went to the trash or were deleted for good, with what the list kept of them.
	rows, err := h.DB.Query(`
		SELECT cw.id, COALESCE(w.id, 0), COALESCE(w.original_title, cw.label, ''),
		       COALESCE(`+authorNames+`, cw.author_label, 'Unknown Author'),
		       COALESCE(wp.cover_url, ''), cw.position, (rp.completed_at IS NOT NULL),
		       (w.id IS NOT NULL AND w.retired_at IS NULL)
		FROM collection_works cw
		LEFT JOIN works w ON w.id = cw.work_id
		LEFT JOIN work_primary wp ON wp.work_id = w.id
		LEFT JOIN LATERAL (
			SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name) AS names,
			       string_agg(CASE WHEN p.family_name IS NULL THEN p.name ELSE p.family_name || COALESCE(', ' || p.given_name, '') END,
			                  '; ' ORDER BY c.position, p.name) AS names_family
			FROM work_contributors c JOIN person p ON p.id = c.person_id
			WHERE c.work_id = w.id AND c.role = 'author'
		) au ON TRUE
		LEFT JOIN reading_progress rp ON rp.file_id = wp.file_id AND rp.user_id = $1::uuid
		WHERE cw.collection_id = $2 AND (NOT cw.official OR w.retired_at IS NULL)
		ORDER BY cw.position NULLS LAST, COALESCE(w.original_title, cw.label), cw.id`, userID, id)
	if err != nil {
		log.Println("Error fetching the works of a collection:", err)
		http.Error(w, "Error fetching the collection", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	works := []CollectionWork{}
	for rows.Next() {
		var cw CollectionWork
		if err := rows.Scan(&cw.EntryID, &cw.ID, &cw.Title, &cw.Author, &cw.CoverURL, &cw.Position, &cw.Completed, &cw.Available); err != nil {
			log.Println("Error scanning a work of a collection:", err)
			continue
		}
		if cw.CoverURL == "" {
			cw.CoverURL = "/covers/placeholder.svg"
		}
		works = append(works, cw)
	}
	for _, cw := range works {
		if cw.Available {
			c.WorkCount++
		}
		if cw.Completed {
			c.CompletedCount++
		}
	}
	// The cover of the collection is the first one a work of it has.
	c.CoverURL = "/covers/placeholder.svg"
	for _, cw := range works {
		if cw.Available && cw.CoverURL != "/covers/placeholder.svg" {
			c.CoverURL = cw.CoverURL
			break
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"collection": c, "works": works})
}
