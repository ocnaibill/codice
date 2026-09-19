package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// FavoritesHandler stores the database connection
type FavoritesHandler struct {
	DB *sql.DB
}

// AddFavorite marks a work as favorite for the current user
func (h *FavoritesHandler) AddFavorite(w http.ResponseWriter, r *http.Request) {
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
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
	workID, ok := workIDParam(r)
	if !ok {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
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

	const seriesLabel = `COALESCE(NULLIF(%s.series, ''), %s.original_title::text)`
	label := func(alias string) string { return fmt.Sprintf(seriesLabel, alias, alias) }

	query := `
		SELECT
			w.id,
			w.original_title,
			` + authorLabel + `,
			COALESCE(wp.cover_url, '') as cover_url,
			` + label("w") + ` as series_label,
			(
				SELECT COUNT(*) FROM works w2
				WHERE w2.retired_at IS NULL AND ` + label("w2") + ` = ` + label("w") + `
			) as series_total,
			(
				SELECT COUNT(*) FROM works w3
				JOIN work_primary wp3 ON wp3.work_id = w3.id
				JOIN reading_progress rp3 ON rp3.file_id = wp3.file_id AND rp3.user_id = $1 AND rp3.completed_at IS NOT NULL
				WHERE w3.retired_at IS NULL AND ` + label("w3") + ` = ` + label("w") + `
			) as series_completed
		FROM favorites f
		JOIN works w ON w.id = f.work_id
		LEFT JOIN work_primary wp ON wp.work_id = w.id
		LEFT JOIN LATERAL (
			SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name) AS names
			FROM work_contributors c JOIN person p ON p.id = c.person_id
			WHERE c.work_id = w.id AND c.role = 'author'
		) au ON TRUE
		WHERE f.user_id = $1 AND w.retired_at IS NULL
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
