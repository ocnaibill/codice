package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
)

// OCRHandler lists the files whose pages have no text layer (RF-019) and how far reading them by OCR has got (#24), and
// lets the staff ask for the pages that failed to be tried again.
type OCRHandler struct {
	DB *sql.DB
}

// List returns every active work's PDF that has pages without text, with what became of them.
func (h *OCRHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT w.id, w.original_title, f.id, tl.page_count, tl.pages_without_text, oc.read, oc.failed, oc.languages, ocf.language, ocf.source,
		       (SELECT CASE j.state WHEN 'running' THEN 'reading' ELSE 'queued' END FROM jobs j
		        WHERE j.type = 'ocr' AND j.work_id = w.id AND j.state IN ('pending', 'running') ORDER BY j.id LIMIT 1)
		FROM text_layers tl
		JOIN files f ON f.id = tl.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		LEFT JOIN LATERAL (
			SELECT count(*) FILTER (WHERE p.state IN ('done', 'blank')) AS read, count(*) FILTER (WHERE p.state = 'failed') AS failed,
			       array_remove(array_agg(DISTINCT NULLIF(p.language, '')), NULL) AS languages
			FROM ocr_pages p WHERE p.file_id = f.id AND p.source_sha256 IS NOT DISTINCT FROM f.sha256
			  AND p.page + 1 = ANY (tl.pages_without_text)
		) oc ON TRUE
		LEFT JOIN ocr_files ocf ON ocf.file_id = f.id AND ocf.source_sha256 IS NOT DISTINCT FROM f.sha256
		WHERE tl.needs_ocr AND w.retired_at IS NULL
		ORDER BY w.original_title, f.id`)
	if err != nil {
		log.Println("Error listing files that need OCR:", err)
		http.Error(w, "Error listing files that need OCR", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type item struct {
		WorkID           int    `json:"workId"`
		Title            string `json:"title"`
		FileID           int64  `json:"fileId"`
		PageCount        int    `json:"pageCount"`
		PagesWithoutText []int  `json:"pagesWithoutText"`
		// Read counts the pages OCR has read (a blank one included) and Failed the ones it could not; State says whether
		// the work is waiting for the engine ("queued") or being read ("reading"); Languages are the ones it was read in.
		Read      int      `json:"read"`
		Failed    int      `json:"failed"`
		State     string   `json:"state,omitempty"`
		Languages []string `json:"languages"`
		// Language is the one the file is read in, and LanguageSource how it was chosen: "declared" by the edition,
		// "detected" from a few pages, "default" (the owner's, when it could not be told) or "manual" (somebody on the
		// staff). Both are empty until the file has been looked at.
		Language       string `json:"language,omitempty"`
		LanguageSource string `json:"languageSource,omitempty"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		var missing pq.Int64Array
		var languages pq.StringArray
		var state, language, source sql.NullString
		if err := rows.Scan(&it.WorkID, &it.Title, &it.FileID, &it.PageCount, &missing, &it.Read, &it.Failed, &languages, &language, &source, &state); err != nil {
			http.Error(w, "Error reading the list", http.StatusInternalServerError)
			return
		}
		it.State = state.String
		it.Language, it.LanguageSource = language.String, source.String
		it.Languages = append([]string{}, languages...)
		it.PagesWithoutText = []int{}
		for _, n := range missing {
			it.PagesWithoutText = append(it.PagesWithoutText, int(n))
		}
		out = append(out, it)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}

