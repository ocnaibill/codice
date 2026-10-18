package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// What a work can hand to the others of its series (DEC-203): the descriptive data that is the same for a whole series, so that it is written
// once. What belongs to each file (title, number, ISBN, publication date, cover, the file) is never shared; volume and arc have "Agrupar".
var shareScalars = map[string]func(f *workFields) *string{
	"publisher":     func(f *workFields) *string { return &f.Publisher },
	"language":      func(f *workFields) *string { return &f.Language },
	"comic_kind":    func(f *workFields) *string { return &f.ComicKind },
	"original_year": func(f *workFields) *string { return &f.OriginalYear },
	"description":   func(f *workFields) *string { return &f.Description },
}

// shareRoles are the credits that can be shared, by the field that names them.
var shareRoles = map[string]string{"authors": "author", "translators": "translator", "illustrators": "illustrator"}

func validShareField(name string) bool {
	if _, ok := shareScalars[name]; ok {
		return true
	}
	if _, ok := shareRoles[name]; ok {
		return true
	}
	return name == "categories" || name == "tags"
}

// ShareFieldNames are the names the page may send, in the order it lists them.
var ShareFieldNames = []string{"publisher", "language", "comic_kind", "original_year", "description", "authors", "translators", "illustrators", "categories", "tags"}

// shareValue is what a work has of a shared field, as something comparable: a text for a scalar, the ordered ids for a credit and the
// sorted ids for categories and tags.
func shareValue(ctx context.Context, tx *sql.Tx, workID int, field string, fields workFields) (string, error) {
	if get, ok := shareScalars[field]; ok {
		return *get(&fields), nil
	}
	var rows *sql.Rows
	var err error
	switch {
	case shareRoles[field] != "":
		rows, err = tx.QueryContext(ctx, `SELECT person_id FROM work_contributors WHERE work_id = $1 AND role = $2 ORDER BY position, person_id`, workID, shareRoles[field])
	case field == "categories":
		rows, err = tx.QueryContext(ctx, `SELECT category_id FROM work_categories WHERE work_id = $1 ORDER BY category_id`, workID)
	default:
		rows, err = tx.QueryContext(ctx, `SELECT tag_id FROM work_tags WHERE work_id = $1 ORDER BY tag_id`, workID)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		out += strconv.FormatInt(id, 10) + ","
	}
	return out, rows.Err()
}

