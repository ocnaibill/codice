package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/clippings"
	"github.com/ocnaibill/codice/backend/internal/locator"
	"github.com/ocnaibill/codice/backend/internal/people"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// NotesImportHandler brings in what a person marked on a reading device: the file My Clippings.txt of a Kindle, and the export of highlights of
// KOReader (JSON). Each passage becomes a highlight or a note of the person in the work they say it belongs to, with a place in the book when
// one can be found: KOReader says it (an xpointer, turned into the place of the paragraph), and a Kindle's passage is looked for in the text of the
// book. Importing the same file twice brings nothing twice.
type NotesImportHandler struct {
	DB          *sql.DB
	StorageRoot string

	kosync KosyncHandler // its reading of the place KOReader says (the spines of the books are kept in it)
}

const (
	maxImportBytes   = 5 << 20
	maxImportEntries = 20000
)

// readImport reads the file of the request and parses it, whichever device it came from.
func (h *NotesImportHandler) readImport(w http.ResponseWriter, r *http.Request) (clippings.Result, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+(64<<10))
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		http.Error(w, "O arquivo tem até 5 MB.", http.StatusRequestEntityTooLarge)
		return clippings.Result{}, nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Envie o arquivo no campo file.", http.StatusBadRequest)
		return clippings.Result{}, nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(data) > maxImportBytes {
		http.Error(w, "O arquivo tem até 5 MB.", http.StatusRequestEntityTooLarge)
		return clippings.Result{}, nil, false
	}
	if !utf8.Valid(data) {
		http.Error(w, "O arquivo não é um texto em UTF-8.", http.StatusBadRequest)
		return clippings.Result{}, nil, false
	}
	var res clippings.Result
	if strings.HasPrefix(strings.TrimLeft(string(data), " \t\r\n\ufeff"), "{") {
		if res, err = clippings.ParseKOReader(data); err != nil {
			http.Error(w, "Este JSON não é uma exportação de destaques do KOReader.", http.StatusBadRequest)
			return res, nil, false
		}
	} else {
		res = clippings.ParseKindle(string(data))
	}
	if len(res.Entries) == 0 {
		http.Error(w, "Não achei nenhum destaque ou nota neste arquivo.", http.StatusBadRequest)
		return res, nil, false
	}
	if len(res.Entries) > maxImportEntries {
		http.Error(w, "O arquivo tem anotações demais de uma vez.", http.StatusRequestEntityTooLarge)
		return res, nil, false
	}
	return res, data, true
}

// ImportGroup is a book as the device names it, with the works of the library that it may be.
type ImportGroup struct {
	Key        string            `json:"key"`
	Title      string            `json:"title"`
	Author     string            `json:"author"`
	Highlights int               `json:"highlights"`
	Notes      int               `json:"notes"`
	Candidates []ImportCandidate `json:"candidates"`
	// Suggested is the work chosen when the title and the author match, 0 when the person has to choose.
	Suggested int `json:"suggested"`
}

// ImportCandidate is a work of the library that a book of the file may be.
type ImportCandidate struct {
	WorkID int    `json:"workId"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Score  int    `json:"score"`
}

// importGroups lists the books of the file, in the order they come, with how many of each.
func importGroups(res clippings.Result) []*ImportGroup {
	var out []*ImportGroup
	index := map[string]*ImportGroup{}
	for _, e := range res.Entries {
		key := clippings.GroupKey(e.Title, e.Author)
		g := index[key]
		if g == nil {
			g = &ImportGroup{Key: key, Title: e.Title, Author: e.Author, Candidates: []ImportCandidate{}}
			index[key] = g
			out = append(out, g)
		}
		if e.Kind == "note" {
			g.Notes++
		} else {
			g.Highlights++
		}
	}
	return out
}

// candidates are the works of the library that a title (and an author) may be: those whose title has the words, best first.
func (h *NotesImportHandler) candidates(r *http.Request, userID string, g *ImportGroup) error {
	title := strings.TrimSpace(g.Title)
	if title == "" {
		return nil
	}
	if utf8.RuneCountInString(title) > 200 {
		title = string([]rune(title)[:200])
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT w.id, w.original_title, `+authorLabelFor(people.OrderFor(r.Context(), h.DB, userID).Effective)+`
		`+catalogFrom+`
		WHERE w.retired_at IS NULL AND `+titleMatches("$1")+`
		ORDER BY w.id DESC LIMIT 25`, catalogSearchPattern(title))
	if err != nil {
		return err
	}
	defer rows.Close()
	wantTitle, wantAuthor := clippings.Key(g.Title), clippings.Key(g.Author)
	for rows.Next() {
		var c ImportCandidate
		if err := rows.Scan(&c.WorkID, &c.Title, &c.Author); err != nil {
			return err
		}
		switch got := clippings.Key(c.Title); {
		case got == wantTitle:
			c.Score = 2
		case strings.HasPrefix(got, wantTitle) || strings.HasPrefix(wantTitle, got):
			c.Score = 1
		}
		// the author as the library writes it ("Herbert, Frank") and as the device does ("Frank Herbert") share their words
		if wantAuthor != "" && sameWords(clippings.Key(c.Author), wantAuthor) {
			c.Score += 2
		}
		g.Candidates = append(g.Candidates, c)
	}
	sort.SliceStable(g.Candidates, func(i, j int) bool { return g.Candidates[i].Score > g.Candidates[j].Score })
	if len(g.Candidates) > 5 {
		g.Candidates = g.Candidates[:5]
	}
	if len(g.Candidates) > 0 && g.Candidates[0].Score >= 2 && (len(g.Candidates) == 1 || g.Candidates[0].Score > g.Candidates[1].Score) {
		g.Suggested = g.Candidates[0].WorkID
	}
	return rows.Err()
}

