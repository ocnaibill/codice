package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/jobs"
)

// JobExportData is the job that makes the file of "Exportar meus dados". It runs in the API process.
const JobExportData = "export_data"

const (
	// ExportTTL is how long the file is kept once it is ready: a copy of everything a person has is not left lying around.
	ExportTTL = 24 * time.Hour
	// MaxExportsPerHour is how many files one person may ask for in an hour.
	MaxExportsPerHour = 3
	// staleAfter is how long a request may stay "being made" before it is taken to have died with the server.
	staleAfter = 2 * time.Hour
)

var exportIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DataExportHandler is "Exportar meus dados" (UI-22, DEC-121 line): a person asks for a ZIP with everything that is theirs (their
// notes, reading, favorites, preferences, devices and apps, and the record of their own sign-ins), it is made by a job, kept for 24
// hours in a folder of the server that nothing else serves, and only the person it belongs to can take it. It has no book and no
// secret in it: no password, no token.
type DataExportHandler struct {
	DB *sql.DB
	// Dir is where the files wait (CODICE_EXPORT_DIR; by default a folder in the system's temporary directory, which also means a
	// restart of the server takes them: they are a day old at most anyway).
	Dir string
}

type exportView struct {
	ID          string     `json:"id"`
	State       string     `json:"state"` // pending, ready, failed or expired
	RequestedAt time.Time  `json:"requestedAt"`
	ReadyAt     *time.Time `json:"readyAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Bytes       int64      `json:"bytes,omitempty"`
}

func (h *DataExportHandler) path(id string) string { return filepath.Join(h.Dir, id+".zip") }

func (h *DataExportHandler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// view reads one request of a person and says what state it is really in: a file that is ready but past its day, or gone, is expired.
func (h *DataExportHandler) view(ctx context.Context, userID, id string) (*exportView, error) {
	var v exportView
	var state string
	var ready, expires sql.NullTime
	var bytes sql.NullInt64
	q := `SELECT id, state, requested_at, ready_at, expires_at, bytes FROM data_exports WHERE user_id = $1 `
	args := []any{userID}
	if id != "" {
		q += `AND id = $2`
		args = append(args, id)
	} else {
		q += `AND NOT superseded ORDER BY requested_at DESC LIMIT 1`
	}
	err := h.DB.QueryRowContext(ctx, q, args...).Scan(&v.ID, &state, &v.RequestedAt, &ready, &expires, &bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.State = state
	if ready.Valid {
		v.ReadyAt = &ready.Time
	}
	if expires.Valid {
		v.ExpiresAt = &expires.Time
	}
	v.Bytes = bytes.Int64
	if state == "ready" {
		if _, statErr := os.Stat(h.path(v.ID)); !expires.Valid || !expires.Time.After(time.Now()) || statErr != nil {
			v.State = "expired"
		}
	}
	if state == "pending" && time.Since(v.RequestedAt) > staleAfter {
		v.State = "failed"
	}
	return &v, nil
}

// Latest answers the caller's most recent request, or {"export": null}.
func (h *DataExportHandler) Latest(w http.ResponseWriter, r *http.Request) {
	v, err := h.view(r.Context(), currentUserID(r), "")
	if err != nil {
		http.Error(w, "Error reading the export", http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"export": v})
}

// Request asks for a file. One at a time for a person, and a few an hour: it reads everything they have.
func (h *DataExportHandler) Request(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := currentUserID(r)
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	// The lock makes "look, then insert" safe against a second click of the same person.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('data_export:' || $1))`, userID); err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	var live string
	err = tx.QueryRowContext(ctx, `SELECT id FROM data_exports WHERE user_id = $1 AND state = 'pending' AND requested_at > now() - $2::interval LIMIT 1`,
		userID, fmt.Sprintf("%d seconds", int(staleAfter.Seconds()))).Scan(&live)
	if err == nil {
		h.writeJSON(w, http.StatusConflict, map[string]any{"error": "An export is already being made", "id": live})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	var recent int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM data_exports WHERE user_id = $1 AND requested_at > now() - interval '1 hour'`, userID).Scan(&recent); err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	if recent >= MaxExportsPerHour {
		http.Error(w, "Too many exports: try again in a while", http.StatusTooManyRequests)
		return
	}
	// A new request replaces the file of the one before it: the old file goes, and its row stays for the hour, so that the count
	// of requests above is not erased by the request it counts.
	rows, err := tx.QueryContext(ctx, `UPDATE data_exports SET expires_at = now(), superseded = true WHERE user_id = $1 AND state = 'ready' AND expires_at > now() RETURNING id`, userID)
	if err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	var old []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		old = append(old, id)
	}
	rows.Close()
	var id string
	if err := tx.QueryRowContext(ctx, `INSERT INTO data_exports (user_id) VALUES ($1) RETURNING id`, userID).Scan(&id); err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	// One attempt: a failure is shown to the person, who asks again, instead of being retried in the background.
	payload, _ := json.Marshal(map[string]string{"export_id": id})
	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs (type, payload, priority, max_attempts, created_by) VALUES ($1, $2::jsonb, $3, 1, $4::uuid)`,
		JobExportData, payload, jobs.PriorityManual, userID); err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(ctx, tx, userID, "data_export.request", "user", userID, map[string]any{"export": id}); err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error queueing the export", http.StatusInternalServerError)
		return
	}
	for _, o := range old {
		os.Remove(h.path(o))
	}
	h.writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

