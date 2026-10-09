package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// CategoryRulesHandler keeps the rules that put works in categories, and applies them (DEC-140). A rule is a term: a work that has a tag
// that is that term, without regard to case or accents, is put in the category (each part of a tag like "Fiction / Science Fiction /
// General" counts on its own). Rules only add: they are applied after a preview, they never take a work out of a category, and they do not
// put a work back in one that a person took it out of, whoever had put it there.
type CategoryRulesHandler struct{ DB *sql.DB }

const (
	maxRuleTerm         = 100
	maxRulesPerCategory = 300
)

// CategoryRule is a term of a category.
type CategoryRule struct {
	ID         int64  `json:"id"`
	CategoryID int64  `json:"categoryId"`
	Term       string `json:"term"`
}

// termKeySQL is how a text is compared with a term: lower case, no accents, single spaces. The database does it for both the rule and
// the tag, so that the two are always made the same way.
func termKeySQL(expr string) string {
	return "lower(unaccent(btrim(regexp_replace(" + expr + ", '\\s+', ' ', 'g'))))"
}

// rulesCTE is what the rules do to the library now, as three sets of (work, category): hits (what the rules say), fresh (the hits that are
// not there yet and that nobody took out). Retired works are left out. The tags of a work are compared whole and by the parts of
// "A / B / C".
var rulesCTE = `
	tag_keys AS (
		SELECT wt.work_id, keys.k
		FROM work_tags wt JOIN tags t ON t.id = wt.tag_id
		CROSS JOIN LATERAL (
			SELECT ` + termKeySQL("s") + ` AS k FROM regexp_split_to_table(t.name, '/') s
			UNION
			SELECT ` + termKeySQL("t.name") + `
		) keys
	),
	hits AS (
		SELECT DISTINCT tk.work_id, r.category_id
		FROM tag_keys tk JOIN category_rules r ON r.term_key = tk.k
		JOIN works w ON w.id = tk.work_id AND w.retired_at IS NULL
	),
	fresh AS (
		SELECT h.work_id, h.category_id FROM hits h
		WHERE NOT EXISTS (SELECT 1 FROM work_categories wc WHERE wc.work_id = h.work_id AND wc.category_id = h.category_id)
		  AND NOT EXISTS (SELECT 1 FROM work_category_exclusions e WHERE e.work_id = h.work_id AND e.category_id = h.category_id)
	)`

