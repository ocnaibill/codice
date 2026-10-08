package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/ocnaibill/codice/backend/internal/people"
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

// FavoriteItem is one card of the "favorites" widget of the home (#184, #208): a collection the person favorited (an official
// one, or one of their lists), or a work they favorited that is not already under a collection they favorited. A work in a
// collection is not shown apart from it, but it is shown when the collection is not a favorite: a favorite is never hidden.
type FavoriteItem struct {
	// Kind is "collection" or "work".
	Kind string `json:"kind"`
	// WorkID is the work of a "work" card.
	WorkID int `json:"workId,omitempty"`
	// CollectionID is the collection of a "collection" card, and CollectionKind says whether it is "official" or "personal".
	CollectionID   int64  `json:"collectionId,omitempty"`
	CollectionKind string `json:"collectionKind,omitempty"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	CoverURL       string `json:"coverUrl"`
	// WorkCount and CompletedCount are of a collection: how many works are in it, and how many the person finished.
	WorkCount      int `json:"workCount"`
	CompletedCount int `json:"completedCount"`
	// Completed is of a work: the person finished it.
	Completed bool `json:"completed"`

	at time.Time
}

// AddCollectionFavorite answers POST /collections/{id}/favorite: the collection is a favorite of the caller. It must be one they can
// see (an official one that is not retired, or one of their own lists); any other is not found.
func (h *FavoritesHandler) AddCollectionFavorite(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	res, err := h.DB.Exec(`
		INSERT INTO favorite_collections (user_id, collection_id)
		SELECT $1::uuid, c.id FROM collections c
		WHERE c.id = $2 AND c.retired_at IS NULL AND (c.kind = 'official' OR c.owner_id = $1::uuid)
		ON CONFLICT DO NOTHING`, currentUserID(r), id)
	if err != nil {
		log.Println("Error favoriting a collection:", err)
		http.Error(w, "Error adding favorite", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Either it is a favorite already, or it is not theirs to see.
		var seen bool
		if err := h.DB.QueryRow(`
			SELECT EXISTS (SELECT 1 FROM collections c WHERE c.id = $2 AND c.retired_at IS NULL AND (c.kind = 'official' OR c.owner_id = $1::uuid))`,
			currentUserID(r), id).Scan(&seen); err != nil || !seen {
			http.Error(w, "Collection not found", http.StatusNotFound)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// RemoveCollectionFavorite answers DELETE /collections/{id}/favorite: the collection is not a favorite of the caller any more.
// It is always theirs to take away, so it is not an error when it was not one.
func (h *FavoritesHandler) RemoveCollectionFavorite(w http.ResponseWriter, r *http.Request) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM favorite_collections WHERE user_id = $1::uuid AND collection_id = $2`, currentUserID(r), id); err != nil {
		log.Println("Error removing the favorite of a collection:", err)
		http.Error(w, "Error removing favorite", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// GetFavorites returns what the person favorited, newest first: their favorite collections as one card each, and the favorite works
// that are not under one of those. Used by the "Seus favoritos" widget.
func (h *FavoritesHandler) GetFavorites(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	items := []FavoriteItem{}

	rows, err := h.DB.Query(`
		SELECT c.id, c.kind, c.name, fc.created_at, COALESCE(s.works, 0), COALESCE(s.completed, 0), COALESCE(s.cover, '')
		FROM favorite_collections fc
		JOIN collections c ON c.id = fc.collection_id`+collectionStats+`
		WHERE fc.user_id = $1::uuid AND c.retired_at IS NULL AND (c.kind = 'official' OR c.owner_id = $1::uuid)`, userID)
	if err != nil {
		log.Println("Error fetching favorite collections:", err)
		http.Error(w, "Error fetching favorites", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		item := FavoriteItem{Kind: "collection"}
		if err := rows.Scan(&item.CollectionID, &item.CollectionKind, &item.Title, &item.at, &item.WorkCount, &item.CompletedCount, &item.CoverURL); err != nil {
			log.Println("Error scanning a favorite collection:", err)
			continue
		}
		if item.CoverURL == "" {
			item.CoverURL = "/covers/placeholder.svg"
		}
		items = append(items, item)
	}
	rows.Close()

	rows, err = h.DB.Query(`
		SELECT w.id, w.original_title, `+authorLabelFor(people.OrderFor(r.Context(), h.DB, userID).Effective)+`,
		       COALESCE(wp.cover_url, ''), f.created_at, COALESCE(rp.completed_at IS NOT NULL, FALSE)
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
		LEFT JOIN reading_progress rp ON rp.file_id = wp.file_id AND rp.user_id = $1::uuid AND rp.completed_at IS NOT NULL
		WHERE f.user_id = $1::uuid AND w.retired_at IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM collection_works cw
			JOIN favorite_collections fc ON fc.collection_id = cw.collection_id AND fc.user_id = $1::uuid
			JOIN collections c ON c.id = cw.collection_id AND c.retired_at IS NULL
			WHERE cw.work_id = w.id AND cw.official)`, userID)
	if err != nil {
		log.Println("Error fetching favorite works:", err)
		http.Error(w, "Error fetching favorites", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		item := FavoriteItem{Kind: "work"}
		if err := rows.Scan(&item.WorkID, &item.Title, &item.Author, &item.CoverURL, &item.at, &item.Completed); err != nil {
			log.Println("Error scanning a favorite work:", err)
			continue
		}
		if item.CoverURL == "" {
			item.CoverURL = "/covers/placeholder.svg"
		}
		items = append(items, item)
	}
	rows.Close()

	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}