// Download hands the file to the person it belongs to, while it is ready and within its day.
func (h *DataExportHandler) Download(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := currentUserID(r)
	if !exportIDPattern.MatchString(id) {
		http.Error(w, "Export not found", http.StatusNotFound)
		return
	}
	v, err := h.view(r.Context(), userID, id)
	if err != nil {
		http.Error(w, "Error reading the export", http.StatusInternalServerError)
		return
	}
	if v == nil {
		// Someone else's, or none: the same answer, so a stranger learns nothing.
		http.Error(w, "Export not found", http.StatusNotFound)
		return
	}
	if v.State != "ready" {
		http.Error(w, "The export is not ready", http.StatusGone)
		return
	}
	f, err := os.Open(h.path(id))
	if err != nil {
		http.Error(w, "The export is not ready", http.StatusGone)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "The export is not ready", http.StatusGone)
		return
	}
	h.DB.ExecContext(r.Context(), `UPDATE data_exports SET downloaded_at = now() WHERE id = $1`, id)
	audit.Record(r.Context(), h.DB, userID, "data_export.download", "user", userID, map[string]any{"export": id})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="codice-meus-dados-`+time.Now().Format("20060102")+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// Delete removes the file and its record before the day is out, for a person who has taken it and wants it gone.
func (h *DataExportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !exportIDPattern.MatchString(id) {
		http.Error(w, "Export not found", http.StatusNotFound)
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM data_exports WHERE id = $1 AND user_id = $2`, id, currentUserID(r))
	if err != nil {
		http.Error(w, "Error removing the export", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "Export not found", http.StatusNotFound)
		return
	}
	os.Remove(h.path(id))
	w.WriteHeader(http.StatusNoContent)
}

// Prepare is the job: it writes the ZIP of the request aside, renames it when whole, and marks the request ready (or failed).
func (h *DataExportHandler) Prepare(ctx context.Context, exportID string) error {
	var userID string
	if err := h.DB.QueryRowContext(ctx, `SELECT user_id FROM data_exports WHERE id = $1 AND state = 'pending'`, exportID).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return jobs.Permanent(errors.New("the request is gone or was already answered"))
		}
		return err
	}
	fail := func(err error) error {
		log.Printf("data export %s: %v", exportID, err)
		h.DB.ExecContext(context.WithoutCancel(ctx), `UPDATE data_exports SET state = 'failed', error = $2 WHERE id = $1`, exportID, err.Error())
		return jobs.Permanent(err)
	}
	if err := os.MkdirAll(h.Dir, 0o700); err != nil {
		return fail(err)
	}
	final := h.path(exportID)
	partial := final + ".partial"
	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fail(err)
	}
	defer os.Remove(partial)
	err = BuildDataExport(ctx, h.DB, userID, f, time.Now())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(err)
	}
	if err := os.Rename(partial, final); err != nil {
		return fail(err)
	}
	info, err := os.Stat(final)
	if err != nil {
		return fail(err)
	}
	if _, err := h.DB.ExecContext(ctx, `UPDATE data_exports SET state = 'ready', bytes = $2, ready_at = now(), expires_at = now() + $3::interval WHERE id = $1`,
		exportID, info.Size(), fmt.Sprintf("%d seconds", int(ExportTTL.Seconds()))); err != nil {
		os.Remove(final)
		return fail(err)
	}
	return nil
}

// Purge takes away the files that are past their day, the requests that are an hour past it (or failed a week ago), and any file
// that no request owns any more. It says how many requests went.
func (h *DataExportHandler) Purge(ctx context.Context) (int, error) {
	// A request that never answered (the server stopped in the middle) is a failure, not a wait.
	h.DB.ExecContext(ctx, `UPDATE data_exports SET state = 'failed', error = 'interrupted' WHERE state = 'pending' AND requested_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int(staleAfter.Seconds())))
	// The file of a request goes the moment its day is over; the row stays another hour, because the limit on requests counts rows.
	expired, err := h.DB.QueryContext(ctx, `SELECT id FROM data_exports WHERE state = 'ready' AND expires_at < now()`)
	if err != nil {
		return 0, err
	}
	for expired.Next() {
		var id string
		expired.Scan(&id)
		os.Remove(h.path(id))
	}
	expired.Close()
	rows, err := h.DB.QueryContext(ctx, `DELETE FROM data_exports
		WHERE (state = 'ready' AND expires_at < now() - interval '1 hour') OR (state = 'failed' AND requested_at < now() - interval '7 days')
		RETURNING id`)
	if err != nil {
		return 0, err
	}
	n := 0
	for rows.Next() {
		var id string
		rows.Scan(&id)
		os.Remove(h.path(id))
		n++
	}
	rows.Close()
	h.removeStrays(ctx)
	return n, nil
}

// removeStrays deletes files in the folder that no request owns (left by a crash), once they are an hour old: a file being written
// right now is not one.
func (h *DataExportHandler) removeStrays(ctx context.Context) {
	entries, err := os.ReadDir(h.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".partial"), ".zip")
		info, err := e.Info()
		if err != nil || !e.Type().IsRegular() || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		var exists bool
		if exportIDPattern.MatchString(id) {
			h.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM data_exports WHERE id = $1)`, id).Scan(&exists)
		}
		if !exists || strings.HasSuffix(name, ".partial") {
			os.Remove(filepath.Join(h.Dir, name))
		}
	}
}
