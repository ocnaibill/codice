package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/backup"
)

func TestBackupAdmin_ShowsTheLatestBackupOrNone(t *testing.T) {
	db := migratedDB(t)
	h := &BackupAdminHandler{DB: db}
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		h.Get(rec, httptest.NewRequest("GET", "/admin/backup", nil))
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	if got := get(); got["lastBackup"] != nil {
		t.Errorf("before any backup: %v", got)
	}
	at := time.Now().UTC().Truncate(time.Second)
	if err := backup.Record(t.Context(), db, backup.Result{Bytes: 4096, Encrypted: true,
		Manifest: backup.Manifest{CreatedAt: at, IncludesFiles: true, Files: backup.FilesSummary{Included: 3}}}); err != nil {
		t.Fatal(err)
	}
	last, _ := get()["lastBackup"].(map[string]any)
	if last == nil || last["bytes"] != float64(4096) || last["encrypted"] != true || last["includesFiles"] != true || last["files"] != float64(3) {
		t.Errorf("last = %v", last)
	}
}
