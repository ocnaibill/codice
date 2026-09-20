package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// place says, in words, where a locator points, and in what kind of file. It is for a person
// reading an export; the exact address goes along in the note's own comment and in the JSON. It
// reads the kind from the locator, not from the file: the file may be gone and the note is not.
func place(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var l struct {
		Type   string  `json:"type"`
		Href   string  `json:"href"`
		CFI    string  `json:"cfi"`
		Page   int     `json:"page"`
		Label  string  `json:"label"`
		Index  int     `json:"index"`
		Track  int     `json:"track"`
		Ms     int64   `json:"ms"`
		Offset int     `json:"offset"`
		Prog   float64 `json:"progression"`
	}
	if json.Unmarshal(raw, &l) != nil {
		return ""
	}
	switch l.Type {
	case "epub":
		if l.Href != "" {
			return "EPUB, capítulo " + plain(l.Href)
		}
		return "EPUB, posição " + plain(l.CFI)
	case "pdf":
		if l.Label != "" {
			return fmt.Sprintf("PDF, página %s (%d do arquivo)", plain(l.Label), l.Page+1)
		}
		return fmt.Sprintf("PDF, página %d", l.Page+1)
	case "image":
		return fmt.Sprintf("imagens, número %d", l.Index+1)
	case "audio":
		s := l.Ms / 1000
		return fmt.Sprintf("áudio, faixa %d, %d:%02d", l.Track+1, s/60, s%60)
	case "text":
		return fmt.Sprintf("texto, caractere %d", l.Offset)
	}
	return ""
}

// plain keeps text that came from a client (a page label, a chapter name) from opening markup
// in the reader of an export: no angle brackets, on one line.
func plain(s string) string {
	return oneLine(strings.NewReplacer("<", "", ">", "").Replace(s))
}

var kindLabel = map[string]string{kindNote: "Nota", kindMark: "Destaque", kindBookmk: "Marcador"}

// markdownExport writes notes as a Markdown document: one section per work, in the work's
// title order, each note with its date, its place, the quotation, the person's own text and
// tags. The reference (title and author) comes from the note, so a work that is gone still
// has its heading. A comment per note keeps the ids and the exact locator for tools.
func markdownExport(notes []Note, now time.Time) string {
	byWork := map[string][]Note{}
	var order []string
	key := func(n Note) string { return n.WorkTitle + "\x00" + n.WorkAuthor }
	for _, n := range notes {
		k := key(n)
		if _, ok := byWork[k]; !ok {
			order = append(order, k)
		}
		byWork[k] = append(byWork[k], n)
	}
	sort.SliceStable(order, func(i, j int) bool { return strings.ToLower(order[i]) < strings.ToLower(order[j]) })

	var b strings.Builder
	fmt.Fprintf(&b, "# Anotações do Códice\n\nExportadas em %s · %d anotações\n", now.Format("2006-01-02"), len(notes))
	for _, k := range order {
		group := byWork[k]
		first := group[0]
		fmt.Fprintf(&b, "\n## %s\n", oneLine(first.WorkTitle))
		if first.WorkAuthor != "" && first.WorkAuthor != "Unknown Author" {
			fmt.Fprintf(&b, "*%s*\n", oneLine(first.WorkAuthor))
		}
		if !first.SourceAvailable {
			b.WriteString("\n*A obra não está mais no acervo; a referência acima foi preservada com as anotações.*\n")
		}
		sort.SliceStable(group, func(i, j int) bool { return group[i].CreatedAt.Before(group[j].CreatedAt) })
		for _, n := range group {
			head := kindLabel[n.Kind] + " · " + n.CreatedAt.Format("2006-01-02")
			if p := place(n.Locator); p != "" {
				head += " · " + p
			}
			fmt.Fprintf(&b, "\n### %s\n", head)
			if n.Quote != "" {
				b.WriteString("\n")
				for _, line := range strings.Split(n.Quote, "\n") {
					b.WriteString("> " + line + "\n")
				}
			}
			if n.Body != "" {
				fmt.Fprintf(&b, "\n%s\n", n.Body)
			}
			if len(n.Tags) > 0 {
				tags := make([]string, len(n.Tags))
				for i, t := range n.Tags {
					tags[i] = "#" + strings.ReplaceAll(t, " ", "-")
				}
				fmt.Fprintf(&b, "\nTags: %s\n", strings.Join(tags, " "))
			}
			meta := fmt.Sprintf("codice:note id=%d", n.ID)
			if n.FileID != nil {
				meta += fmt.Sprintf(" file=%d", *n.FileID)
			}
			if n.WorkID != nil {
				meta += fmt.Sprintf(" work=%d", *n.WorkID)
			}
			if len(n.Locator) > 0 {
				// "--" cannot appear inside an HTML comment; JSON of a locator has none, but be sure.
				var compact bytes.Buffer
				if json.Compact(&compact, n.Locator) == nil {
					meta += " locator=" + strings.ReplaceAll(compact.String(), "--", "-\\u002d")
				}
			}
			fmt.Fprintf(&b, "\n<!-- %s -->\n", meta)
		}
	}
	return b.String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// ExportNotes downloads the caller's marginalia as Markdown (format=md, the default) or JSON
// (format=json), for all of it or filtered like the list: work, file, kind, tag, search. Only
// their own notes can ever be in it (FL-09).
func (h *NotesHandler) ExportNotes(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "json" {
		http.Error(w, "format is md or json", http.StatusBadRequest)
		return
	}
	f, err := parseNoteFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cond, args := f.where(currentUserID(r))
	rows, err := h.DB.QueryContext(r.Context(), noteSelect+` WHERE `+cond+` ORDER BY n.created_at, n.id LIMIT `+strconv.Itoa(exportCap), args...)
	if err != nil {
		log.Println("Error exporting notes:", err)
		http.Error(w, "Error exporting notes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			log.Println("Error scanning note for export:", err)
			http.Error(w, "Error exporting notes", http.StatusInternalServerError)
			return
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		log.Println("Error exporting notes:", err)
		http.Error(w, "Error exporting notes", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	name := "codice-anotacoes-" + now.Format("20060102")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.json"`)
		json.NewEncoder(w).Encode(map[string]any{"exportedAt": now.UTC(), "count": len(notes), "notes": notes})
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.md"`)
	w.Write([]byte(markdownExport(notes, now)))
}
