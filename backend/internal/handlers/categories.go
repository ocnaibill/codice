package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// CategoriesHandler is the tree of categories that owner and admin keep (DEC-140) and what each work has in it. Everyone who is signed in
// may read the tree, because the library is navigated by it; only the staff changes it.
type CategoriesHandler struct{ DB *sql.DB }

const (
	maxCategoryName = 80
	// maxCategoryDepth: a category, its subcategory and the one below it ("Mangá" > "Seinen" > ...). More than that is not navigation.
	maxCategoryDepth = 3
)

// Category is a place of the tree, with how many works are in it (Works, counting the ones in its subcategories) and how many are in it
// directly (Own). Retired works do not count.
type Category struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parentId"`
	Name     string `json:"name"`
	Works    int    `json:"works"`
	Own      int    `json:"own"`
}

// WorkCategory is a category a work was put in, with where it is in the tree ("Mangá › Seinen").
type WorkCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// tidyCategoryName is a name as it is kept: one line, single spaces. It says whether it can be a name.
func tidyCategoryName(raw string) (string, bool) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" || utf8.RuneCountInString(name) > maxCategoryName {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return name, true
}

// List answers GET /categories: the whole tree, flat, each with its parent; the client nests it.
func (h *CategoriesHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `
		WITH RECURSIVE tree AS (
			SELECT id, id AS root FROM categories
			UNION ALL
			SELECT c.id, t.root FROM categories c JOIN tree t ON c.parent_id = t.id
		)
		SELECT c.id, c.parent_id, c.name,
		       (SELECT count(DISTINCT wc.work_id) FROM tree t JOIN work_categories wc ON wc.category_id = t.id
		         JOIN works w ON w.id = wc.work_id AND w.retired_at IS NULL WHERE t.root = c.id),
		       (SELECT count(*) FROM work_categories wc JOIN works w ON w.id = wc.work_id AND w.retired_at IS NULL WHERE wc.category_id = c.id)
		FROM categories c
		ORDER BY unaccent(lower(c.name)), c.id`)
	if err != nil {
		log.Println("Error listing the categories:", err)
		http.Error(w, "Error listing the categories", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		var parent sql.NullInt64
		if err := rows.Scan(&c.ID, &parent, &c.Name, &c.Works, &c.Own); err != nil {
			log.Println("Error reading a category:", err)
			http.Error(w, "Error listing the categories", http.StatusInternalServerError)
			return
		}
		if parent.Valid {
			c.ParentID = &parent.Int64
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

type categoryBody struct {
	Name     string `json:"name"`
	ParentID *int64 `json:"parentId"`
}

func categoryID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

// depthOf is how many levels the chain from the category up to the root has (a category with no parent is 1). It is 0 for no category.
func depthOf(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
	var d int
	err := tx.QueryRowContext(ctx, `
		WITH RECURSIVE up AS (SELECT id, parent_id, 1 AS d FROM categories WHERE id = $1
		                      UNION ALL SELECT c.id, c.parent_id, up.d + 1 FROM categories c JOIN up ON c.id = up.parent_id)
		SELECT COALESCE(max(d), 0) FROM up`, id).Scan(&d)
	return d, err
}

// heightOf is how many levels the category and what is under it have (a category with no subcategory is 1).
func heightOf(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
	var d int
	err := tx.QueryRowContext(ctx, `
		WITH RECURSIVE down AS (SELECT id, 1 AS d FROM categories WHERE id = $1
		                        UNION ALL SELECT c.id, down.d + 1 FROM categories c JOIN down ON c.parent_id = down.id)
		SELECT COALESCE(max(d), 0) FROM down`, id).Scan(&d)
	return d, err
}

// isUnder says whether `id` is `ancestor` or is somewhere under it.
func isUnder(ctx context.Context, tx *sql.Tx, id, ancestor int64) (bool, error) {
	var found bool
	err := tx.QueryRowContext(ctx, `
		WITH RECURSIVE up AS (SELECT id, parent_id FROM categories WHERE id = $1
		                      UNION ALL SELECT c.id, c.parent_id FROM categories c JOIN up ON c.id = up.parent_id)
		SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, id, ancestor).Scan(&found)
	return found, err
}

func isDuplicateName(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505"
}

// Create answers POST /admin/categories: a category at the top, or under another.
func (h *CategoriesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req categoryBody
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	name, ok := tidyCategoryName(req.Name)
	if !ok {
		http.Error(w, "A category needs a name of up to 80 characters", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	if req.ParentID != nil {
		d, err := depthOf(r.Context(), tx, *req.ParentID)
		if err != nil {
			http.Error(w, "Error creating the category", http.StatusInternalServerError)
			return
		}
		if d == 0 {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}
		if d >= maxCategoryDepth {
			http.Error(w, "Categories go at most 3 levels deep", http.StatusBadRequest)
			return
		}
	}
	var id int64
	err = tx.QueryRowContext(r.Context(), `INSERT INTO categories (parent_id, name, created_by) VALUES ($1, $2, NULLIF($3, '')::uuid) RETURNING id`,
		req.ParentID, name, currentUserID(r)).Scan(&id)
	if isDuplicateName(err) {
		http.Error(w, "A category with that name already exists there", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error creating a category:", err)
		http.Error(w, "Error creating the category", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "category.create", "category", strconv.FormatInt(id, 10), map[string]any{"name": name, "parentId": req.ParentID}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error creating the category", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, Category{ID: id, ParentID: req.ParentID, Name: name})
}

// Update answers PUT /admin/categories/{id}: the new name and the new place (parentId null is the top).
func (h *CategoriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := categoryID(r)
	var req categoryBody
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	name, valid := tidyCategoryName(req.Name)
	if !valid {
		http.Error(w, "A category needs a name of up to 80 characters", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var oldName string
	var oldParent sql.NullInt64
	if err := tx.QueryRowContext(r.Context(), `SELECT name, parent_id FROM categories WHERE id = $1 FOR UPDATE`, id).Scan(&oldName, &oldParent); err != nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if req.ParentID != nil {
		inside, err := isUnder(r.Context(), tx, *req.ParentID, id)
		if err != nil {
			http.Error(w, "Error updating the category", http.StatusInternalServerError)
			return
		}
		if inside {
			http.Error(w, "A category cannot go inside itself", http.StatusBadRequest)
			return
		}
		d, err := depthOf(r.Context(), tx, *req.ParentID)
		if err != nil {
			http.Error(w, "Error updating the category", http.StatusInternalServerError)
			return
		}
		if d == 0 {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}
		height, err := heightOf(r.Context(), tx, id)
		if err != nil {
			http.Error(w, "Error updating the category", http.StatusInternalServerError)
			return
		}
		if d+height > maxCategoryDepth {
			http.Error(w, "Categories go at most 3 levels deep", http.StatusBadRequest)
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE categories SET name = $2, parent_id = $3 WHERE id = $1`, id, name, req.ParentID)
	if isDuplicateName(err) {
		http.Error(w, "A category with that name already exists there", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error updating a category:", err)
		http.Error(w, "Error updating the category", http.StatusInternalServerError)
		return
	}
	var oldParentID any
	if oldParent.Valid {
		oldParentID = oldParent.Int64
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "category.update", "category", strconv.FormatInt(id, 10),
		map[string]any{"name": name, "was": oldName, "parentId": req.ParentID, "wasParentId": oldParentID}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error updating the category", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, Category{ID: id, ParentID: req.ParentID, Name: name})
}

// Delete answers DELETE /admin/categories/{id}. A category with subcategories is not deleted (they would be lost with it); the works that
// were in it are not touched, they only stop being in it.
func (h *CategoriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := categoryID(r)
	if !ok {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var name string
	if err := tx.QueryRowContext(r.Context(), `SELECT name FROM categories WHERE id = $1 FOR UPDATE`, id).Scan(&name); err != nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	var children, works int
	tx.QueryRowContext(r.Context(), `SELECT count(*) FROM categories WHERE parent_id = $1`, id).Scan(&children)
	if children > 0 {
		http.Error(w, "Move or delete its subcategories first", http.StatusConflict)
		return
	}
	tx.QueryRowContext(r.Context(), `SELECT count(*) FROM work_categories WHERE category_id = $1`, id).Scan(&works)
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM categories WHERE id = $1`, id); err != nil {
		log.Println("Error deleting a category:", err)
		http.Error(w, "Error deleting the category", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "category.delete", "category", strconv.FormatInt(id, 10), map[string]any{"name": name, "works": works}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error deleting the category", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// loadWorkCategories reads the categories a work is in, each with where it is in the tree, by that path.
func loadWorkCategories(ctx context.Context, db *sql.DB, workID int) ([]WorkCategory, error) {
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE up AS (
			SELECT c.id AS leaf, c.id, c.parent_id, c.name, 1 AS d FROM categories c JOIN work_categories wc ON wc.category_id = c.id WHERE wc.work_id = $1
			UNION ALL
			SELECT up.leaf, p.id, p.parent_id, p.name, up.d + 1 FROM categories p JOIN up ON p.id = up.parent_id
		)
		SELECT leaf, (SELECT name FROM up u WHERE u.leaf = up.leaf AND u.d = 1), string_agg(name, ' › ' ORDER BY d DESC)
		FROM up GROUP BY leaf ORDER BY 3`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkCategory{}
	for rows.Next() {
		var c WorkCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Path); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetForWork answers PUT /works/{id}/categories: the set of categories the work is in, from now on. The ones not named stop being.
func (h *CategoriesHandler) SetForWork(w http.ResponseWriter, r *http.Request) {
	workID, err := strconv.Atoi(chi.URLParam(r, "id"))
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err != nil || workID <= 0 {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req) != nil || req.IDs == nil {
		http.Error(w, "ids are the categories of the work", http.StatusBadRequest)
		return
	}
	seen := map[int64]bool{}
	ids := []int64{}
	for _, id := range req.IDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM works WHERE id = $1)`, workID).Scan(&exists); err != nil || !exists {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var known int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM categories WHERE id = ANY($1)`, pq.Array(ids)).Scan(&known); err != nil {
		http.Error(w, "Error setting the categories", http.StatusInternalServerError)
		return
	}
	if known != len(ids) {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM work_categories WHERE work_id = $1 AND NOT (category_id = ANY($2))`, workID, pq.Array(ids)); err != nil {
		http.Error(w, "Error setting the categories", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO work_categories (work_id, category_id, assigned_by) SELECT $1, c, NULLIF($3, '')::uuid FROM unnest($2::bigint[]) AS c
		ON CONFLICT DO NOTHING`, workID, pq.Array(ids), currentUserID(r)); err != nil {
		http.Error(w, "Error setting the categories", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "work.categories", "work", strconv.Itoa(workID), map[string]any{"ids": ids}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error setting the categories", http.StatusInternalServerError)
		return
	}
	cats, err := loadWorkCategories(r.Context(), h.DB, workID)
	if err != nil {
		http.Error(w, "Error reading the categories", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}
