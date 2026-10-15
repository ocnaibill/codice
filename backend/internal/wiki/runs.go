package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// The queue of generations and the list the administration chooses from (DEC-185). A wiki is of a series (an official collection)
// or of a work in none; only books whose EPUB has its text extracted can have one, since the generator reads that text.

// Scope is a work or a series.
type Scope struct {
	Kind string `json:"kind"` // work or series
	ID   int64  `json:"id"`
}

func (s Scope) valid() bool { return (s.Kind == "work" || s.Kind == "series") && s.ID > 0 }

// columns are the work_id and collection_id of the scope.
func (s Scope) columns() (sql.NullInt64, sql.NullInt64) {
	if s.Kind == "work" {
		return sql.NullInt64{Int64: s.ID, Valid: true}, sql.NullInt64{}
	}
	return sql.NullInt64{}, sql.NullInt64{Int64: s.ID, Valid: true}
}

var (
	// ErrNoProvider says no generation can start: the owner has not configured the provider (DEC-177).
	ErrNoProvider = errors.New("no provider")
	// ErrNotEligible says the work or series has no book whose text the generator can read.
	ErrNotEligible = errors.New("not eligible")
	// ErrState says the run is not in a state that allows what was asked.
	ErrState = errors.New("not in that state")
)

// hasText is true for a work with an EPUB whose text was extracted.
const hasText = `EXISTS (SELECT 1 FROM editions e JOIN files f ON f.edition_id = e.id JOIN text_extractions te ON te.file_id = f.id
	WHERE e.work_id = %s AND lower(f.format) = 'epub' AND f.availability = 'available' AND te.status = 'ready')`

// units are the series and the works in no series that have at least one book with text, with their title and how many books.
var units = `
	WITH units AS (
		SELECT 'series' AS kind, c.id::bigint AS id, c.name::text AS title, count(*)::int AS books,
		       count(*) FILTER (WHERE ` + strings.Replace(hasText, "%s", "w.id", 1) + `)::int AS readable
		FROM collections c
		JOIN collection_works cw ON cw.collection_id = c.id AND cw.official
		JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
		WHERE c.kind = 'official' AND c.retired_at IS NULL
		GROUP BY c.id
		UNION ALL
		SELECT 'work', w.id::bigint, w.original_title::text, 1,
		       CASE WHEN ` + strings.Replace(hasText, "%s", "w.id", 1) + ` THEN 1 ELSE 0 END
		FROM works w
		WHERE w.retired_at IS NULL AND NOT EXISTS (
			SELECT 1 FROM collection_works cw JOIN collections c ON c.id = cw.collection_id
			WHERE cw.work_id = w.id AND cw.official AND c.kind = 'official' AND c.retired_at IS NULL)
	)`

// LibraryItem is a work or a series as the administration lists it.
type LibraryItem struct {
	Scope
	Title    string     `json:"title"`
	Books    int        `json:"books"`
	Readable int        `json:"readable"` // books with text the generator can read
	Excluded bool       `json:"excluded"`
	Wiki     *WikiState `json:"wiki"`
	Run      *RunState  `json:"run"`
}

