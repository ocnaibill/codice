package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/people"
)

// CollectionsHandler reads the collections (#184, DEC-130): the official ones of the library, which everybody sees, and
// the personal ones, which only their owner does.
type CollectionsHandler struct{ DB *sql.DB }

// Collection is a collection as the lists show it.
type Collection struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Description is what was said of the collection (DEC-163); only the page of one asks for it.
	Description string `json:"description,omitempty"`
	// ReadingDirection is how the series is read ("ltr", "rtl" or "webtoon") when somebody said (DEC-166); empty otherwise.
	ReadingDirection string `json:"readingDirection,omitempty"`
	WorkCount        int    `json:"workCount"`
	CompletedCount   int    `json:"completedCount"`
	CoverURL         string `json:"coverUrl"`
	Retired          bool   `json:"retired,omitempty"`
	IsFavorite       bool   `json:"isFavorite"`
	// System is the key of a list the Códice keeps for the person ("read_later"): it is not renamed or put away. Empty for the others.
	System string `json:"system,omitempty"`
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
	// Unit and ComicKind are what the work is of a series (#187): "volume", "chapter" or "oneshot", and "comic" or "manga"; empty when
	// nobody said.
	Unit      string `json:"unit"`
	ComicKind string `json:"comicKind"`
	// What the caller has done of the work and what it is, for a row of the page of a collection (DEC-162); empty for a place whose work is gone.
	Percent      float64    `json:"percent"`
	Started      bool       `json:"started"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	OriginalYear *int       `json:"originalYear,omitempty"`
	Synopsis     string     `json:"synopsis,omitempty"`
	Formats      []string   `json:"formats"`
	Rating       int        `json:"rating"`
	Chapter      string     `json:"chapter,omitempty"`
	UnitIndex    int        `json:"unitIndex,omitempty"`
	UnitTotal    int        `json:"unitTotal,omitempty"`
	ReadFormat   string     `json:"readFormat,omitempty"`
	// Notes is how many notes and highlights with a passage the caller made on the work (DEC-163).
	Notes int `json:"notes"`
	// Bookmarks is how many places the caller marked in the work (DEC-165).
	Bookmarks int `json:"bookmarks"`
	// VolumeNumber is the bound volume that collected a chapter and StoryArc the arc it is in (DEC-169); empty when nobody said.
	VolumeNumber *float64 `json:"volumeNumber,omitempty"`
	StoryArc     string   `json:"storyArc,omitempty"`
	// seconds is the time the caller spent in the work, for the pace of the whole.
	seconds int
}

// CollectionSummary is what the page of a collection says of the whole (DEC-162): how far the caller is, in how many hours, the years the
// works span, who wrote and translated them and the tags they carry most. The numbers about reading are the caller's own.
type CollectionSummary struct {
	Works          int     `json:"works"`
	Finished       int     `json:"finished"`
	InProgress     int     `json:"inProgress"`
	Percent        float64 `json:"percent"`
	ReadingSeconds int     `json:"readingSeconds"`
	// Notes is how many notes and highlights with a passage the caller made on the works of the collection (DEC-163).
	Notes int `json:"notes"`
	// Bookmarks is how many places the caller marked in the works of the collection (DEC-165).
	Bookmarks int `json:"bookmarks"`
	// RemainingSeconds is how long is left of the sequence at the pace of the caller in it, by unit (DEC-165); absent until the pace is
	// worth saying for all that is left.
	RemainingSeconds int  `json:"remainingSeconds,omitempty"`
	YearFrom         *int `json:"yearFrom,omitempty"`
	YearTo           *int `json:"yearTo,omitempty"`
	// Missing are the numbers of a series that the works skip, by unit: with the volumes 1, 2 and 4 there is a 3 that is not in the library.
	Missing     []MissingNumber `json:"missing"`
	Authors     []PersonRef     `json:"authors"`
	Translators []PersonRef     `json:"translators"`
	Tags        []PersonTag     `json:"tags"`
}

// MissingNumber is a number of a series that no work has, and the unit it is of ("volume", "chapter" or none).
type MissingNumber struct {
	Unit   string  `json:"unit"`
	Number float64 `json:"number"`
}

// PersonRef is a person of a work, to open their page from the page of a collection.
type PersonRef struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Works int    `json:"works"`
}

// visibleCollection is the condition for a collection (alias c) the caller ($1) may see: an official one that is not
// retired, or one of their own. The retired ones are for the staff, who ask for them apart.
const visibleCollection = `((c.kind = 'official' AND c.retired_at IS NULL) OR c.owner_id = $1::uuid)`

// A collection born from the series of a work, that no work has now, is not worth a card: it is shown once a person has
// made it theirs (renamed or made it by hand), or while it has works in it.
const shownCollection = `(c.origin = 'manual' OR c.edited_at IS NOT NULL OR EXISTS (
	SELECT 1 FROM collection_works cw JOIN works w ON w.id = cw.work_id WHERE cw.collection_id = c.id AND w.retired_at IS NULL))`

// collectionStats is what a collection (alias c) says of itself to the caller ($1): how many works are in it, how many of those
// they finished, and the cover of the first that has one (the lateral join is `s`).
const collectionStats = `
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
	) s ON TRUE`

// isFavoriteCollection is the condition for a collection (alias c) to be a favorite of the caller ($1).
const isFavoriteCollection = `EXISTS (SELECT 1 FROM favorite_collections fc WHERE fc.collection_id = c.id AND fc.user_id = $1::uuid)`

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
		       COALESCE(s.works, 0), COALESCE(s.completed, 0), COALESCE(s.cover, ''), `+isFavoriteCollection+`, COALESCE(c.system_key, '')
		FROM collections c`+collectionStats+`
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
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.Retired, &c.WorkCount, &c.CompletedCount, &c.CoverURL, &c.IsFavorite, &c.System); err != nil {
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
	err = h.DB.QueryRow(`SELECT c.id, c.kind, c.name, c.retired_at IS NOT NULL, `+isFavoriteCollection+`, COALESCE(c.system_key, ''), COALESCE(c.description, ''), COALESCE(c.reading_direction, '') FROM collections c WHERE c.id = $2 AND `+visible, userID, id).
		Scan(&c.ID, &c.Kind, &c.Name, &c.Retired, &c.IsFavorite, &c.System, &c.Description, &c.ReadingDirection)
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
		       (w.id IS NOT NULL AND w.retired_at IS NULL), COALESCE(w.unit, ''), COALESCE(w.comic_kind, ''),
		       w.volume_number, COALESCE(w.story_arc, '')
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
		if err := rows.Scan(&cw.EntryID, &cw.ID, &cw.Title, &cw.Author, &cw.CoverURL, &cw.Position, &cw.Completed, &cw.Available, &cw.Unit, &cw.ComicKind, &cw.VolumeNumber, &cw.StoryArc); err != nil {
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
	// Where to go on in a series (#187), for the page of an official collection that is not retired: a personal list is not a
	// series, and a retired collection is not read on.
	var goOn *SeriesStep
	if c.Kind == "official" && !c.Retired {
		entries, err := loadSeries(h.DB, c.ID, userID)
		if err != nil {
			log.Println("Error reading the series of a collection:", err)
			http.Error(w, "Error fetching the collection", http.StatusInternalServerError)
			return
		}
		goOn = continueSeries(entries)
	}
	summary, err := h.summarize(r.Context(), userID, order, works)
	if err != nil {
		log.Println("Error summarizing a collection:", err)
		http.Error(w, "Error fetching the collection", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"collection": c, "works": works, "continue": goOn, "summary": summary})
}
