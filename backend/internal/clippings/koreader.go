package clippings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type koEntry struct {
	Sort    string          `json:"sort"`
	Text    string          `json:"text"`
	Note    string          `json:"note"`
	Chapter string          `json:"chapter"`
	Color   string          `json:"color"`
	Time    json.RawMessage `json:"time"`
	Page    json.RawMessage `json:"page"`
	PnXp    json.RawMessage `json:"pn_xp"`
}

type koBook struct {
	Title   string    `json:"title"`
	Author  string    `json:"author"`
	Entries []koEntry `json:"entries"`
}

type koFile struct {
	koBook
	Documents []koBook `json:"documents"`
}

// asText reads a JSON value that may be a string or a number as text.
func asText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil && f == float64(int64(f)) {
		return fmt.Sprint(int64(f))
	}
	return ""
}

func koTime(raw json.RawMessage) *time.Time {
	var f float64
	if json.Unmarshal(raw, &f) == nil && f > 1e8 {
		t := time.Unix(int64(f), 0).UTC()
		return &t
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
				return &t
			}
		}
	}
	return nil
}

// ParseKOReader reads the export of highlights of KOReader in JSON (the Exporter plugin): one book, or several under "documents". The place of
// an entry of a reflowable book is its xpointer (`pn_xp`), which is what the importer turns into a place of the file.
func ParseKOReader(data []byte) (Result, error) {
	res := Result{Format: KOReader, Skipped: map[string]int{}}
	var f koFile
	if err := json.Unmarshal(data, &f); err != nil {
		return res, errors.New("this is not a KOReader export in JSON")
	}
	books := f.Documents
	if len(books) == 0 && (len(f.Entries) > 0 || f.Title != "") {
		books = []koBook{f.koBook}
	}
	if len(books) == 0 {
		return res, errors.New("this is not a KOReader export in JSON")
	}
	for _, b := range books {
		title, author := strings.TrimSpace(b.Title), strings.TrimSpace(b.Author)
		for _, e := range b.Entries {
			text := strings.TrimSpace(e.Text)
			note := strings.TrimSpace(e.Note)
			if e.Sort != "" && e.Sort != "highlight" && e.Sort != "note" {
				res.Skipped["bookmarks"]++
				continue
			}
			if text == "" && note == "" {
				res.Skipped["empty"]++
				continue
			}
			place := asText(e.PnXp)
			if place == "" {
				place = asText(e.Page)
			}
			entry := Entry{Origin: KOReader, Title: title, Author: author, Kind: "highlight", Text: text, Note: note, Color: strings.ToLower(e.Color),
				Chapter: strings.TrimSpace(e.Chapter), Place: place, At: koTime(e.Time)}
			if note != "" {
				entry.Kind = "note"
			}
			res.Entries = append(res.Entries, entry)
		}
	}
	return res, nil
}
