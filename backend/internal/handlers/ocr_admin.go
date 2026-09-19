package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/lib/pq"
)

// OCRHandler lists the files whose pages have no text layer (RF-019). It only
// reports; running OCR on them is a later step.
type OCRHandler struct {
	DB *sql.DB
}

// List returns every active work's PDF that has pages without text.
func (h *OCRHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT w.id, w.original_title, f.id, tl.page_count, tl.pages_without_text
		FROM text_layers tl
		JOIN files f ON f.id = tl.file_id
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id
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
	}
	out := []item{}
	for rows.Next() {
		var it item
		var missing pq.Int64Array
		if err := rows.Scan(&it.WorkID, &it.Title, &it.FileID, &it.PageCount, &missing); err != nil {
			http.Error(w, "Error reading the list", http.StatusInternalServerError)
			return
		}
		it.PagesWithoutText = []int{}
		for _, n := range missing {
			it.PagesWithoutText = append(it.PagesWithoutText, int(n))
		}
		out = append(out, it)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": out})
}