// WikiState is the wiki a work or series has.
type WikiState struct {
	ProjectID   int64     `json:"projectId"`
	State       string    `json:"state"`
	Review      string    `json:"review"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// RunState is the live run of a work or series.
type RunState struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
	Stage string `json:"stage,omitempty"`
}

// Library lists the works and series that can have a wiki, by title, filtered by what is typed and by state ("" for all,
// "without", "with", "excluded", "live").
func Library(ctx context.Context, db *sql.DB, q, filter string, limit, offset int) ([]LibraryItem, int, error) {
	where := []string{"u.readable > 0"}
	args := []any{}
	if q = strings.TrimSpace(q); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, "lower(u.title) LIKE $1")
	}
	switch filter {
	case "without":
		where = append(where, "p.id IS NULL")
	case "with":
		where = append(where, "p.id IS NOT NULL")
	case "excluded":
		where = append(where, "x.id IS NOT NULL")
	case "live":
		where = append(where, "r.id IS NOT NULL")
	}
	from := units + `
		SELECT u.kind, u.id, u.title, u.books, u.readable, x.id IS NOT NULL,
		       p.id, p.state, p.review, p.generated_at, r.id, r.state, r.stage, count(*) OVER ()
		FROM units u
		LEFT JOIN wiki_projects p ON (u.kind = 'series' AND p.collection_id = u.id) OR (u.kind = 'work' AND p.work_id = u.id)
		LEFT JOIN wiki_exclusions x ON (u.kind = 'series' AND x.collection_id = u.id) OR (u.kind = 'work' AND x.work_id = u.id)
		LEFT JOIN wiki_runs r ON r.state IN ('queued', 'running', 'paused')
		     AND ((u.kind = 'series' AND r.collection_id = u.id) OR (u.kind = 'work' AND r.work_id = u.id))
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY lower(u.title), u.kind, u.id`
	page := placeholders(len(args))
	args = append(args, limit, offset)
	rows, err := db.QueryContext(ctx, from+page, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []LibraryItem{}
	total := 0
	for rows.Next() {
		var it LibraryItem
		var pid, rid sql.NullInt64
		var pstate, preview, rstate, rstage sql.NullString
		var pgen sql.NullTime
		if err := rows.Scan(&it.Kind, &it.ID, &it.Title, &it.Books, &it.Readable, &it.Excluded,
			&pid, &pstate, &preview, &pgen, &rid, &rstate, &rstage, &total); err != nil {
			return nil, 0, err
		}
		if pid.Valid {
			it.Wiki = &WikiState{ProjectID: pid.Int64, State: pstate.String, Review: preview.String, GeneratedAt: pgen.Time}
		}
		if rid.Valid {
			it.Run = &RunState{ID: rid.Int64, State: rstate.String, Stage: rstage.String}
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// placeholders is the LIMIT and OFFSET after n arguments.
func placeholders(n int) string {
	return " LIMIT $" + itoa(n+1) + " OFFSET $" + itoa(n+2)
}

func itoa(n int) string { return strconv.Itoa(n) }

// eligible says whether the scope is a work in no series, or a series, with a book the generator can read.
func eligible(ctx context.Context, q queryer, s Scope) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, units+` SELECT EXISTS (SELECT 1 FROM units WHERE kind = $1 AND id = $2 AND readable > 0)`, s.Kind, s.ID).Scan(&ok)
	return ok, err
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Enqueue puts the generation of a work or series in the queue, or says the live run it already has. A direct request generates even
// what is in the exceptions (DEC-185).
func Enqueue(ctx context.Context, db *sql.DB, s Scope, origin, userID string) (runID int64, created bool, err error) {
	if !s.valid() {
		return 0, false, ErrNotEligible
	}
	p, err := LoadProvider(ctx, db)
	if err != nil {
		return 0, false, err
	}
	if p == nil {
		return 0, false, ErrNoProvider
	}
	ok, err := eligible(ctx, db, s)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, ErrNotEligible
	}
	work, collection := s.columns()
	err = db.QueryRowContext(ctx, `
		INSERT INTO wiki_runs (work_id, collection_id, origin, requested_by) VALUES ($1, $2, $3, NULLIF($4, '')::uuid)
		ON CONFLICT DO NOTHING RETURNING id`, work, collection, origin, userID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRowContext(ctx, `SELECT id FROM wiki_runs WHERE state IN ('queued', 'running', 'paused')
			AND (work_id = $1 OR collection_id = $2)`, work, collection).Scan(&runID)
		return runID, false, err
	}
	if err != nil {
		return 0, false, err
	}
	_ = audit.Record(ctx, db, userID, "wiki.generate", "wiki_run", itoa(int(runID)), map[string]any{"scope": s.Kind, "id": s.ID, "origin": origin})
	return runID, true, nil
}

// EnqueueAll puts in the queue every work and series that can have a wiki and has none, no live run, and is not in the exceptions
// ("Gerar para todas", DEC-185). It says how many it queued.
func EnqueueAll(ctx context.Context, db *sql.DB, userID string) (int, error) {
	p, err := LoadProvider(ctx, db)
	if err != nil {
		return 0, err
	}
	if p == nil {
		return 0, ErrNoProvider
	}
	res, err := db.ExecContext(ctx, units+`
		INSERT INTO wiki_runs (work_id, collection_id, origin, requested_by)
		SELECT CASE WHEN u.kind = 'work' THEN u.id::int END, CASE WHEN u.kind = 'series' THEN u.id END, 'batch', NULLIF($1, '')::uuid
		FROM units u
		WHERE u.readable > 0
		  AND NOT EXISTS (SELECT 1 FROM wiki_projects p WHERE (u.kind = 'series' AND p.collection_id = u.id) OR (u.kind = 'work' AND p.work_id = u.id))
		  AND NOT EXISTS (SELECT 1 FROM wiki_exclusions x WHERE (u.kind = 'series' AND x.collection_id = u.id) OR (u.kind = 'work' AND x.work_id = u.id))
		ORDER BY lower(u.title)
		ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	_ = audit.Record(ctx, db, userID, "wiki.generate_all", "wiki_run", "", map[string]any{"queued": n})
	return int(n), nil
}

// SetExcluded marks or unmarks a work or series as "não gerar wiki".
func SetExcluded(ctx context.Context, db *sql.DB, s Scope, excluded bool, userID string) error {
	if !s.valid() {
		return ErrNotEligible
	}
	work, collection := s.columns()
	var err error
	if excluded {
		_, err = db.ExecContext(ctx, `INSERT INTO wiki_exclusions (work_id, collection_id, created_by) VALUES ($1, $2, NULLIF($3, '')::uuid)
			ON CONFLICT DO NOTHING`, work, collection, userID)
	} else {
		_, err = db.ExecContext(ctx, `DELETE FROM wiki_exclusions WHERE work_id = $1 OR collection_id = $2`, work, collection)
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, db, userID, "wiki.exclusion", s.Kind, itoa(int(s.ID)), map[string]any{"excluded": excluded})
}

