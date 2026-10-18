package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
)

// maxShelfWorks is how many works one answer of the shelf may carry: the books on a reader are some dozens, and a bigger list is asked in parts.
const maxShelfWorks = 100

// shelfCategory is a category a work is in, or one above it (a category counts the works under it), so that a reader can group its own
// books by category with no other answer.
type shelfCategory struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parentId,omitempty"`
	Name     string `json:"name"`
}

type shelfAuthor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// shelfWork is what a reader keeps about a work it has on the device, so that it can browse its own books with no network and know how far
// the person is in each: everything that is shown in a card, a list or a sheet of the library, and nothing more.
type shelfWork struct {
	ID          int             `json:"id"`
	Title       string          `json:"title"`
	Author      string          `json:"author"`
	Authors     []shelfAuthor   `json:"authors"`
	Cover       string          `json:"cover"`
	Series      string          `json:"series,omitempty"`
	SeriesIndex float64         `json:"seriesIndex,omitempty"`
	Categories  []shelfCategory `json:"categories"`
	Favorite    bool            `json:"favorite"`
	Percent     float64         `json:"percent"`
	InProgress  bool            `json:"inProgress"`
	Completed   bool            `json:"completed"`
	Finished    bool            `json:"finished"`
	Files       []deviceFile    `json:"files"`
}

// workAs asks the handler of a work of the web for work `id`, as the person: nil when it does not answer (it does not exist, or the person
// may not see it).
func (h *DeviceHandler) workAs(r *http.Request, id int) *Work {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.Itoa(id))
	req := r.Clone(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	var work Work
	if !pull(h.Sources.Work, req, nil, &work) || work.ID == 0 {
		return nil
	}
	return &work
}

// ShelfState answers, for the works a reader asks for (`?works=1,2,3`), what it keeps about them: who wrote them, the series, the categories,
// the files, and the state of the person in each (favorite, how far, finished). One answer, for the reader to refresh its whole shelf when it
// gets the network. A work that does not exist, or that the person may not see, comes in `missing` (the reader forgets it).
func (h *DeviceHandler) ShelfState(w http.ResponseWriter, r *http.Request) {
	var ids []int
	seen := map[int]bool{}
	for _, part := range strings.Split(r.URL.Query().Get("works"), ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id < 1 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	// a reader that has files on the device and does not know the works yet (they were downloaded before it kept a shelf) asks by file:
	// the works those files belong to are answered, each with its files, and the reader sees which is which
	var fileIDs []int64
	for _, part := range strings.Split(r.URL.Query().Get("files"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			fileIDs = append(fileIDs, id)
		}
	}
	if len(fileIDs) > maxShelfWorks {
		http.Error(w, "Too many files at once", http.StatusBadRequest)
		return
	}
	if len(fileIDs) > 0 {
		rows, err := h.DB.QueryContext(r.Context(), `SELECT DISTINCT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = ANY($1) ORDER BY e.work_id`, pq.Array(fileIDs))
		if err == nil {
			for rows.Next() {
				var id int
				if rows.Scan(&id) == nil && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			rows.Close()
		}
	}
	if len(ids) == 0 {
		http.Error(w, "Ask for works or files: ?works=1,2,3 or ?files=4,5", http.StatusBadRequest)
		return
	}
	if len(ids) > maxShelfWorks {
		http.Error(w, "Too many works at once", http.StatusBadRequest)
		return
	}

	works := []shelfWork{}
	missing := []int{}
	var found []int
	for _, id := range ids {
		work := h.workAs(r, id)
		if work == nil {
			missing = append(missing, id)
			continue
		}
		var last int64
		if work.Continue != nil {
			last = work.Continue.FileID
		}
		sw := shelfWork{ID: work.ID, Title: work.Title, Author: work.Author, Cover: work.CoverURL, Series: work.Series, SeriesIndex: work.SeriesIndex,
			Favorite: work.IsFavorite, Percent: work.PercentComplete, InProgress: work.InProgress, Completed: work.Completed, Finished: work.Finished,
			Authors: []shelfAuthor{}, Categories: []shelfCategory{}, Files: []deviceFile{}}
		for _, a := range work.Authors {
			sw.Authors = append(sw.Authors, shelfAuthor{ID: a.ID, Name: a.Name})
		}
		for _, e := range work.Editions {
			for i, f := range e.Files {
				if f.Availability != "available" {
					continue
				}
				df := deviceFile{ID: f.ID, Format: f.Format, Language: e.Language, Primary: e.IsPrimary && i == 0, Last: f.ID == last,
					Percent: f.PercentComplete, Completed: f.Completed, Started: f.Started}
				if f.SizeBytes != nil {
					df.SizeBytes = *f.SizeBytes
				}
				sw.Files = append(sw.Files, df)
			}
		}
		works = append(works, sw)
		found = append(found, work.ID)
	}

	// the categories of all the works in one query: each one a work is in, and the ones above it
	if len(found) > 0 {
		rows, err := h.DB.QueryContext(r.Context(), `
			WITH RECURSIVE up AS (
				SELECT wc.work_id, c.id, c.parent_id, c.name FROM work_categories wc JOIN categories c ON c.id = wc.category_id
				WHERE wc.work_id = ANY($1)
				UNION
				SELECT up.work_id, p.id, p.parent_id, p.name FROM up JOIN categories p ON p.id = up.parent_id
			)
			SELECT work_id, id, parent_id, name FROM up ORDER BY work_id, id`, pq.Array(found))
		if err == nil {
			defer rows.Close()
			byWork := map[int][]shelfCategory{}
			for rows.Next() {
				var workID int
				var c shelfCategory
				if rows.Scan(&workID, &c.ID, &c.ParentID, &c.Name) == nil {
					byWork[workID] = append(byWork[workID], c)
				}
			}
			for i := range works {
				if cs := byWork[works[i].ID]; cs != nil {
					works[i].Categories = cs
				}
			}
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"works": works, "missing": missing})
}
