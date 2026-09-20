package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/backup"
)

// BackupAdminHandler shows when the latest backup was made on this instance (UI-19).
// Making and restoring backups is done with the codice-admin command on the server:
// it needs access to the machine, and a restore replaces the database, so it is
// deliberately not something an HTTP request can start.
type BackupAdminHandler struct {
	DB *sql.DB
}

func (h *BackupAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	last, err := backup.Last(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the last backup", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"lastBackup": last})
}