// copyShared writes the list of the source (credits, categories, tags) on a target, in place of what it had.
func copyShared(ctx context.Context, tx *sql.Tx, source, target int, field, actor string) error {
	switch {
	case shareRoles[field] != "":
		role := shareRoles[field]
		if _, err := tx.ExecContext(ctx, `DELETE FROM work_contributors WHERE work_id = $1 AND role = $2`, target, role); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO work_contributors (work_id, person_id, role, position)
			SELECT $1, person_id, role, position FROM work_contributors WHERE work_id = $2 AND role = $3`, target, source, role); err != nil {
			return err
		}
		if role == "author" {
			// the bibliographic reference kept on the notes follows the author (DEC-040)
			_, err := tx.ExecContext(ctx, `
				UPDATE notes SET source_author = (SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
				                                  WHERE c.work_id = $1 AND c.role = 'author' ORDER BY c.position, p.name LIMIT 1)
				WHERE work_id = $1`, target)
			return err
		}
		return nil
	case field == "categories":
		if _, err := tx.ExecContext(ctx, `DELETE FROM work_categories WHERE work_id = $1`, target); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work_categories (work_id, category_id, assigned_by)
			SELECT $1, category_id, NULLIF($3, '')::uuid FROM work_categories WHERE work_id = $2`, target, source, actor)
		return err
	default:
		if _, err := tx.ExecContext(ctx, `DELETE FROM work_tags WHERE work_id = $1`, target); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO work_tags (work_id, tag_id) SELECT $1, tag_id FROM work_tags WHERE work_id = $2`, target, source)
		return err
	}
}

// Share answers POST /collections/{id}/share (staff, DEC-203): takes the chosen fields of a work of the series and gives them to the others,
// the ones of the units (and the range of numbers) asked for. With mode "fill" only the works that have none of it get it; with "replace" every
// work gets it. With preview, nothing is written: the answer says what would change. What is written is confirmed by hand, like an edit of the
// work, and the action is in the audit.
func (h *CollectionsAdminHandler) Share(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	var req struct {
		SourceWorkID int      `json:"sourceWorkId"`
		Fields       []string `json:"fields"`
		Units        []string `json:"units"`
		From         *float64 `json:"from"`
		To           *float64 `json:"to"`
		Mode         string   `json:"mode"`
		Preview      bool     `json:"preview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.SourceWorkID <= 0 || len(req.Fields) == 0 || (req.Mode != "fill" && req.Mode != "replace") {
		http.Error(w, "Diga a obra de origem, os campos e se é para preencher ou substituir.", http.StatusBadRequest)
		return
	}
	for _, f := range req.Fields {
		if !validShareField(f) {
			http.Error(w, "Esse campo não é compartilhado entre as obras da série.", http.StatusBadRequest)
			return
		}
	}
	for _, u := range req.Units {
		if !validUnit(u) {
			http.Error(w, "A unidade é volume, capítulo, único ou complementar.", http.StatusBadRequest)
			return
		}
	}
	if req.From != nil && req.To != nil && *req.From > *req.To {
		http.Error(w, "O começo da faixa não pode passar do fim.", http.StatusBadRequest)
		return
	}
	actor := currentUserID(r)
	tx, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer tx.Rollback()
	name, retired, err := lockOfficial(r.Context(), tx, id)
	if err != nil {
		h.fail(w, "share", err)
		return
	}
	if retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	var inSeries bool
	if err := tx.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM collection_works cw JOIN works w ON w.id = cw.work_id
		               WHERE cw.collection_id = $1 AND cw.work_id = $2 AND w.retired_at IS NULL)`, id, req.SourceWorkID).Scan(&inSeries); err != nil {
		h.fail(w, "share", err)
		return
	}
	if !inSeries {
		http.Error(w, "A obra de origem não está nesta série.", http.StatusBadRequest)
		return
	}
	source, _, err := readWorkFields(tx, req.SourceWorkID)
	if err != nil {
		h.fail(w, "share", err)
		return
	}
	var units any
	if len(req.Units) > 0 {
		units = pq.Array(req.Units)
	}
	rows, err := tx.QueryContext(r.Context(), `
		SELECT w.id FROM collection_works cw JOIN works w ON w.id = cw.work_id
		WHERE cw.collection_id = $1 AND w.retired_at IS NULL AND w.id <> $5
		  AND ($2::text[] IS NULL OR COALESCE(w.unit, '') = ANY($2))
		  AND ($3::real IS NULL OR cw.position >= $3) AND ($4::real IS NULL OR cw.position <= $4)
		  AND (($3::real IS NULL AND $4::real IS NULL) OR cw.position IS NOT NULL)
		ORDER BY w.id FOR UPDATE OF w`, id, units, req.From, req.To, req.SourceWorkID)
	if err != nil {
		h.fail(w, "share", err)
		return
	}
	var targets []int
	for rows.Next() {
		var wid int
		if err := rows.Scan(&wid); err != nil {
			rows.Close()
			h.fail(w, "share", err)
			return
		}
		targets = append(targets, wid)
	}
	rows.Close()

	// What the source has of each field; a field it has none of is not shared (it would only take the value away from the others).
	sourceValue := map[string]string{}
	var empty []string
	var fields []string
	for _, f := range req.Fields {
		v, err := shareValue(r.Context(), tx, req.SourceWorkID, f, source)
		if err != nil {
			h.fail(w, "share", err)
			return
		}
		if v == "" {
			empty = append(empty, f)
			continue
		}
		sourceValue[f] = v
		fields = append(fields, f)
	}
	change, keep := map[string]int{}, map[string]int{}
	worksChanged := 0
	for _, t := range targets {
		cur, retiredWork, err := readWorkFields(tx, t)
		if err != nil {
			h.fail(w, "share", err)
			return
		}
		next := cur
		var lists []string
		for _, f := range fields {
			have, err := shareValue(r.Context(), tx, t, f, cur)
			if err != nil {
				h.fail(w, "share", err)
				return
			}
			switch {
			case have == sourceValue[f]:
			case have != "" && req.Mode == "fill":
				keep[f]++
			default:
				change[f]++
				if get, ok := shareScalars[f]; ok {
					*get(&next) = sourceValue[f] // the text of a scalar is its value
				} else {
					lists = append(lists, f)
				}
			}
		}
		touched := len(lists) > 0 || next != cur
		if touched {
			worksChanged++
		}
		if req.Preview || !touched {
			continue
		}
		if next != cur {
			if _, err := applyWorkFields(r.Context(), tx, t, actor, sourceManual, cur, retiredWork, next, nil); err != nil {
				h.fail(w, "share", err)
				return
			}
		}
		for _, f := range lists {
			if err := copyShared(r.Context(), tx, req.SourceWorkID, t, f, actor); err != nil {
				h.fail(w, "share", err)
				return
			}
		}
	}
	if !req.Preview {
		sort.Strings(fields)
		details := map[string]any{"name": name, "source": req.SourceWorkID, "mode": req.Mode, "fields": fields, "works": len(targets), "changed": change}
		if err := audit.Record(r.Context(), tx, actor, "collection.share", "collection", strconv.FormatInt(id, 10), details); err != nil {
			h.fail(w, "share", err)
			return
		}
		if err := tx.Commit(); err != nil {
			h.fail(w, "share", err)
			return
		}
	}
	if empty == nil {
		empty = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"preview": req.Preview, "works": len(targets), "worksChanged": worksChanged, "changed": change, "kept": keep, "sourceHasNone": empty,
	})
}
