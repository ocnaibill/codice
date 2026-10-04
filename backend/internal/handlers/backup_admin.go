package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ocnaibill/codice/backend/internal/backup"
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

	out := map[string]any{"lastBackup": view, "queue": queue, "storage": nil}
	if free, total, ok := diskUsage(h.StoragePath); ok && h.StoragePath != "" {
		out["storage"] = map[string]uint64{"freeBytes": free, "totalBytes": total}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
