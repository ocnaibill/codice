package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// StorageHandler is the administration of where files live (RF-044). Reorganizing
// is always explicit: first a preview, then a confirmation of that exact preview.
type StorageHandler struct {
	Mover       *storage.Mover
	DB          *sql.DB
	StoragePath string
}

// PreviewReorganize lists what a reorganization would move, and to where, and a
// hash of that plan. Nothing is moved.
func (h *StorageHandler) PreviewReorganize(w http.ResponseWriter, r *http.Request) {
	plan, err := h.Mover.Plan(r.Context())
	if err != nil {
		log.Println("Error planning reorganization:", err)
		http.Error(w, "Error planning the reorganization", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

type reorganizeRequest struct {
	Hash string `json:"hash"`
}

// Reorganize executes the plan the admin previewed. It plans again and refuses
// (409) if the result is not the plan that was shown.
func (h *StorageHandler) Reorganize(w http.ResponseWriter, r *http.Request) {
	var req reorganizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Hash == "" {
		http.Error(w, "The hash of the previewed plan is required", http.StatusBadRequest)
		return
	}
	res, err := h.Mover.Apply(r.Context(), req.Hash)
	if errors.Is(err, storage.ErrPlanChanged) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error reorganizing storage:", err)
		http.Error(w, "Error reorganizing the storage", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), h.DB, currentUserID(r), "storage.reorganize", "storage", "managed",
		map[string]any{"moved": res.Moved, "failed": len(res.Failures)}); err != nil {
		log.Println("Could not audit the reorganization:", err)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// StorageRoot is a directory the owner has authorised for the referenced library.
type StorageRoot struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

// ListRoots returns the authorised directories and the managed storage directory.
func (h *StorageHandler) ListRoots(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `SELECT id, path FROM storage_roots ORDER BY id`)
	if err != nil {
		http.Error(w, "Error listing roots", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	roots := []StorageRoot{}
	for rows.Next() {
		var sr StorageRoot
		if rows.Scan(&sr.ID, &sr.Path) == nil {
			roots = append(roots, sr)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"roots": roots, "managed": h.StoragePath})
}

type addRootRequest struct {
	Path string `json:"path"`
}

// AddRoot authorises a directory for the referenced library. Only the owner may
// (DEC-035). The directory must exist, is stored by its real path, and must not
// overlap the managed storage or another root: cataloguing the same files twice,
// or the library's own files as if they were someone else's, would be a mistake.
func (h *StorageHandler) AddRoot(w http.ResponseWriter, r *http.Request) {
	var req addRootRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		http.Error(w, "A path is required", http.StatusBadRequest)
		return
	}
	if !filepath.IsAbs(req.Path) {
		http.Error(w, "The path must be absolute", http.StatusBadRequest)
		return
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(req.Path))
	if err != nil {
		http.Error(w, "The directory does not exist or cannot be read", http.StatusBadRequest)
		return
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		http.Error(w, "The path is not a directory", http.StatusBadRequest)
		return
	}
	managed, _ := filepath.EvalSymlinks(h.StoragePath)
	if managed != "" && (isWithin(real, managed) || isWithin(managed, real)) {
		http.Error(w, "The directory overlaps the managed storage", http.StatusBadRequest)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(r.Context(), `SELECT path FROM storage_roots`)
	if err != nil {
		http.Error(w, "Error reading roots", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var other string
		if rows.Scan(&other) == nil && (isWithin(real, other) || isWithin(other, real)) {
			rows.Close()
			http.Error(w, "The directory overlaps an existing root", http.StatusConflict)
			return
		}
	}
	rows.Close()

	var id int
	err = tx.QueryRowContext(r.Context(), `INSERT INTO storage_roots (path, created_by) VALUES ($1, NULLIF($2, '')::uuid) RETURNING id`,
		real, currentUserID(r)).Scan(&id)
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		http.Error(w, "The directory is already a root", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "Error adding the root", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "storage.root_add", "storage_root", strconv.Itoa(id), map[string]any{"path": real}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error adding the root", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(StorageRoot{ID: id, Path: real})
}

// RemoveRoot stops authorising a directory. It refuses while catalogued files
// still point into it: those would be left with a location nobody may read.
func (h *StorageHandler) RemoveRoot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "Root not found", http.StatusNotFound)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var p string
	if err := tx.QueryRowContext(r.Context(), `SELECT path FROM storage_roots WHERE id = $1 FOR UPDATE`, id).Scan(&p); err != nil {
		http.Error(w, "Root not found", http.StatusNotFound)
		return
	}
	var files int
	tx.QueryRowContext(r.Context(), `SELECT count(*) FROM storage_locations WHERE mode = 'referenced' AND root = $1`, p).Scan(&files)
	if files > 0 {
		http.Error(w, "Files in this directory are still catalogued: move them to the managed storage first", http.StatusConflict)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM storage_roots WHERE id = $1`, id); err != nil {
		http.Error(w, "Error removing the root", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "storage.root_remove", "storage_root", strconv.Itoa(id), map[string]any{"path": p}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error removing the root", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scanRequest struct {
	RootID int    `json:"rootId"`
	Subdir string `json:"subdir"`
}

// Scan queues the cataloguing of an authorised directory. Owner and admin may
// scan, but only inside roots the owner authorised, and never outside them. It
// runs as a job (it hashes every new file), so this answers at once with the job id.
func (h *StorageHandler) Scan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RootID <= 0 {
		http.Error(w, "rootId is required", http.StatusBadRequest)
		return
	}
	var root string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT path FROM storage_roots WHERE id = $1`, req.RootID).Scan(&root); err != nil {
		http.Error(w, "Root not found", http.StatusNotFound)
		return
	}
	if req.Subdir != "" {
		if _, ok := storage.SafeRel(req.Subdir); !ok {
			http.Error(w, "The subdirectory is not a safe relative path", http.StatusBadRequest)
			return
		}
	}

	payload, _ := json.Marshal(map[string]any{"root_id": req.RootID, "subdir": req.Subdir})
	// One live scan per directory: asking twice returns the job already waiting.
	var id int64
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT id FROM jobs WHERE type = 'scan' AND state IN ('pending', 'running') AND payload = $1::jsonb LIMIT 1`, payload).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = h.DB.QueryRowContext(r.Context(), `
			INSERT INTO jobs (type, payload, priority, created_by) VALUES ('scan', $1::jsonb, $2, NULLIF($3, '')::uuid) RETURNING id`,
			payload, jobs.PriorityManual, currentUserID(r)).Scan(&id)
	}
	if err != nil {
		log.Println("Error queueing scan:", err)
		http.Error(w, "Error queueing the scan", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"job_id": id})
}

type moveRequest struct {
	FileIDs []int64 `json:"fileIds"`
}

// MoveToManaged queues the transfer of referenced files into the managed
// storage ("Mover para o armazenamento gerenciado"). The original is removed once
// the copy is verified, so this is an explicit action on chosen files (DEC-034).
func (h *StorageHandler) MoveToManaged(w http.ResponseWriter, r *http.Request) {
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 || len(req.FileIDs) > 500 {
		http.Error(w, "fileIds is required (1 to 500 ids)", http.StatusBadRequest)
		return
	}
	type queued struct {
		FileID int64  `json:"fileId"`
		JobID  int64  `json:"jobId,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	out := []queued{}
	for _, id := range req.FileIDs {
		q := queued{FileID: id}
		var workID int
		err := h.DB.QueryRowContext(r.Context(), `
			SELECT e.work_id FROM storage_locations l JOIN files f ON f.id = l.file_id JOIN editions e ON e.id = f.edition_id
			WHERE f.id = $1 AND l.mode = 'referenced' AND l.state = 'ok'`, id).Scan(&workID)
		if errors.Is(err, sql.ErrNoRows) {
			q.Error = "not a referenced file that is available"
			out = append(out, q)
			continue
		}
		if err != nil {
			http.Error(w, "Error looking up the file", http.StatusInternalServerError)
			return
		}
		q.JobID, err = jobs.Enqueue(r.Context(), h.DB, "transfer", workID, map[string]any{"file_id": id}, jobs.PriorityManual, currentUserID(r))
		if err != nil {
			q.Error = "could not queue the transfer"
		}
		out = append(out, q)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// ListCleanups shows originals that a transfer or import could not remove yet,
// and why.
func (h *StorageHandler) ListCleanups(w http.ResponseWriter, r *http.Request) {
	list, err := storage.PendingCleanups(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error listing pending removals", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": list})
}

// RetryCleanups tries again to remove the pending originals. Each is checked
// again first: one that changed is kept.
func (h *StorageHandler) RetryCleanups(w http.ResponseWriter, r *http.Request) {
	removed, remaining, err := storage.RetryCleanups(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error retrying the removals", http.StatusInternalServerError)
		return
	}
	audit.Record(r.Context(), h.DB, currentUserID(r), "storage.cleanup_retry", "storage", "cleanups",
		map[string]any{"removed": removed, "remaining": remaining})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"removed": removed, "remaining": remaining})
}
