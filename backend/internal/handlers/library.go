package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Work representa a estrutura enviada ao frontend
type Work struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	CoverURL string `json:"coverUrl"`
}

// LibraryHandler guarda a conexão com o banco
type LibraryHandler struct {
	DB *sql.DB
}

// GetWorks busca as obras reais no PostgreSQL
func (h *LibraryHandler) GetWorks(w http.ResponseWriter, r *http.Request) {
	// A Query faz um JOIN seguro considerando que autor ou capa podem ser nulos
	query := `
		SELECT 
			w.id, 
			w.original_title, 
			COALESCE(p.name, 'Autor Desconhecido') as author, 
			COALESCE(e.cover_url, '') as cover_url
		FROM works w
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		ORDER BY w.id DESC
	`

	rows, err := h.DB.Query(query)
	if err != nil {
		http.Error(w, "Erro ao buscar obras", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var works []Work
	for rows.Next() {
		var work Work
		if err := rows.Scan(&work.ID, &work.Title, &work.Author, &work.CoverURL); err != nil {
			http.Error(w, "Erro ao ler os dados", http.StatusInternalServerError)
			return
		}
		
		// Fallback para capa caso o banco retorne vazio
		if work.CoverURL == "" {
			work.CoverURL = "https://via.placeholder.com/300x450/1f2937/d1d5db?text=Sem+Capa"
		}
		
		works = append(works, work)
	}

	// Evita retornar null (retorna array vazio)
	if works == nil {
		works = []Work{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(works)
}