func tidyTerm(raw string) (string, bool) {
	term := strings.Join(strings.Fields(raw), " ")
	if term == "" || utf8.RuneCountInString(term) > maxRuleTerm {
		return "", false
	}
	for _, r := range term {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return term, true
}

// List answers GET /admin/categories/rules: every rule, by category and term.
func (h *CategoryRulesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT r.id, r.category_id, r.term FROM category_rules r JOIN categories c ON c.id = r.category_id
		ORDER BY unaccent(lower(c.name)), c.id, r.term_key`)
	if err != nil {
		log.Println("Error listing the category rules:", err)
		http.Error(w, "Error listing the rules", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []CategoryRule{}
	for rows.Next() {
		var rule CategoryRule
		if err := rows.Scan(&rule.ID, &rule.CategoryID, &rule.Term); err != nil {
			http.Error(w, "Error listing the rules", http.StatusInternalServerError)
			return
		}
		out = append(out, rule)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// Add answers POST /admin/categories/{id}/rules: a term for the category.
func (h *CategoryRulesHandler) Add(w http.ResponseWriter, r *http.Request) {
	id, ok := categoryID(r)
	var req struct {
		Term string `json:"term"`
	}
	if !ok {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil {
		http.Error(w, "term is required", http.StatusBadRequest)
		return
	}
	term, valid := tidyTerm(req.Term)
	if !valid {
		http.Error(w, "A term needs up to 100 characters", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM category_rules WHERE category_id = $1`, id).Scan(&n); err != nil {
		http.Error(w, "Error adding the term", http.StatusInternalServerError)
		return
	}
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM categories WHERE id = $1)`, id).Scan(&exists); err != nil || !exists {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if n >= maxRulesPerCategory {
		http.Error(w, "A category has at most 300 terms", http.StatusBadRequest)
		return
	}
	var ruleID int64
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO category_rules (category_id, term, term_key, created_by) VALUES ($1, $2::text, `+termKeySQL("$2::text")+`, NULLIF($3, '')::uuid) RETURNING id`,
		id, term, currentUserID(r)).Scan(&ruleID)
	if isDuplicateName(err) {
		http.Error(w, "That term is already a rule of this category", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error adding a category rule:", err)
		http.Error(w, "Error adding the term", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "category.rule_add", "category", strconv.FormatInt(id, 10), map[string]any{"term": term}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error adding the term", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, CategoryRule{ID: ruleID, CategoryID: id, Term: term})
}

// Remove answers DELETE /admin/categories/rules/{ruleId}. The works the rule already put in the category stay in it.
func (h *CategoryRulesHandler) Remove(w http.ResponseWriter, r *http.Request) {
	ruleID, err := strconv.ParseInt(chi.URLParam(r, "ruleId"), 10, 64)
	if err != nil {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}
	var categoryID int64
	var term string
	err = h.DB.QueryRowContext(r.Context(), `DELETE FROM category_rules WHERE id = $1 RETURNING category_id, term`, ruleID).Scan(&categoryID, &term)
	if err != nil {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}
	if err := audit.Record(r.Context(), h.DB, currentUserID(r), "category.rule_remove", "category", strconv.FormatInt(categoryID, 10), map[string]any{"term": term}); err != nil {
		log.Println("Could not audit the removal of a rule:", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// RulesPreviewCategory is what the rules would do for one category.
type RulesPreviewCategory struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Matched int    `json:"matched"` // works the rules say are in it
	Fresh   int    `json:"fresh"`   // among them, the ones that are not in it yet
}

// RulesPreview is what applying the rules would do, without doing it.
type RulesPreview struct {
	Categories []RulesPreviewCategory `json:"categories"`
	// Links are the new places a work would be put in, and Works the works that would be put in some; WithoutNow and WithoutAfter are the works in
	// no category now and after.
	Links        int `json:"links"`
	Works        int `json:"works"`
	WithoutNow   int `json:"withoutNow"`
	WithoutAfter int `json:"withoutAfter"`
}

func (h *CategoryRulesHandler) preview(r *http.Request) (RulesPreview, error) {
	out := RulesPreview{Categories: []RulesPreviewCategory{}}
	tx, err := h.DB.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(r.Context(), `
		WITH `+rulesCTE+`
		SELECT c.id, c.name, (SELECT count(*) FROM hits h WHERE h.category_id = c.id), (SELECT count(*) FROM fresh f WHERE f.category_id = c.id)
		FROM categories c WHERE EXISTS (SELECT 1 FROM category_rules r WHERE r.category_id = c.id)
		ORDER BY unaccent(lower(c.name)), c.id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c RulesPreviewCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Matched, &c.Fresh); err != nil {
			rows.Close()
			return out, err
		}
		out.Categories = append(out.Categories, c)
	}
	rows.Close()
	err = tx.QueryRowContext(r.Context(), `
		WITH `+rulesCTE+`
		SELECT (SELECT count(*) FROM fresh), (SELECT count(DISTINCT work_id) FROM fresh),
		       (SELECT count(*) FROM works w WHERE w.retired_at IS NULL AND NOT EXISTS (SELECT 1 FROM work_categories wc WHERE wc.work_id = w.id)),
		       (SELECT count(*) FROM works w WHERE w.retired_at IS NULL AND NOT EXISTS (SELECT 1 FROM work_categories wc WHERE wc.work_id = w.id)
		          AND NOT EXISTS (SELECT 1 FROM fresh f WHERE f.work_id = w.id))`).Scan(&out.Links, &out.Works, &out.WithoutNow, &out.WithoutAfter)
	return out, err
}

// Preview answers GET /admin/categories/rules/preview: what applying the rules would do. Nothing changes.
func (h *CategoryRulesHandler) Preview(w http.ResponseWriter, r *http.Request) {
	out, err := h.preview(r)
	if err != nil {
		log.Println("Error previewing the category rules:", err)
		http.Error(w, "Error previewing the rules", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Apply answers POST /admin/categories/rules/apply {"links": N}: puts the works in the categories the rules say, but only if the preview
// the person looked at, which had N new links, is still what the rules would do (else nothing is done, and they look again).
func (h *CategoryRulesHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Links *int `json:"links"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil || req.Links == nil || *req.Links < 0 {
		http.Error(w, "links is the number of new places the preview showed", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var links, works int
	if err := tx.QueryRowContext(r.Context(), `WITH `+rulesCTE+` SELECT count(*), count(DISTINCT work_id) FROM fresh`).Scan(&links, &works); err != nil {
		log.Println("Error counting what the rules would do:", err)
		http.Error(w, "Error applying the rules", http.StatusInternalServerError)
		return
	}
	if links != *req.Links {
		http.Error(w, "The library changed since the preview: look at it again", http.StatusConflict)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		WITH `+rulesCTE+`
		INSERT INTO work_categories (work_id, category_id, assigned_by, source)
		SELECT work_id, category_id, NULLIF($1, '')::uuid, 'rule' FROM fresh ON CONFLICT DO NOTHING`, currentUserID(r)); err != nil {
		log.Println("Error applying the category rules:", err)
		http.Error(w, "Error applying the rules", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "category.rules_applied", "category", "", map[string]any{"links": links, "works": works}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error applying the rules", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links, "works": works})
}
