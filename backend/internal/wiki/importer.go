package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
)

// The generator (the worker) hands the bundle over in the database and the backend, which owns the importer, stores it and closes the
// run. A run is in that hand-over while its stage is "import" and its state still "running".

// importLease is how long an importer holds a run before another may take it (a backend that died in the middle).
const importLease = 10 * time.Minute

// ImportNext imports the next run that is waiting for it. It says whether there was one.
func ImportNext(ctx context.Context, db *sql.DB, owner string) (bool, error) {
	var id int64
	var origin string
	var requestedBy sql.NullString
	err := db.QueryRowContext(ctx, `
		UPDATE wiki_runs SET lease_owner = $1, heartbeat_at = now(), updated_at = now()
		WHERE id = (SELECT r.id FROM wiki_runs r JOIN wiki_run_bundles b ON b.run_id = r.id
		            WHERE r.state = 'running' AND r.stage = 'import'
		              AND (r.lease_owner IS NULL OR r.lease_owner NOT LIKE 'importer:%' OR r.heartbeat_at < now() - make_interval(secs => $2))
		            ORDER BY r.id LIMIT 1 FOR UPDATE OF r SKIP LOCKED)
		RETURNING id, origin, requested_by::text`, owner, importLease.Seconds()).Scan(&id, &origin, &requestedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, `SELECT bundle FROM wiki_run_bundles WHERE run_id = $1`, id).Scan(&raw); err != nil {
		return true, closeRun(ctx, db, id, "", err)
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return true, closeRun(ctx, db, id, "", fmt.Errorf("o pacote da geração não é um JSON válido: %w", err))
	}
	projectID, err := Import(ctx, db, &b, requestedBy.String)
	if err != nil {
		return true, closeRun(ctx, db, id, "", err)
	}
	if origin == "new_book" {
		// an update a new book brought is for the staff to look at (DEC-180)
		if _, err := db.ExecContext(ctx, `UPDATE wiki_projects SET review = 'pending' WHERE id = $1`, projectID); err != nil {
			log.Println("Error marking a wiki to review:", err)
		}
	}
	return true, closeRun(ctx, db, id, fmt.Sprint(projectID), nil)
}

// closeRun ends the run: done with the project it made, or failed with the sentence for the administration.
func closeRun(ctx context.Context, db *sql.DB, id int64, project string, cause error) error {
	if cause == nil {
		_, err := db.ExecContext(ctx, `UPDATE wiki_runs SET state = 'done', lease_owner = NULL, finished_at = now(), updated_at = now() WHERE id = $1`, id)
		if err == nil {
			_, err = db.ExecContext(ctx, `DELETE FROM wiki_run_bundles WHERE run_id = $1`, id)
		}
		return err
	}
	var bad InputError
	msg := "Não foi possível guardar a wiki: " + cause.Error()
	if errors.As(cause, &bad) {
		msg = bad.Msg
	}
	entry, _ := json.Marshal(map[string]string{"at": time.Now().UTC().Format(time.RFC3339), "message": msg})
	_, err := db.ExecContext(ctx, `
		UPDATE wiki_runs SET state = 'failed', lease_owner = NULL, finished_at = now(), updated_at = now(),
		       errors = (CASE WHEN jsonb_array_length(errors) >= 10 THEN errors - 0 ELSE errors END) || $2::jsonb
		WHERE id = $1`, id, "["+string(entry)+"]")
	if err == nil {
		_, err = db.ExecContext(ctx, `DELETE FROM wiki_run_bundles WHERE run_id = $1`, id)
	}
	if cause != nil {
		log.Println("A wiki run could not be imported:", id, cause)
	}
	return err
}

// RunImporter imports the bundles of the runs that are done, until the context ends.
func RunImporter(ctx context.Context, db *sql.DB, every time.Duration) {
	owner := fmt.Sprintf("importer:%d", os.Getpid())
	for {
		worked, err := ImportNext(ctx, db, owner)
		if err != nil && ctx.Err() == nil {
			log.Println("Error importing a wiki:", err)
		}
		if worked && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
