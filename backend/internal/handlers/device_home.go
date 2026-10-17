package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// HomeSources are the handlers of the web whose answers the home of a reading device is made of. The home asks them as the person (the
// same rules about what is shown), and keeps from each answer only what a small screen draws.
type HomeSources struct {
	Works, Stats, Notes, Categories http.Handler
}

const (
	homeShelves       = 6  // categories shown as shelves, the biggest first
	homeShelfWorks    = 12 // works in a shelf
	homeContinueWorks = 8
	homeQuoteRunes    = 280
)

// DeviceCard is a work as the screen of a reader draws it: a cover, a title, how far the person is.
type DeviceCard struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Cover       string  `json:"cover"` // a path of this server, asked with the access of the device and ?w= for the size
	Percent     float64 `json:"percent"`
	Format      string  `json:"format,omitempty"`
	FileID      int64   `json:"fileId,omitempty"` // the file to read next, or the first; 0 when there is none to open
	InProgress  bool    `json:"inProgress"`
	Completed   bool    `json:"completed"`
	Series      string  `json:"series,omitempty"`
	SeriesIndex float64 `json:"seriesIndex,omitempty"`
}

type deviceShelf struct {
	ID    int64        `json:"id"`
	Title string       `json:"title"`
	Total int          `json:"total"`
	Works []DeviceCard `json:"works"`
}

type deviceQuote struct {
	Text   string `json:"text"`
	Title  string `json:"title"`
	Author string `json:"author"`
	WorkID int    `json:"workId,omitempty"`
}

// pull asks a handler of the web, as the person, and decodes its answer. False when it did not answer well: that part of the home is
// then left out, and the rest is still shown.
func pull(src http.Handler, r *http.Request, query url.Values, into any) bool {
	if src == nil {
		return false
	}
	req := r.Clone(r.Context())
	req.Method = http.MethodGet
	req.Body = http.NoBody
	req.URL.RawQuery = query.Encode()
	rec := httptest.NewRecorder()
	src.ServeHTTP(rec, req)
	return rec.Code == http.StatusOK && json.Unmarshal(rec.Body.Bytes(), into) == nil
}

func cardOf(w Work) DeviceCard {
	c := DeviceCard{ID: w.ID, Title: w.Title, Author: w.Author, Cover: w.CoverURL, Percent: w.PercentComplete, Format: w.Format,
		InProgress: w.InProgress, Completed: w.Completed, Series: w.Series, SeriesIndex: w.SeriesIndex}
	if w.FileID != nil {
		c.FileID = *w.FileID
	}
	// the book to continue is the version the person reads, with their own place and format
	if w.Continue != nil {
		c.FileID, c.Format = w.Continue.FileID, w.Continue.Format
	}
	return c
}

func cardsOf(works []Work) []DeviceCard {
	out := make([]DeviceCard, 0, len(works))
	for _, w := range works {
		out = append(out, cardOf(w))
	}
	return out
}

func shorten(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit])) + "…"
}

// Home is the first screen of a reading device, in the order of the home of the web: greeting and numbers, what to continue, the
// favorites, a few things the person marked, and the library as shelves by category. One answer, because the network of an e-reader is
// slow and each request is a wait.
func (h *DeviceHandler) Home(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"version": 1}
	userID := currentUserID(r)
	var name string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(NULLIF(display_name, ''), username) FROM users WHERE id = $1`, userID).Scan(&name); err == nil {
		out["user"] = map[string]string{"name": name}
	}

	var stats struct {
		WorksTotal          int `json:"worksTotal"`
		InProgressCount     int `json:"inProgressCount"`
		CompletedThisMonth  int `json:"completedThisMonth"`
		TotalReadingSeconds int `json:"totalReadingSeconds"`
	}
	if pull(h.Sources.Stats, r, nil, &stats) {
		out["stats"] = stats
	}

	var works struct {
		Data []Work `json:"data"`
	}
	if pull(h.Sources.Works, r, url.Values{"inProgress": {"true"}, "limit": {strconv.Itoa(homeContinueWorks)}}, &works) {
		out["continue"] = cardsOf(works.Data)
	}

	// the favorites are the works the person marked (not the series of the web rail), each with the file to open
	var favorites struct {
		Data []Work `json:"data"`
	}
	if pull(h.Sources.Works, r, url.Values{"favorite": {"true"}, "limit": {strconv.Itoa(homeContinueWorks)}}, &favorites) {
		out["favorites"] = cardsOf(favorites.Data)
	}

	var notes struct {
		Data []struct {
			Quote      string `json:"quote"`
			Body       string `json:"body"`
			WorkID     *int   `json:"workId"`
			WorkTitle  string `json:"workTitle"`
			WorkAuthor string `json:"workAuthor"`
		} `json:"data"`
	}
	if pull(h.Sources.Notes, r, url.Values{"limit": {"2"}, "sample": {"true"}}, &notes) {
		quotes := make([]deviceQuote, 0, len(notes.Data))
		for _, n := range notes.Data {
			text := n.Quote
			if text == "" {
				text = n.Body
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			q := deviceQuote{Text: shorten(text, homeQuoteRunes), Title: n.WorkTitle, Author: n.WorkAuthor}
			if n.WorkID != nil {
				q.WorkID = *n.WorkID
			}
			quotes = append(quotes, q)
		}
		out["notes"] = quotes
	}

	var categories struct {
		Data []struct {
			ID       int64  `json:"id"`
			ParentID *int64 `json:"parentId"`
			Name     string `json:"name"`
			Works    int    `json:"works"`
		} `json:"data"`
	}
	shelves := []deviceShelf{}
	if pull(h.Sources.Categories, r, nil, &categories) {
		// the biggest top-level categories (a category counts the works under it), each with its newest works
		type top struct {
			id    int64
			name  string
			works int
		}
		var tops []top
		for _, c := range categories.Data {
			if c.ParentID == nil && c.Works > 0 {
				tops = append(tops, top{c.ID, c.Name, c.Works})
			}
		}
		for i := 1; i < len(tops); i++ { // insertion sort, by size and then by name: few of them
			for j := i; j > 0 && (tops[j].works > tops[j-1].works || (tops[j].works == tops[j-1].works && tops[j].name < tops[j-1].name)); j-- {
				tops[j], tops[j-1] = tops[j-1], tops[j]
			}
		}
		if len(tops) > homeShelves {
			tops = tops[:homeShelves]
		}
		for _, c := range tops {
			var in struct {
				Data  []Work `json:"data"`
				Total int    `json:"total"`
			}
			q := url.Values{"category": {strconv.FormatInt(c.id, 10)}, "limit": {strconv.Itoa(homeShelfWorks)}, "series": {"collapse"}}
			if pull(h.Sources.Works, r, q, &in) && len(in.Data) > 0 {
				shelves = append(shelves, deviceShelf{ID: c.id, Title: c.name, Total: in.Total, Works: cardsOf(in.Data)})
			}
		}
	}
	out["shelves"] = shelves

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
