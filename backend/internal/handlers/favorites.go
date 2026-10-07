package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/ocnaibill/codice/backend/internal/people"
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
//
// The favorites of one series come as a single item (#184): Kind is "series", Title is the name of the series,
// WorkID and the cover are those of the favorite that comes first in the series, and FavoriteCount says how many of
// the series are favorites. A favorite with no series stays loose (Kind "work").
type FavoriteSeriesItem struct {
	Kind            string `json:"kind"`
	WorkID          int    `json:"workId"`
	Title           string `json:"title"`
	Author          string `json:"author"`
	CoverURL        string `json:"coverUrl"`
	SeriesLabel     string `json:"seriesLabel"`
	SeriesTotal     int    `json:"seriesTotal"`
	SeriesCompleted int    `json:"seriesCompleted"`
	FavoriteCount   int    `json:"favoriteCount"`
}

const (
	favoriteKindWork   = "work"
	favoriteKindSeries = "series"
)

// groupFavoriteSeries folds the favorites of one series into the item of the series. The rows come newest favorite
// first, and the item of a series takes the place of its newest favorite. Inside a series, the favorite with the
// lowest number (then the lowest id) stands for it.
func groupFavoriteSeries(rows []favoriteRow) []FavoriteSeriesItem {
	items := []FavoriteSeriesItem{}
	at := map[string]int{}
	lead := map[string]favoriteRow{}
	for _, row := range rows {
		if row.series == "" {
			row.item.Kind = favoriteKindWork
			row.item.FavoriteCount = 1
			items = append(items, row.item)
			continue
		}
		i, seen := at[row.series]
		if !seen {
			at[row.series] = len(items)
			lead[row.series] = row
			row.item.Kind = favoriteKindSeries
			row.item.Title = row.series
			row.item.FavoriteCount = 1
			items = append(items, row.item)
			continue
		}
		items[i].FavoriteCount++
		cur := lead[row.series]
		if row.index < cur.index || (row.index == cur.index && row.item.WorkID < cur.item.WorkID) {
			lead[row.series] = row
			items[i].WorkID = row.item.WorkID
			items[i].CoverURL = row.item.CoverURL
			items[i].Author = row.item.Author
		}
	}
	return items
}

// favoriteRow is a favorite as the query reads it, before the series are folded.
type favoriteRow struct {
	item   FavoriteSeriesItem
	series string
	index  float64
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
			` + authorLabelFor(people.OrderFor(r.Context(), h.DB, userID).Effective) + `,
			COALESCE(wp.cover_url, '') as cover_url,
			` + label("w") + ` as series_label,
			COALESCE(w.series, '') as series,
			COALESCE(w.series_index, 0) as series_index,
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
			SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name) AS names,
			       string_agg(CASE WHEN p.family_name IS NULL THEN p.name ELSE p.family_name || COALESCE(', ' || p.given_name, '') END,
			                  '; ' ORDER BY c.position, p.name) AS names_family
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

	var favorites []favoriteRow
	for rows.Next() {
		var row favoriteRow
		item := &row.item
		if err := rows.Scan(&item.WorkID, &item.Title, &item.Author, &item.CoverURL, &item.SeriesLabel, &row.series, &row.index, &item.SeriesTotal, &item.SeriesCompleted); err != nil {
			log.Println("Error scanning favorite:", err)
			continue
		}
		if item.CoverURL == "" {
			item.CoverURL = "/covers/placeholder.svg"
		}
		favorites = append(favorites, row)
	}
	items := groupFavoriteSeries(favorites)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}
