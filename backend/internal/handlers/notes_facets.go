package handlers

import (
	"log"
	"net/http"
	"strconv"
)

// maxFacetTags bounds the list of tags: a person's tags are few, and the screen shows the commonest.
const maxFacetTags = 100

// TagCount is a tag of the caller's notes and how many notes carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// NoteFacets is what the screen of notes offers to narrow the list by, with how many notes each choice would leave
// (UI-05 of the specification): the notes of each kind, and the tags in use. The counts follow the other filters
// (text, work, file) and, for each facet, the choice of the other: the kinds count under the chosen tag, and the tags
// under the chosen kind, but neither counts under its own choice, so that the person sees what the other choices would
// give, as in any filter.
type NoteFacets struct {
	Kinds map[string]int `json:"kinds"`
	Tags  []TagCount     `json:"tags"`
}

// NoteFacets counts the caller's notes by kind and by tag, under the same filters as the list. Only the caller's own
// notes count (the condition always starts with them).
func (h *NotesHandler) NoteFacets(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	f, err := parseNoteFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out := NoteFacets{Kinds: map[string]int{kindNote: 0, kindMark: 0, kindBookmk: 0}, Tags: []TagCount{}}

	byKind := f
	byKind.kind = ""
	cond, args := byKind.where(userID)
	rows, err := h.DB.QueryContext(r.Context(), `SELECT n.kind, count(*) FROM notes n WHERE `+cond+` GROUP BY n.kind`, args...)
	if err != nil {
		log.Println("Error counting notes by kind:", err)
		http.Error(w, "Error counting notes", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			rows.Close()
			log.Println("Error reading the count of a kind:", err)
			http.Error(w, "Error counting notes", http.StatusInternalServerError)
			return
		}
		out.Kinds[kind] = n
	}
	rows.Close()

	// A tag is one whatever its case (the filter by tag ignores it too); it is shown as the first spelling by order.
	byTag := f
	byTag.tag = ""
	cond, args = byTag.where(userID)
	rows, err = h.DB.QueryContext(r.Context(), `
		SELECT min(t), count(*) FROM notes n, unnest(n.tags) t WHERE `+cond+`
		GROUP BY lower(t) ORDER BY count(*) DESC, lower(t) LIMIT `+strconv.Itoa(maxFacetTags), args...)
	if err != nil {
		log.Println("Error counting notes by tag:", err)
		http.Error(w, "Error counting notes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			log.Println("Error reading the count of a tag:", err)
			http.Error(w, "Error counting notes", http.StatusInternalServerError)
			return
		}
		out.Tags = append(out.Tags, t)
	}
	writeJSON(w, http.StatusOK, out)
}
