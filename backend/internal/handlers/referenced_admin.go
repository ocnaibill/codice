package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// referencedPage is how many files a page has when the request does not say, and referencedMaxPage the most it
// may ask for; variables so a test can make them small.
var (
	referencedPage    = 50
	referencedMaxPage = 200
)

const maxTransferIDs = 500

// ReferencedFile is a file the library only points at: it lives in a directory the owner authorised and stays
// there until an administrator moves it into the managed storage (RF-011, DEC-034).
type ReferencedFile struct {
	FileID       int64  `json:"fileId"`
	WorkID       int    `json:"workId"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	Format       string `json:"format"`
	SizeBytes    *int64 `json:"sizeBytes"`
	RootID       *int   `json:"rootId"`
	Root         string `json:"root"`
	Path         string `json:"path"`
	State        string `json:"state"` // ok, missing (not where it was catalogued), conflict (it changed since)
	Availability string `json:"availability"`
	// Transfer is the latest transfer asked for this file that has not ended well: waiting, running or failed.
	Transfer *ReferencedTransfer `json:"transfer"`
}

// ReferencedTransfer is the state of a transfer job of one file.
type ReferencedTransfer struct {
	JobID     int64  `json:"jobId"`
	State     string `json:"state"`
	LastError string `json:"lastError,omitempty"`
}

// ListReferenced lists the referenced files of the library, by authorised directory, with their format, size and
// whether they are still where they were catalogued (owner and admin, #15). Filters: rootId, state (ok, missing,
// conflict) and q (title or path); paged. The summary counts every referenced file of the directory asked for.
func (h *StorageHandler) ListReferenced(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"l.mode = 'referenced'", "w.retired_at IS NULL"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if v := q.Get("rootId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			http.Error(w, "rootId is a number", http.StatusBadRequest)
			return
		}
		add("r.id = ?", id)
	}
	// What the summary counts: the directory, not the other filters.
	scope := strings.Join(where, " AND ")
	scopeArgs := append([]any{}, args...)
	if v := q.Get("state"); v != "" {
		if v != "ok" && v != "missing" && v != "conflict" {
			http.Error(w, "state is ok, missing or conflict", http.StatusBadRequest)
			return
		}
		add("l.state = ?", v)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		if len([]rune(v)) > 200 {
			http.Error(w, "the search is too long", http.StatusBadRequest)
			return
		}
		add(`(w.original_title ILIKE ? ESCAPE '\' OR l.path ILIKE ? ESCAPE '\')`, "%"+likeEscaper.Replace(v)+"%")
	}
	limit, offset := referencedPage, 0
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= referencedMaxPage {
		limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}

	from := `
		FROM storage_locations l
		JOIN files f ON f.id = l.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		LEFT JOIN storage_roots r ON r.path = l.root`
	var total int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT count(*) `+from+` WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		log.Println("Error counting referenced files:", err)
		http.Error(w, "Error listing referenced files", http.StatusInternalServerError)
		return
	}
	summary := map[string]int{"ok": 0, "missing": 0, "conflict": 0}
	srows, err := h.DB.QueryContext(r.Context(), `SELECT l.state, count(*) `+from+` WHERE `+scope+` GROUP BY l.state`, scopeArgs...)
	if err != nil {
		http.Error(w, "Error listing referenced files", http.StatusInternalServerError)
		return
	}
	for srows.Next() {
		var st string
		var n int
		if srows.Scan(&st, &n) == nil {
			summary[st] = n
		}
	}
	srows.Close()

	args = append(args, limit, offset)
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT f.id, w.id, w.original_title, COALESCE(a.name, ''), COALESCE(f.format, ''), f.size_bytes, r.id,
		       COALESCE(l.root, ''), l.path, l.state, f.availability,
		       t.id, t.state, COALESCE(t.last_error, '')`+from+`
		LEFT JOIN LATERAL (`+firstAuthorSQL+`) a ON TRUE
		LEFT JOIN LATERAL (
			SELECT j.id, j.state, j.last_error FROM jobs j
			WHERE j.type = 'transfer' AND j.state IN ('pending', 'running', 'failed') AND (j.payload->>'file_id')::bigint = f.id
			ORDER BY j.id DESC LIMIT 1
		) t ON TRUE
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY l.root, l.path, f.id
		LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		log.Println("Error listing referenced files:", err)
		http.Error(w, "Error listing referenced files", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []ReferencedFile{}
	for rows.Next() {
		var f ReferencedFile
		var size sql.NullInt64
		var rootID, jobID sql.NullInt64
		var jobState, jobError sql.NullString
		if err := rows.Scan(&f.FileID, &f.WorkID, &f.Title, &f.Author, &f.Format, &size, &rootID, &f.Root, &f.Path, &f.State,
			&f.Availability, &jobID, &jobState, &jobError); err != nil {
			log.Println("Error reading a referenced file:", err)
			http.Error(w, "Error listing referenced files", http.StatusInternalServerError)
			return
		}
		if size.Valid {
			f.SizeBytes = &size.Int64
		}
		if rootID.Valid {
			id := int(rootID.Int64)
			f.RootID = &id
		}
		if jobID.Valid {
			f.Transfer = &ReferencedTransfer{JobID: jobID.Int64, State: jobState.String, LastError: jobError.String}
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Error listing referenced files", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "summary": summary})
}

// TransferOutcome is how a transfer asked for one file stands, in words the screen can show.
type TransferOutcome struct {
	JobID  int64  `json:"jobId"`
	FileID int64  `json:"fileId"`
	Title  string `json:"title"`
	Format string `json:"format"`
	// Outcome: queued, running, moved (the original is gone), moved_original_kept (the copy stands and the
	// original waits for removal, see Origin and Reason), failed (see Error) or cancelled.
	Outcome    string     `json:"outcome"`
	Error      string     `json:"error,omitempty"`
	Origin     string     `json:"origin,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Transfers says how the transfers asked for are going, one per job (?ids=1,2,3, at most 500): waiting, running,
// done (and whether the original was removed or is kept waiting, with the reason) or failed (and why). The screen
// asks again until none is waiting.
func (h *StorageHandler) Transfers(w http.ResponseWriter, r *http.Request) {
	var ids []int64
	for _, part := range strings.Split(r.URL.Query().Get("ids"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "ids are numbers", http.StatusBadRequest)
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > maxTransferIDs {
		http.Error(w, "ids is required (1 to 500 job ids)", http.StatusBadRequest)
		return
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	idArray := "{" + strings.Join(parts, ",") + "}"
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT j.id, COALESCE((j.payload->>'file_id')::bigint, 0), COALESCE(w.original_title, ''), COALESCE(f.format, ''),
		       j.state, COALESCE(j.last_error, ''), j.finished_at,
		       COALESCE(c.path, ''), COALESCE(c.reason, ''), (l.mode = 'managed') AS moved
		FROM jobs j
		LEFT JOIN works w ON w.id = j.work_id
		LEFT JOIN files f ON f.id = (j.payload->>'file_id')::bigint
		LEFT JOIN storage_locations l ON l.file_id = f.id
		LEFT JOIN storage_cleanups c ON c.file_id = f.id
		WHERE j.type = 'transfer' AND j.id = ANY($1::bigint[])
		ORDER BY j.id`, idArray)
	if err != nil {
		log.Println("Error reading the transfers:", err)
		http.Error(w, "Error reading the transfers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []TransferOutcome{}
	for rows.Next() {
		var t TransferOutcome
		var state string
		var finished sql.NullTime
		var moved sql.NullBool
		if err := rows.Scan(&t.JobID, &t.FileID, &t.Title, &t.Format, &state, &t.Error, &finished, &t.Origin, &t.Reason, &moved); err != nil {
			log.Println("Error reading a transfer:", err)
			http.Error(w, "Error reading the transfers", http.StatusInternalServerError)
			return
		}
		if finished.Valid {
			t.FinishedAt = &finished.Time
		}
		switch state {
		case "pending":
			t.Outcome = "queued"
		case "running":
			t.Outcome = "running"
		case "succeeded":
			t.Outcome = "moved"
			if t.Origin != "" {
				t.Outcome = "moved_original_kept"
			}
		case "cancelled":
			t.Outcome = "cancelled"
		default:
			t.Outcome = "failed"
		}
		if t.Outcome != "failed" {
			t.Error = ""
		}
		if t.Outcome != "moved_original_kept" {
			t.Origin, t.Reason = "", ""
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Error reading the transfers", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