// sameWords says whether two names have the same words, in any order.
func sameWords(a, b string) bool {
	x, y := strings.Fields(a), strings.Fields(b)
	if len(x) == 0 || len(x) != len(y) {
		return false
	}
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// Preview says what is in the file, the books of it and the works of the library each may be. Nothing is written.
func (h *NotesImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	res, _, ok := h.readImport(w, r)
	if !ok {
		return
	}
	userID := currentUserID(r)
	groups := importGroups(res)
	for _, g := range groups {
		if err := h.candidates(r, userID, g); err != nil {
			log.Println("Error looking for the works of an import:", err)
			http.Error(w, "Error reading the file", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"format": res.Format, "entries": len(res.Entries), "groups": groups, "skipped": res.Skipped})
}

var importColors = map[string]string{
	"red": "terracotta", "orange": "terracotta", "yellow": "sepia", "olive": "sepia", "green": "sage", "cyan": "sage",
	"blue": "indigo", "purple": "indigo", "gray": "indigo", "grey": "indigo",
}

// noteKey is what two writings of the same passage have in common, so that importing it again, or from the other device, brings nothing twice.
func noteKey(e clippings.Entry) string {
	sum := sha256.Sum256([]byte(clippings.Key(e.Text) + "|" + clippings.Key(e.Note)))
	return hex.EncodeToString(sum[:])
}

type importFile struct {
	ID       int64
	Format   string
	FullPath string
}

// fileFor is the file of a work that the passages are put in: the EPUB if there is one, then the PDF.
func (h *NotesImportHandler) fileFor(r *http.Request, workID int) (importFile, bool, error) {
	var f importFile
	var mode, root, rel sql.NullString
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT f.id, COALESCE(f.format, ''), l.mode, l.root, l.path
		FROM files f JOIN editions e ON e.id = f.edition_id
		LEFT JOIN LATERAL (SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1) l ON TRUE
		WHERE e.work_id = $1 AND f.availability = 'available'
		ORDER BY (lower(f.format) = 'epub') DESC, (lower(f.format) = 'pdf') DESC, e.is_primary DESC, f.id LIMIT 1`, workID).Scan(&f.ID, &f.Format, &mode, &root, &rel)
	if errors.Is(err, sql.ErrNoRows) {
		return f, false, nil
	}
	if err != nil {
		return f, false, err
	}
	f.Format = strings.ToLower(f.Format)
	if rel.Valid {
		if full, ok := storage.AbsPath(h.StorageRoot, mode.String, root.String, rel.String); ok {
			f.FullPath = full
		}
	}
	return f, true, nil
}

// locate finds where a passage is in the file: what KOReader says is turned into the place of the paragraph, and what a Kindle gives (only the
// text, and a count of its own) is looked for in the text of the book. Nil when it cannot be found.
func (h *NotesImportHandler) locate(r *http.Request, f importFile, e clippings.Entry) json.RawMessage {
	if e.Origin == clippings.KOReader && e.Place != "" {
		if raw, ok := h.kosync.locatorOf(kosyncFile{ID: f.ID, Format: f.Format, FullPath: f.FullPath}, e.Place); ok {
			if canonical, err := locator.Validate(f.Format, raw); err == nil {
				return canonical
			}
		}
	}
	text := e.Text
	if text == "" {
		return nil
	}
	words := strings.Fields(clippings.Key(text))
	for _, n := range []int{10, 5} {
		if len(words) < n && n != 5 {
			continue
		}
		if len(words) < 3 {
			return nil
		}
		take := n
		if len(words) < take {
			take = len(words)
		}
		var raw []byte
		err := h.DB.QueryRowContext(r.Context(), `
			SELECT s.locator FROM document_segments s
			JOIN text_extractions te ON te.file_id = s.file_id AND te.generation = s.generation
			WHERE s.file_id = $1 AND s.tsv @@ phraseto_tsquery('codice_simple', $2)
			ORDER BY s.sequence LIMIT 1`, f.ID, strings.Join(words[:take], " ")).Scan(&raw)
		if err == nil {
			if canonical, err := locator.Validate(f.Format, raw); err == nil {
				return canonical
			}
		}
	}
	return nil
}

// Import writes the passages of the books the person matched to a work. `mapping` says, by the key of each book of the file, the work it is (a
// book left out is not imported).
func (h *NotesImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	res, _, ok := h.readImport(w, r)
	if !ok {
		return
	}
	var mapping map[string]int
	if err := json.Unmarshal([]byte(r.FormValue("mapping")), &mapping); err != nil || len(mapping) == 0 {
		http.Error(w, "Diga a que obra cada livro do arquivo pertence.", http.StatusBadRequest)
		return
	}
	userID := currentUserID(r)

	works := map[int]importFile{}
	withFile := map[int]bool{}
	for _, workID := range mapping {
		if workID <= 0 {
			continue
		}
		if _, seen := works[workID]; seen {
			continue
		}
		var exists bool
		if err := h.DB.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM works WHERE id = $1 AND retired_at IS NULL)`, workID).Scan(&exists); err != nil || !exists {
			http.Error(w, "Uma das obras escolhidas não existe.", http.StatusBadRequest)
			return
		}
		f, found, err := h.fileFor(r, workID)
		if err != nil {
			log.Println("Error finding the file of an import:", err)
			http.Error(w, "Error importing", http.StatusInternalServerError)
			return
		}
		works[workID], withFile[workID] = f, found
	}

	var created, duplicates, located, unlocated, ignored int
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error importing", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	for _, e := range res.Entries {
		workID := mapping[clippings.GroupKey(e.Title, e.Author)]
		if workID <= 0 {
			ignored++
			continue
		}
		file := works[workID]
		var loc, ver, fileID any
		if withFile[workID] {
			fileID = file.ID
			if raw := h.locate(r, file, e); raw != nil {
				loc, ver = string(raw), locator.Version
				located++
			} else {
				unlocated++
			}
		} else {
			unlocated++
		}
		quote, err1 := cleanText(e.Text)
		body, err2 := cleanText(e.Note)
		if err := errors.Join(err1, err2); err != nil {
			ignored++
			continue
		}
		quote = clip(quote, maxQuote)
		if utf8.RuneCountInString(body) > maxBody {
			body = string([]rune(body)[:maxBody])
		}
		if quote == "" && body == "" {
			ignored++
			continue
		}
		kind := kindMark
		if body != "" {
			kind = kindNote
		}
		color := importColors[e.Color]
		if color == "" {
			color = defaultColor
		}
		at := time.Now()
		if e.At != nil {
			at = *e.At
		}
		var id int
		err := tx.QueryRowContext(r.Context(), `
			INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color, created_at, import_key)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8::jsonb, $9, $10, $11, $12)
			ON CONFLICT (user_id, work_id, import_key) WHERE import_key IS NOT NULL DO NOTHING
			RETURNING id`,
			userID, workID, fileID, kind, quote, body, pq.Array([]string{e.Origin}), loc, ver, color, at, noteKey(e)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			duplicates++
			continue
		}
		if err == nil {
			err = syncNoteLinks(r.Context(), tx, userID, int64(id), body)
		}
		if err != nil {
			log.Println("Error importing a note:", err)
			http.Error(w, "Error importing", http.StatusInternalServerError)
			return
		}
		created++
	}
	if err := tx.Commit(); err != nil {
		log.Println("Error importing notes:", err)
		http.Error(w, "Error importing", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created": created, "duplicates": duplicates, "located": located, "unlocated": unlocated, "ignored": ignored,
		"works": strconv.Itoa(len(works)),
	})
}