// Run is a generation as the administration follows it.
type Run struct {
	ID             int64           `json:"id"`
	Scope          Scope           `json:"scope"`
	Title          string          `json:"title"`
	Origin         string          `json:"origin"`
	State          string          `json:"state"`
	Stage          string          `json:"stage,omitempty"`
	Progress       json.RawMessage `json:"progress"`
	Place          string          `json:"place,omitempty"`
	Errors         json.RawMessage `json:"errors"`
	Resumes        int             `json:"resumes"`
	PauseRequested bool            `json:"pauseRequested"`
	CancelRequest  bool            `json:"cancelRequested"`
	Provider       string          `json:"provider,omitempty"`
	Model          string          `json:"model,omitempty"`
	HeartbeatAt    *time.Time      `json:"heartbeatAt"`
	CreatedAt      time.Time       `json:"createdAt"`
	StartedAt      *time.Time      `json:"startedAt"`
	FinishedAt     *time.Time      `json:"finishedAt"`
	Position       int             `json:"position,omitempty"` // in the queue, from 1
}

// Runs lists the live runs (running first, then the queue in its order, then the paused ones) and the last finished ones.
func Runs(ctx context.Context, db *sql.DB, finished int) ([]Run, error) {
	rows, err := db.QueryContext(ctx, `
		WITH picked AS (
			(SELECT * FROM wiki_runs WHERE state IN ('queued', 'running', 'paused'))
			UNION ALL
			(SELECT * FROM wiki_runs WHERE state IN ('done', 'failed', 'cancelled') ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT $1)
		)
		SELECT r.id, CASE WHEN r.work_id IS NULL THEN 'series' ELSE 'work' END, COALESCE(r.work_id::bigint, r.collection_id),
		       COALESCE(w.original_title, c.name, ''), r.origin, r.state, COALESCE(r.stage, ''), r.progress, COALESCE(r.place, ''), r.errors,
		       r.resumes, r.pause_requested, r.cancel_requested, COALESCE(r.provider, ''), COALESCE(r.model, ''), r.heartbeat_at,
		       r.created_at, r.started_at, r.finished_at
		FROM picked r
		LEFT JOIN works w ON w.id = r.work_id
		LEFT JOIN collections c ON c.id = r.collection_id
		ORDER BY CASE r.state WHEN 'running' THEN 0 WHEN 'queued' THEN 1 WHEN 'paused' THEN 2 ELSE 3 END,
		         CASE WHEN r.state IN ('queued', 'running', 'paused') THEN r.created_at END,
		         CASE WHEN r.state IN ('queued', 'running', 'paused') THEN r.id END, r.finished_at DESC NULLS LAST, r.id DESC`, finished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	position := 0
	for rows.Next() {
		var r Run
		var progress, errs []byte
		if err := rows.Scan(&r.ID, &r.Scope.Kind, &r.Scope.ID, &r.Title, &r.Origin, &r.State, &r.Stage, &progress, &r.Place, &errs,
			&r.Resumes, &r.PauseRequested, &r.CancelRequest, &r.Provider, &r.Model, &r.HeartbeatAt, &r.CreatedAt, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		r.Progress, r.Errors = progress, errs
		if r.State == "queued" {
			position++
			r.Position = position
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Control pauses, resumes or cancels a run. A queued run pauses or is cancelled at once; a running one is asked to, and the worker
// stops at its next step, keeping what it finished so that resuming does not do it again (RF-050).
func Control(ctx context.Context, db *sql.DB, runID int64, action, userID string) error {
	var q string
	switch action {
	case "pause":
		q = `UPDATE wiki_runs SET state = CASE WHEN state = 'queued' THEN 'paused' ELSE state END,
			pause_requested = (state = 'running'), updated_at = now()
			WHERE id = $1 AND state IN ('queued', 'running')`
	case "resume":
		q = `UPDATE wiki_runs SET state = 'queued', pause_requested = false, resumes = resumes + CASE WHEN started_at IS NULL THEN 0 ELSE 1 END,
			updated_at = now() WHERE id = $1 AND state = 'paused'`
	case "cancel":
		q = `UPDATE wiki_runs SET state = CASE WHEN state = 'running' THEN state ELSE 'cancelled' END,
			cancel_requested = (state = 'running'), finished_at = CASE WHEN state = 'running' THEN finished_at ELSE now() END, updated_at = now()
			WHERE id = $1 AND state IN ('queued', 'running', 'paused')`
	default:
		return ErrState
	}
	res, err := db.ExecContext(ctx, q, runID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM wiki_runs WHERE id = $1)`, runID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return ErrState
	}
	return audit.Record(ctx, db, userID, "wiki.run."+action, "wiki_run", itoa(int(runID)), nil)
}
