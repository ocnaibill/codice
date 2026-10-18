package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// CollectionImagesHandler is how staff choose the cover and the banner of a series by hand (DEC-201), and go back to the automatic ones. The
// picture is cut by the page and checked here, as the cover of a work is (DEC-173). The files live with the other covers, under names that
// start with cover_manual_, which every backup keeps.
type CollectionImagesHandler struct {
	DB        *sql.DB
	CoversDir string
}

// imageKind says which picture of the collection a request is about.
type imageKind struct {
	column string // the column of collections
	spec   manualImage
	prefix string // what the names of its files start with, before the id
	action string // of the audit
}

var (
	collectionCover  = imageKind{"cover_url", coverImage, "cover_manual_c", "collection.cover"}
	collectionBanner = imageKind{"banner_url", bannerImage, "cover_manual_banner_c", "collection.banner"}
)

func (k imageKind) filePrefix(id int64) string { return fmt.Sprintf("%s%d_", k.prefix, id) }

// ownFile says whether a stored address is a file this kind of picture of this collection owns, so that nothing else is ever removed.
func (k imageKind) ownFile(id int64, url string) bool {
	return strings.HasPrefix(url, "/covers/"+k.filePrefix(id))
}

func (h *CollectionImagesHandler) drop(k imageKind, id int64, url string) {
	if h.CoversDir == "" || url == "" || !k.ownFile(id, url) {
		return
	}
	os.Remove(filepath.Join(h.CoversDir, filepath.Base(url)))
}

func (h *CollectionImagesHandler) set(w http.ResponseWriter, r *http.Request, k imageKind) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	if h.CoversDir == "" {
		http.Error(w, "Covers cannot be kept here", http.StatusInternalServerError)
		return
	}
	data, ok := readManualJPEG(w, r, k.spec)
	if !ok {
		return
	}
	if err := os.MkdirAll(h.CoversDir, 0o755); err != nil {
		log.Println("Error preparing the folder of the covers:", err)
		http.Error(w, "Error saving the picture", http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("%s%d.jpg", k.filePrefix(id), time.Now().UnixNano())
	dest := filepath.Join(h.CoversDir, name)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		log.Println("Error writing the picture of a collection:", err)
		http.Error(w, "Error saving the picture", http.StatusInternalServerError)
		return
	}
	url := "/covers/" + name

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		os.Remove(dest)
		http.Error(w, "Error saving the picture", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	_, retired, err := lockOfficial(r.Context(), tx, id)
	if errors.Is(err, errCollectionNotFound) {
		os.Remove(dest)
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	if err == nil && retired {
		os.Remove(dest)
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	var previous sql.NullString
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `SELECT `+k.column+` FROM collections WHERE id = $1`, id).Scan(&previous)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE collections SET `+k.column+` = $2 WHERE id = $1`, id, url)
	}
	if err == nil {
		err = audit.Record(r.Context(), tx, currentUserID(r), k.action+".set", "collection", strconv.FormatInt(id, 10), map[string]any{"bytes": len(data)})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		os.Remove(dest)
		log.Println("Error saving the picture of a collection:", err)
		http.Error(w, "Error saving the picture", http.StatusInternalServerError)
		return
	}
	h.drop(k, id, previous.String)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func (h *CollectionImagesHandler) reset(w http.ResponseWriter, r *http.Request, k imageKind) {
	id, ok := collectionIDParam(r, "id")
	if !ok {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error going back to the automatic picture", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	_, retired, err := lockOfficial(r.Context(), tx, id)
	if errors.Is(err, errCollectionNotFound) {
		http.Error(w, "Collection not found", http.StatusNotFound)
		return
	}
	if err == nil && retired {
		conflict(w, "A coleção está aposentada: restaure antes de mudar.", id)
		return
	}
	var previous sql.NullString
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `SELECT `+k.column+` FROM collections WHERE id = $1`, id).Scan(&previous)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE collections SET `+k.column+` = NULL WHERE id = $1`, id)
	}
	if err == nil {
		err = audit.Record(r.Context(), tx, currentUserID(r), k.action+".reset", "collection", strconv.FormatInt(id, 10), map[string]any{})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Println("Error going back to the automatic picture of a collection:", err)
		http.Error(w, "Error going back to the automatic picture", http.StatusInternalServerError)
		return
	}
	h.drop(k, id, previous.String)
	w.WriteHeader(http.StatusNoContent)
}

// SetCover answers PUT /collections/{id}/cover (staff): the cover of the series, a JPEG 2:3 in "image".
func (h *CollectionImagesHandler) SetCover(w http.ResponseWriter, r *http.Request) {
	h.set(w, r, collectionCover)
}

// ResetCover answers DELETE /collections/{id}/cover (staff): back to the cover of the first work that has one.
func (h *CollectionImagesHandler) ResetCover(w http.ResponseWriter, r *http.Request) {
	h.reset(w, r, collectionCover)
}

// SetBanner answers PUT /collections/{id}/banner (staff): the banner of the series, a JPEG 3:1 in "image".
func (h *CollectionImagesHandler) SetBanner(w http.ResponseWriter, r *http.Request) {
	h.set(w, r, collectionBanner)
}

// ResetBanner answers DELETE /collections/{id}/banner (staff): the series has no banner again.
func (h *CollectionImagesHandler) ResetBanner(w http.ResponseWriter, r *http.Request) {
	h.reset(w, r, collectionBanner)
}
