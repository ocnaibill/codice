package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/backup"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

// BackupAdminHandler is what the "Sistema" tab of the administration shows (UI-19, DEC-123): when the
// latest backup was made, which package it is and whether it was checked, the free space of the
// storage, and how the job queue is doing. Making and restoring backups is done with the
// codice-admin command on the server: it needs access to the machine, and a restore replaces the
// database, so a restore is deliberately not something an HTTP request can start.
type BackupAdminHandler struct {
	DB          *sql.DB
	StoragePath string
	// Panel is what the owner's two buttons run (DEC-123). Its zero value means "not set up".
	Panel backup.Panel
}

// The job types of the two buttons. They run in the API process, which has pg_dump.
const (
	JobBackup       = "backup"
	JobVerifyBackup = "verify_backup"
)

type jobView struct {
	ID         int64      `json:"id"`
	Type       string     `json:"type"`
	State      string     `json:"state"`
	Name       string     `json:"name,omitempty"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	// Error is what went wrong, for the owner: it can name paths and tools of the server.
	Error string `json:"error,omitempty"`
}

type panelView struct {
	Enabled bool `json:"enabled"`
	// Dir is where the packages made here go: the owner's, like the path of a package.
	Dir      string               `json:"dir,omitempty"`
	Packages []backup.PackageInfo `json:"packages"`
	Job      *jobView             `json:"job"`
}

type backupView struct {
	backup.LastBackup
	Verified *verifiedView `json:"verified"`
	// SameDisk is true when the package is on the same filesystem as the storage: a failed disk would
	// take both. nil when this process cannot tell (the package is somewhere it does not see).
	SameDisk *bool `json:"sameDisk"`
}

type verifiedView struct {
	At   time.Time `json:"at"`
	Deep bool      `json:"deep"`
}

type queueView struct {
	Pending       int        `json:"pending"`
	Running       int        `json:"running"`
	FailedRecent  int        `json:"failedRecent"`
	OldestWaiting *time.Time `json:"oldestWaiting"`
}

func (h *BackupAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	last, err := backup.Last(ctx, h.DB)
	if err != nil {
		http.Error(w, "Error reading the last backup", http.StatusInternalServerError)
		return
	}
	verified, err := backup.LastVerified(ctx, h.DB)
	if err != nil {
		http.Error(w, "Error reading the last backup", http.StatusInternalServerError)
		return
	}
	var queue queueView
	rows, err := h.DB.QueryContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE state = 'pending'),
			COUNT(*) FILTER (WHERE state = 'running'),
			COUNT(*) FILTER (WHERE state = 'failed' AND updated_at > now() - interval '24 hours'),
			MIN(run_at) FILTER (WHERE state = 'pending' AND run_at <= now())
		FROM jobs`)
	if err != nil {
		http.Error(w, "Error reading the queue", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	if rows.Next() {
		var oldest sql.NullTime
		if err := rows.Scan(&queue.Pending, &queue.Running, &queue.FailedRecent, &oldest); err != nil {
			http.Error(w, "Error reading the queue", http.StatusInternalServerError)
			return
		}
		if oldest.Valid {
			queue.OldestWaiting = &oldest.Time
		}
	}

	var view *backupView
	if last != nil {
		v := backupView{LastBackup: *last}
		// Which package was checked: only a check of THIS package says anything about it.
		if verified != nil && last.Name != "" && verified.Name == last.Name {
			v.Verified = &verifiedView{At: verified.At, Deep: verified.Deep}
		}
		if last.Path != "" && h.StoragePath != "" {
			if same, ok := sameDevice(filepath.Dir(last.Path), h.StoragePath); ok {
				v.SameDisk = &same
			}
		}
		// The address of the package is the owner's: an administrator may not even have access to the
		// server, and a path of the server's own is of no use to them (DEC-123).
		if role, _ := ctx.Value(middleware.UserRoleKey).(string); role != "owner" {
			v.Path = ""
		}
		view = &v
	}

	owner := false
	if role, _ := ctx.Value(middleware.UserRoleKey).(string); role == "owner" {
		owner = true
	}
	panel, err := h.panelView(r, owner)
	if err != nil {
		http.Error(w, "Error reading the backups", http.StatusInternalServerError)
		return
	}

	out := map[string]any{"lastBackup": view, "queue": queue, "storage": nil, "panel": panel}
	if free, total, ok := diskUsage(h.StoragePath); ok && h.StoragePath != "" {
		out["storage"] = map[string]uint64{"freeBytes": free, "totalBytes": total}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// panelView is what the interface needs to draw the buttons: whether they are set up, the packages
// there are and how the last job of either button went.
func (h *BackupAdminHandler) panelView(r *http.Request, owner bool) (panelView, error) {
	v := panelView{Enabled: h.Panel.Enabled(), Packages: []backup.PackageInfo{}}
	if !v.Enabled {
		return v, nil
	}
	if owner {
		v.Dir = h.Panel.Dir
	}
	packages, err := h.Panel.Packages()
	if err != nil {
		return v, err
	}
	if packages != nil {
		v.Packages = packages
	}
	var (
		j        jobView
		name     sql.NullString
		started  sql.NullTime
		finished sql.NullTime
		lastErr  sql.NullString
	)
	err = h.DB.QueryRowContext(r.Context(), `
		SELECT id, type, state, payload->>'name', started_at, finished_at, last_error
		FROM jobs WHERE type IN ($1, $2) ORDER BY id DESC LIMIT 1`, JobBackup, JobVerifyBackup).
		Scan(&j.ID, &j.Type, &j.State, &name, &started, &finished, &lastErr)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	j.Name = name.String
	if started.Valid {
		j.StartedAt = &started.Time
	}
	if finished.Valid {
		j.FinishedAt = &finished.Time
	}
	if owner && j.State == "failed" {
		j.Error = lastErr.String
	}
	v.Job = &j
	return v, nil
}

type panelRequest struct {
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Run queues the making of one package, for the owner (DEC-123).
func (h *BackupAdminHandler) Run(w http.ResponseWriter, r *http.Request) {
	h.queue(w, r, JobBackup, false)
}

// Verify queues the check of one package of the backup folder, for the owner.
func (h *BackupAdminHandler) Verify(w http.ResponseWriter, r *http.Request) {
	h.queue(w, r, JobVerifyBackup, true)
}

// queue is what both buttons share. The owner's password is asked again, because a session alone (a
// token that leaked, a computer left open) must not be enough to make the server write a copy of
// the whole library; only one of these jobs lives at a time; and the request is audited.
func (h *BackupAdminHandler) queue(w http.ResponseWriter, r *http.Request, jobType string, needsName bool) {
	ctx := r.Context()
	if !h.Panel.Enabled() {
		http.Error(w, "Backups from the panel are not set up", http.StatusConflict)
		return
	}
	var req panelRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || req.Password == "" {
		http.Error(w, "The password is required", http.StatusBadRequest)
		return
	}
	userID := currentUserID(r)
	ok, err := ownPasswordMatches(ctx, h.DB, userID, req.Password)
	if err != nil {
		http.Error(w, "Error checking the password", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "The password is not correct", http.StatusForbidden)
		return
	}
	payload := map[string]any{}
	if needsName {
		packages, err := h.Panel.Packages()
		if err != nil {
			http.Error(w, "Error reading the backup folder", http.StatusInternalServerError)
			return
		}
		found := false
		for _, p := range packages {
			found = found || p.Name == req.Name
		}
		if !found {
			http.Error(w, "That is not a package in the backup folder", http.StatusNotFound)
			return
		}
		payload["name"] = req.Name
	}
	body, _ := json.Marshal(payload)

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	// One at a time: both jobs run pg_dump or pg_restore against the same database, and two at once
	// only make each other slow. The lock makes "check, then insert" safe against a second click.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7301200)`); err != nil {
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	var live int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM jobs WHERE type IN ($1, $2) AND state IN ('pending', 'running') LIMIT 1`, JobBackup, JobVerifyBackup).Scan(&live)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": "A backup job is already running", "job_id": live})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	// One attempt: a backup that fails is shown to the owner, who decides, instead of being retried
	// by itself in the background.
	var id int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO jobs (type, payload, priority, max_attempts, created_by)
		VALUES ($1, $2::jsonb, $3, 1, NULLIF($4, '')::uuid) RETURNING id`,
		jobType, body, jobs.PriorityManual, userID).Scan(&id); err != nil {
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	details := map[string]any{"job": id}
	if needsName {
		details["name"] = req.Name
	}
	if err := audit.Record(ctx, tx, userID, "backup.requested", "instance", "database", details); err != nil {
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"job_id": id})
}
