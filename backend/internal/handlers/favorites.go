package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// FavoritesHandler stores the database connection
type FavoritesHandler struct {
	DB *sql.DB
}

// AddFavorite marks a work as favorite for the current user
func (h *FavoritesHandler) AddFavorite(w http.ResponseWriter, r *http.Request) {
	workID := chi.URLParam(r, "id")
	userID := currentUserID(r)

	_, err := h.DB.Exec(
		`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, workID,
	)
	if err != nil {
		log.Println("Error adding favorite:", err)
		http.Error(w, "Error adding favorite", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// RemoveFavorite unmarks a work as favorite for the current user
func (h *FavoritesHandler) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	workID := chi.URLParam(r, "id")
	userID := currentUserID(r)

	_, err := h.DB.Exec(`DELETE FROM favorites WHERE user_id = $1 AND work_id = $2`, userID, workID)
	if err != nil {
		log.Println("Error removing favorite:", err)
		http.Error(w, "Error removing favorite", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// FavoriteSeriesItem is one row of the "favorite series" dashboard widget:
// a favorited work plus how many works share its series and how many of
// those the current user has finished. Works without a series are treated
// as a series of one (so "read 1 of 1" / "read 0 of 1").
type FavoriteSeriesItem struct {
	WorkID          int    `json:"workId"`
	Title           string `json:"title"`
	Author          string `json:"author"`
	CoverURL        string `json:"coverUrl"`
	SeriesLabel     string `json:"seriesLabel"`
	SeriesTotal     int    `json:"seriesTotal"`
	SeriesCompleted int    `json:"seriesCompleted"`
}

// GetFavorites returns the current user's favorited works enriched with
// series-wide completion counts, used by the "Suas séries favoritas" widget.
func (h *FavoritesHandler) GetFavorites(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)

	query := `
		SELECT
			w.id,
			w.original_title,
			COALESCE(p.name, 'Unknown Author') as author,
			COALESCE(e.cover_url, '') as cover_url,
			COALESCE(NULLIF(w.series, ''), w.original_title::text) as series_label,
			(
				SELECT COUNT(*) FROM works w2
				WHERE COALESCE(NULLIF(w2.series, ''), w2.original_title::text) = COALESCE(NULLIF(w.series, ''), w.original_title::text)
			) as series_total,
			(
				SELECT COUNT(*) FROM works w3
				JOIN user_progress up3 ON up3.work_id = w3.id AND up3.user_id = $1 AND up3.completed_at IS NOT NULL
				WHERE COALESCE(NULLIF(w3.series, ''), w3.original_title::text) = COALESCE(NULLIF(w.series, ''), w.original_title::text)
			) as series_completed
		FROM favorites f
		JOIN works w ON w.id = f.work_id
		LEFT JOIN person p ON w.author_id = p.id
		LEFT JOIN editions e ON w.id = e.work_id
		WHERE f.user_id = $1
		ORDER BY f.created_at DESC
	`

	rows, err := h.DB.Query(query, userID)
	if err != nil {
		log.Println("Error fetching favorites:", err)
		http.Error(w, "Error fetching favorites", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := []FavoriteSeriesItem{}
	for rows.Next() {
		var item FavoriteSeriesItem
		if err := rows.Scan(&item.WorkID, &item.Title, &item.Author, &item.CoverURL, &item.SeriesLabel, &item.SeriesTotal, &item.SeriesCompleted); err != nil {
			log.Println("Error scanning favorite:", err)
			continue
		}
		if item.CoverURL == "" {
			item.CoverURL = "/covers/placeholder.svg"
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}