// Retry asks for the pages of a work that could not be read to be tried again (#24). A page that fails is kept as failed and
// is not tried again by itself, because what made it fail may not have changed: this is the person saying it may have.
// It goes ahead of the scheduled reading (DEC-069), and asking twice while one waits or runs is one request.
func (h *OCRHandler) Retry(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	var exists, enabled bool
	var failed int
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM works WHERE id = $1 AND retired_at IS NULL),
		       COALESCE((SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'ocr'), false),
		       (SELECT count(*) FROM ocr_pages p JOIN files f ON f.id = p.file_id JOIN editions e ON e.id = f.edition_id
		        WHERE e.work_id = $1 AND p.state = 'failed' AND p.source_sha256 IS NOT DISTINCT FROM f.sha256)`, workID).Scan(&exists, &enabled, &failed); err != nil {
		log.Println("Error reading the OCR state:", err)
		http.Error(w, "Error reading the OCR state", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if !enabled {
		http.Error(w, "O OCR está desligado.", http.StatusConflict)
		return
	}
	if failed == 0 {
		http.Error(w, "Nenhuma página desta obra falhou.", http.StatusConflict)
		return
	}
	var jobID sql.NullInt64
	err := h.DB.QueryRowContext(r.Context(), `
		INSERT INTO jobs (type, work_id, payload, priority, created_by)
		SELECT 'ocr', w.id, '{"retry_failed": true}', 5, NULLIF($2, '')::uuid
		FROM works w WHERE w.id = $1 AND w.retired_at IS NULL
		ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING
		RETURNING id`, workID, currentUserID(r)).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"queued": true, "alreadyQueued": true})
		return
	}
	if err != nil {
		log.Println("Error queueing OCR:", err)
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "ocr.retry", "work", strconv.Itoa(workID), map[string]any{"pages": failed})
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "jobId": jobID.Int64})
}

// SetLanguage records that a file is to be read in another language than the one chosen for it (#24): somebody on the staff
// saw that the automatic choice was wrong. The pages read so far are dropped, because they were read in the wrong
// language, and the file is read again from its first page, ahead of the scheduled reading (DEC-069). The text the
// library searches stays as it is until the new reading is done. It changes the language the engine reads in, not the
// language of the book's edition.
func (h *OCRHandler) SetLanguage(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if err != nil || fileID <= 0 {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "invalid language", http.StatusBadRequest)
		return
	}
	req.Language = strings.TrimSpace(req.Language)
	if !languageSet.MatchString(req.Language) {
		http.Error(w, "Idioma do OCR inválido: use os códigos do motor, como por ou por+eng.", http.StatusBadRequest)
		return
	}
	var workID int
	var sha, previous sql.NullString
	var enabled bool
	err = h.DB.QueryRowContext(r.Context(), `
		SELECT w.id, f.sha256, ocf.language,
		       COALESCE((SELECT (value->>'enabled')::boolean FROM settings WHERE key = 'ocr'), false)
		FROM text_layers tl
		JOIN files f ON f.id = tl.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
		LEFT JOIN ocr_files ocf ON ocf.file_id = f.id AND ocf.source_sha256 IS NOT DISTINCT FROM f.sha256
		WHERE f.id = $1 AND tl.needs_ocr AND w.retired_at IS NULL`, fileID).Scan(&workID, &sha, &previous, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading the file to set its OCR language:", err)
		http.Error(w, "Error reading the file", http.StatusInternalServerError)
		return
	}
	if !enabled {
		http.Error(w, "O OCR está desligado.", http.StatusConflict)
		return
	}
	worker, err := readOCRWorker(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the OCR settings", http.StatusInternalServerError)
		return
	}
	if missing := worker.missingLanguage(req.Language); missing != "" {
		http.Error(w, "O serviço de OCR não tem o idioma "+missing+".", http.StatusBadRequest)
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving the language", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	// The job first: while the work is being read, what is read goes on in the old language and would be mixed with the new.
	var jobID int64
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO jobs (type, work_id, payload, priority, created_by)
		VALUES ('ocr', $1, '{}', 5, NULLIF($2, '')::uuid)
		ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING
		RETURNING id`, workID, currentUserID(r)).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Esta obra está na fila ou sendo lida agora. Tente de novo quando terminar.", http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error queueing OCR:", err)
		http.Error(w, "Error queueing the job", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO ocr_files (file_id, source_sha256, language, source) VALUES ($1, $2, $3, 'manual')
		ON CONFLICT (file_id) DO UPDATE SET source_sha256 = EXCLUDED.source_sha256, language = EXCLUDED.language,
			source = 'manual', decided_at = now()`, fileID, sha, req.Language); err != nil {
		log.Println("Error saving the OCR language:", err)
		http.Error(w, "Error saving the language", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM ocr_pages WHERE file_id = $1`, fileID); err != nil {
		log.Println("Error dropping the pages read in the wrong language:", err)
		http.Error(w, "Error saving the language", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error saving the language", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "ocr.language", "file", strconv.FormatInt(fileID, 10),
		map[string]any{"work": workID, "language": req.Language, "previous": previous.String})
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "jobId": jobID, "language": req.Language})
}
