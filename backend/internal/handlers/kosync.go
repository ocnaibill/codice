package handlers

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/locator"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

// KosyncHandler speaks the protocol KOReader syncs reading progress with (DEC-190): the same four calls as the server it ships with, so that a
// device only has to be told where the server is. A book is known by the hash the worker keeps of its file, and the place is the chapter: a
// device says where it is with an xpointer, which is read down to the item of the book it is in.
type KosyncHandler struct {
	DB          *sql.DB
	Sessions    *sessions.Store
	StorageRoot string
	// OnFailure is told of every refused attempt, with the name that was given: the log of entries shows it like any other app token.
	OnFailure func(r *http.Request, username string)

	spines spineCache
}

const (
	kosyncDevicePrefix = "KOReader · " // how a device was named in the progress before the person named it (still read, no longer written)
	kosyncWebDevice    = "Códice"
	kosyncWebDeviceID  = "codice-web"
)

// kosyncAccessKey carries the access a device logged in with: its name ("Kindle") is how the web says where they stopped, and the access stands
// for the device when it is asked what it has seen.
type kosyncAccessKey struct{}

var (
	kosyncDocument = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	docFragment    = regexp.MustCompile(`DocFragment\[(\d+)\]`)
)

func kosyncJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Auth reads the credentials KOReader sends (x-auth-user and x-auth-key, the MD5 of the password) and lets the person of a koreader token in.
func (h *KosyncHandler) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, key := r.Header.Get("x-auth-user"), r.Header.Get("x-auth-key")
		access, err := h.Sessions.VerifyKosync(r.Context(), user, key)
		if errors.Is(err, middleware.ErrInvalidCredentials) {
			if h.OnFailure != nil && user != "" {
				h.OnFailure(r, user)
			}
			kosyncJSON(w, http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
			return
		}
		if err != nil {
			log.Println("Error checking KOSync credentials:", err)
			kosyncJSON(w, http.StatusInternalServerError, map[string]string{"message": "Error"})
			return
		}
		ctx := context.WithValue(r.Context(), middleware.UserIDKey, access.UserID)
		ctx = context.WithValue(ctx, middleware.UserRoleKey, access.Role)
		ctx = context.WithValue(ctx, kosyncAccessKey{}, access)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Create is the registration of a KOReader account. Accounts are made in the Códice, so this refuses (402 is "already registered" to the
// device, which shows the message).
func (h *KosyncHandler) Create(w http.ResponseWriter, r *http.Request) {
	kosyncJSON(w, http.StatusPaymentRequired, map[string]string{"message": "Accounts are made in the Códice (Conta, Aplicativos): use the user and the short password it shows there."})
}

// Authorize answers that the credentials of the request are good (Auth has checked them).
func (h *KosyncHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	kosyncJSON(w, http.StatusOK, map[string]string{"authorized": "OK"})
}

type kosyncFile struct {
	ID       int64
	Format   string
	FullPath string
}

// fileOf finds the file KOReader means by the hash it sends; false when the library has no such file.
func (h *KosyncHandler) fileOf(r *http.Request, document string) (kosyncFile, bool, error) {
	var f kosyncFile
	var mode, root, rel sql.NullString
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT f.id, COALESCE(f.format, ''), l.mode, l.root, l.path
		FROM files f
		JOIN editions e ON e.id = f.edition_id
		JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
		LEFT JOIN LATERAL (SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1) l ON TRUE
		WHERE f.koreader_md5 = $1 AND f.availability = 'available'
		ORDER BY f.id LIMIT 1`, strings.ToLower(document)).Scan(&f.ID, &f.Format, &mode, &root, &rel)
	if errors.Is(err, sql.ErrNoRows) {
		return f, false, nil
	}
	if err != nil {
		return f, false, err
	}
	f.Format = strings.ToLower(f.Format)
	if rel.Valid {
		if full, ok := storage.AbsPath(h.StorageRoot, mode.String, root.String, rel.String); ok {
			f.FullPath = full
		}
	}
	return f, true, nil
}

type kosyncPut struct {
	Document   string  `json:"document"`
	Progress   string  `json:"progress"`
	Percentage float64 `json:"percentage"`
	Device     string  `json:"device"`
	DeviceID   string  `json:"device_id"`
}

// PutProgress records where a device is. The last write wins (DEC-190): a device does not send the revision it read, and the progress of the
// book on the web is protected by the revision that the web sends.
func (h *KosyncHandler) PutProgress(w http.ResponseWriter, r *http.Request) {
	var req kosyncPut
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil || !kosyncDocument.MatchString(req.Document) ||
		math.IsNaN(req.Percentage) || math.IsInf(req.Percentage, 0) || len(req.Progress) > 2000 {
		kosyncJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request"})
		return
	}
	now := time.Now().Unix()
	file, found, err := h.fileOf(r, req.Document)
	if err != nil {
		log.Println("Error finding the file of a KOSync progress:", err)
		kosyncJSON(w, http.StatusInternalServerError, map[string]string{"message": "Error"})
		return
	}
	// A book the library does not have (or in a format that has no place to keep) is accepted and not kept: the device does not need to know.
	loc, ok := h.locatorOf(file, req.Progress)
	if !found || !ok {
		kosyncJSON(w, http.StatusAccepted, map[string]any{"document": req.Document, "timestamp": now})
		return
	}
	canonical, err := locator.Validate(file.Format, loc)
	if err != nil {
		kosyncJSON(w, http.StatusAccepted, map[string]any{"document": req.Document, "timestamp": now})
		return
	}
	percent := math.Max(0, math.Min(100, req.Percentage*100))
	// the device is named as the person named the access ("Kindle"), which is what the reader of the web says; what KOReader calls it (its
	// model) is kept apart, because it is what it is told back
	access, _ := r.Context().Value(kosyncAccessKey{}).(sessions.KosyncAccess)
	device := strings.TrimSpace(access.Name)
	if device == "" {
		device = strings.TrimSpace(req.Device)
	}
	if len(device) > 100 {
		device = device[:100]
	}
	model := strings.TrimSpace(req.Device)
	if len(model) > 100 {
		model = model[:100]
	}
	userID := currentUserID(r)
	position := locator.Position(canonical)

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		log.Println("Error saving a KOSync progress:", err)
		kosyncJSON(w, http.StatusInternalServerError, map[string]string{"message": "Error"})
		return
	}
	defer tx.Rollback()
	fail := func(err error) {
		log.Println("Error saving a KOSync progress:", err)
		kosyncJSON(w, http.StatusInternalServerError, map[string]string{"message": "Error"})
	}

	// A send from a device that did not see the newest save of someone else (the web, another device) is not written over it (DEC-191): the
	// device may have read without a network and says it only now, and what it sends carries no time. It is kept apart, for the person to
	// choose. A device that wrote the newest save itself, or has seen it, is the latest.
	var revision int64
	var lastDevice sql.NullString
	var hasPlace bool
	err = tx.QueryRowContext(r.Context(), `
		SELECT revision, kosync_device_id, locator IS NOT NULL FROM reading_progress WHERE user_id = $1 AND file_id = $2 FOR UPDATE`,
		userID, file.ID).Scan(&revision, &lastDevice, &hasPlace)
	exists := !errors.Is(err, sql.ErrNoRows)
	if err != nil && exists {
		fail(err)
		return
	}
	if exists && hasPlace && !(lastDevice.Valid && lastDevice.String == req.DeviceID) {
		var seen int64
		if err := tx.QueryRowContext(r.Context(), `SELECT revision FROM kosync_seen WHERE user_id = $1 AND file_id = $2 AND token_id = $3`,
			userID, file.ID, access.TokenID).Scan(&seen); err != nil && !errors.Is(err, sql.ErrNoRows) {
			fail(err)
			return
		}
		if revision > seen {
			if _, err := tx.ExecContext(r.Context(), `
				INSERT INTO kosync_pending (user_id, file_id, device, position, locator, locator_version, percent_complete,
				                            kosync_progress, kosync_device_id, kosync_device, created_at)
				VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, now())
				ON CONFLICT (user_id, file_id) DO UPDATE SET device = EXCLUDED.device, position = EXCLUDED.position, locator = EXCLUDED.locator,
					locator_version = EXCLUDED.locator_version, percent_complete = EXCLUDED.percent_complete,
					kosync_progress = EXCLUDED.kosync_progress, kosync_device_id = EXCLUDED.kosync_device_id,
					kosync_device = EXCLUDED.kosync_device, created_at = now()`,
				userID, file.ID, device, position, string(canonical), locator.Version, percent, req.Progress, req.DeviceID, model); err != nil {
				fail(err)
				return
			}
			if err := tx.Commit(); err != nil {
				fail(err)
				return
			}
			kosyncJSON(w, http.StatusOK, map[string]any{"document": req.Document, "timestamp": now})
			return
		}
	}

	// A file counts as finished at the same point as on the web (DEC-077), and that is a fact the statistics use.
	finished := percent >= kosyncFinishedAt
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO reading_progress AS rp (user_id, file_id, position, locator, locator_version, percent_complete, completed_at, device, updated_at,
		                                    kosync_progress, kosync_device_id, kosync_device)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, CASE WHEN $11::boolean THEN now() END, $7, now(), $8, $9, $10)
		ON CONFLICT (user_id, file_id) DO UPDATE SET
			position = EXCLUDED.position, locator = EXCLUDED.locator, locator_version = EXCLUDED.locator_version,
			-- the place in words (the chapter, the unit) was of the old locator: a device that does not say them leaves none
			chapter = NULL, unit_index = NULL, unit_total = NULL,
			percent_complete = EXCLUDED.percent_complete, device = EXCLUDED.device, client_updated_at = NULL,
			-- finished stays finished: the first date is kept
			completed_at = CASE WHEN $11::boolean THEN COALESCE(rp.completed_at, now()) ELSE rp.completed_at END,
			kosync_progress = EXCLUDED.kosync_progress, kosync_device_id = EXCLUDED.kosync_device_id, kosync_device = EXCLUDED.kosync_device,
			revision = rp.revision + 1, updated_at = now()
		RETURNING revision`,
		userID, file.ID, position, string(canonical), locator.Version, percent, device, req.Progress, req.DeviceID, model, finished).Scan(&revision)
	if err != nil {
		fail(err)
		return
	}
	// what was kept apart is older than this, and the device has seen what it wrote
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM kosync_pending WHERE user_id = $1 AND file_id = $2`, userID, file.ID); err != nil {
		fail(err)
		return
	}
	if err := h.markSeen(r, tx, userID, file.ID, access.TokenID, revision); err != nil {
		fail(err)
		return
	}
	if !finished {
		// reading on takes the mark "the whole work is finished" off, as it does on the web
		if _, err := tx.ExecContext(r.Context(), `
			DELETE FROM work_reading_state
			WHERE user_id = $1 AND work_id = (SELECT e.work_id FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = $2)`, userID, file.ID); err != nil {
			fail(err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(err)
		return
	}
	kosyncJSON(w, http.StatusOK, map[string]any{"document": req.Document, "timestamp": now})
}

// kosyncFinishedAt is the percentage at which a file counts as finished: the same as on the web (DEC-077).
const kosyncFinishedAt = 95.0

// execer is what a statement can be run on: a transaction or the database.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// markSeen records the revision of a book that an access (a device) has seen: what it wrote, or what it was told.
func (h *KosyncHandler) markSeen(r *http.Request, db execer, userID string, fileID int64, tokenID string, revision int64) error {
	if tokenID == "" {
		return nil
	}
	_, err := db.ExecContext(r.Context(), `
		INSERT INTO kosync_seen (user_id, file_id, token_id, revision) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, file_id, token_id) DO UPDATE SET revision = EXCLUDED.revision, seen_at = now()`, userID, fileID, tokenID, revision)
	return err
}

// GetProgress says where the person is in a book. What a device said last is given back as it said it (the device then knows it is its own
// and does not ask the person to move); where the person read on after it (the web), the place is made from what the Códice has.
func (h *KosyncHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	document := chi.URLParam(r, "document")
	if !kosyncDocument.MatchString(document) {
		kosyncJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request"})
		return
	}
	file, found, err := h.fileOf(r, document)
	if err != nil {
		log.Println("Error finding the file of a KOSync progress:", err)
		kosyncJSON(w, http.StatusInternalServerError, map[string]string{"message": "Error"})
		return
	}
	if !found {
		kosyncJSON(w, http.StatusOK, map[string]any{})
		return
	}
	var loc []byte
	var percent float64
	var device, kProgress, kDevice, kModel sql.NullString
	var updated time.Time
	var revision int64
	err = h.DB.QueryRowContext(r.Context(), `
		SELECT locator, percent_complete, device, kosync_progress, kosync_device_id, kosync_device, updated_at, revision
		FROM reading_progress WHERE user_id = $1 AND file_id = $2`, currentUserID(r), file.ID).
		Scan(&loc, &percent, &device, &kProgress, &kDevice, &kModel, &updated, &revision)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(loc) == 0) {
		kosyncJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if err != nil {
		log.Println("Error reading a KOSync progress:", err)
		kosyncJSON(w, http.StatusInternalServerError, map[string]string{"message": "Error"})
		return
	}
	// the device is now told what the save is: it has seen it, and a send of its own after this one is the latest
	access, _ := r.Context().Value(kosyncAccessKey{}).(sessions.KosyncAccess)
	if err := h.markSeen(r, h.DB, currentUserID(r), file.ID, access.TokenID, revision); err != nil {
		log.Println("Error recording what a KOSync device has seen:", err)
	}
	out := map[string]any{"document": strings.ToLower(document), "percentage": percent / 100, "timestamp": updated.Unix()}
	if kProgress.Valid && kProgress.String != "" && kDevice.Valid && kDevice.String != "" {
		model := kModel.String
		if model == "" { // written before the model was kept apart
			model = strings.TrimPrefix(device.String, kosyncDevicePrefix)
		}
		out["progress"] = kProgress.String
		out["device"] = model
		out["device_id"] = kDevice.String
	} else {
		place, ok := h.placeOf(file, loc)
		if !ok {
			kosyncJSON(w, http.StatusOK, map[string]any{})
			return
		}
		out["progress"] = place
		out["device"] = kosyncWebDevice
		out["device_id"] = kosyncWebDeviceID
	}
	kosyncJSON(w, http.StatusOK, out)
}

// locatorOf turns the place a device sent into a locator of the Códice: the chapter of an EPUB (the DocFragment of the xpointer is the item
// of the reading order it is in, at its start), the page of a PDF or a comic. False for what has no place to keep.
func (h *KosyncHandler) locatorOf(file kosyncFile, progress string) ([]byte, bool) {
	switch file.Format {
	case "epub":
		m := docFragment.FindStringSubmatch(progress)
		if m == nil || file.FullPath == "" {
			return nil, false
		}
		n, _ := strconv.Atoi(m[1])
		spine, err := h.spines.of(file.FullPath)
		if err != nil || n < 1 || n > len(spine) {
			return nil, false
		}
		loc := map[string]any{"type": "epub", "href": spine[n-1].Href, "progression": 0}
		// the paragraph, when the path leads to it: a CFI the reader opens at, and how far into the chapter it is. Otherwise the chapter's start.
		if _, tail, ok := strings.Cut(progress, m[0]+"/"); ok {
			if html, total, err := readChapter(file.FullPath, spine[n-1]); err == nil {
				if cfi, progression, ok := cfiOfXPointer(html, total, n, tail); ok {
					loc["cfi"] = cfi
					loc["progression"] = math.Round(progression*1000) / 1000
				}
			}
		}
		raw, _ := json.Marshal(loc)
		return raw, true
	case "pdf":
		page, err := strconv.Atoi(strings.TrimSpace(progress))
		if err != nil || page < 1 {
			return nil, false
		}
		raw, _ := json.Marshal(map[string]any{"type": "pdf", "page": page - 1})
		return raw, true
	case "cbz", "cbr":
		page, err := strconv.Atoi(strings.TrimSpace(progress))
		if err != nil || page < 1 {
			return nil, false
		}
		raw, _ := json.Marshal(map[string]any{"type": "image", "index": page - 1})
		return raw, true
	}
	return nil, false
}

// xpointerOfItem is where a device is told to go for the start of the item N of the reading order of an EPUB.
func xpointerOfItem(n int) string { return "/body/DocFragment[" + strconv.Itoa(n) + "]" }

// placeOf is the place a device is told to go to, from a locator of the Códice.
func (h *KosyncHandler) placeOf(file kosyncFile, raw []byte) (string, bool) {
	var l struct {
		Href  string `json:"href"`
		CFI   string `json:"cfi"`
		Page  *int   `json:"page"`
		Index *int   `json:"index"`
	}
	if json.Unmarshal(raw, &l) != nil {
		return "", false
	}
	switch file.Format {
	case "epub":
		if file.FullPath == "" {
			return "", false
		}
		spine, err := h.spines.of(file.FullPath)
		if err != nil {
			return "", false
		}
		// the paragraph, when the reader said it with a CFI that leads to an element of the chapter
		if n, inside, ok := itemOfCFI(l.CFI); ok && n >= 1 && n <= len(spine) {
			if html, _, err := readChapter(file.FullPath, spine[n-1]); err == nil {
				if place, ok := xpointerOfCFI(html, n, inside); ok {
					return place, true
				}
			}
			return xpointerOfItem(n), true
		}
		if l.Href == "" {
			return "", false
		}
		clean := strings.SplitN(l.Href, "#", 2)[0]
		for i, item := range spine {
			if item.Href == clean || strings.HasSuffix(item.Href, "/"+clean) || strings.HasSuffix(clean, "/"+item.Href) {
				return xpointerOfItem(i + 1), true
			}
		}
	case "pdf":
		if l.Page != nil {
			return strconv.Itoa(*l.Page + 1), true
		}
	case "cbz", "cbr":
		if l.Index != nil {
			return strconv.Itoa(*l.Index + 1), true
		}
	}
	return "", false
}

// spineCache keeps the reading order of the EPUBs asked about, by file and by when the file changed: a device asks every few seconds while
// someone reads.
type spineCache struct {
	mu    sync.Mutex
	items map[string]cachedSpine
}

type cachedSpine struct {
	modified time.Time
	items    []spineItem
}

func (c *spineCache) of(full string) ([]spineItem, error) {
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if hit, ok := c.items[full]; ok && hit.modified.Equal(info.ModTime()) {
		c.mu.Unlock()
		return hit.items, nil
	}
	c.mu.Unlock()
	items, err := epubSpine(full)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.items == nil || len(c.items) > 256 {
		c.items = map[string]cachedSpine{}
	}
	c.items[full] = cachedSpine{modified: info.ModTime(), items: items}
	c.mu.Unlock()
	return items, nil
}

// epubSpine is the reading order of an EPUB: each item with its href as the OPF writes it (relative to the OPF, which is how the reader names
// the chapters), in order. Every item counts, the ones outside the reading flow too, as the device counts them.
func epubSpine(full string) ([]spineItem, error) {
	zr, err := zip.OpenReader(full)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	read := func(name string) ([]byte, error) {
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 4<<20))
			}
		}
		return nil, errors.New("not in the archive: " + name)
	}
	container, err := read("META-INF/container.xml")
	if err != nil {
		return nil, err
	}
	var c struct {
		Rootfiles []struct {
			Path string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(container, &c); err != nil || len(c.Rootfiles) == 0 {
		return nil, errors.New("no package document")
	}
	opfPath := path.Clean(c.Rootfiles[0].Path)
	opfData, err := read(opfPath)
	if err != nil {
		return nil, err
	}
	var opf struct {
		Items []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"manifest>item"`
		Refs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"spine>itemref"`
	}
	if err := xml.Unmarshal(opfData, &opf); err != nil {
		return nil, err
	}
	byID := map[string]string{}
	for _, it := range opf.Items {
		byID[it.ID] = it.Href
	}
	var out []spineItem
	for _, ref := range opf.Refs {
		if href, ok := byID[ref.IDRef]; ok {
			// the href stays as the OPF writes it (the reader names a chapter so); the place in the archive is the decoded one
			decoded, err := url.PathUnescape(href)
			if err != nil {
				decoded = href
			}
			out = append(out, spineItem{Href: href, Zip: path.Join(path.Dir(opfPath), decoded)})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("empty reading order")
	}
	return out, nil
}
