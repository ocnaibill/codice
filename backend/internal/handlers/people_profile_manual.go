package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// The profile of a person written by hand (DEC-167): for an author no provider knows, or that one of them has wrong. Owner and admin write the
// description, the years, the place, the biography and the photo, and from then on the profile is theirs: the worker does not read it again.
const (
	maxProfileDescription = 500
	maxProfilePlace       = 255
	maxProfileBio         = 6000
	maxProfileCredit      = 300
	maxProfilePhotoBytes  = 5 << 20
)

// profileDateRe is a year, a month or a day, as the profile keeps them: "1962", "1962-11", "1962-11-12"; "-0384" is before the common era.
var profileDateRe = regexp.MustCompile(`^-?[0-9]{4}(-(0[1-9]|1[0-2])(-(0[1-9]|[12][0-9]|3[01]))?)?$`)

// profileTexts are the fields of a profile that staff can write; a field that is not sent is left as it was, and one sent empty is cleared.
type profileTexts struct {
	Description  *string `json:"description"`
	Born         *string `json:"born"`
	Died         *string `json:"died"`
	BornPlace    *string `json:"bornPlace"`
	Bio          *string `json:"bio"`
	ImageCredit  *string `json:"imageCredit"`
	ImageLicense *string `json:"imageLicense"`
}

func (t profileTexts) any() bool {
	return t.Description != nil || t.Born != nil || t.Died != nil || t.BornPlace != nil || t.Bio != nil || t.ImageCredit != nil || t.ImageLicense != nil
}

// cleanLine is a one-line text: the stray spaces go, as they do in a name.
func cleanLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cleanBlock is a text of paragraphs: the line breaks stay and the edges are trimmed.
func cleanBlock(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// validate cleans what was sent and says what is wrong with it, in the words the page shows.
func (t *profileTexts) validate() string {
	one := func(p **string, limit int, message string) string {
		if *p == nil {
			return ""
		}
		v := cleanLine(**p)
		if utf8.RuneCountInString(v) > limit {
			return message
		}
		*p = &v
		return ""
	}
	if m := one(&t.Description, maxProfileDescription, "A descrição tem até 500 caracteres."); m != "" {
		return m
	}
	if m := one(&t.BornPlace, maxProfilePlace, "O local tem até 255 caracteres."); m != "" {
		return m
	}
	for _, p := range []**string{&t.ImageCredit, &t.ImageLicense} {
		if m := one(p, maxProfileCredit, "O crédito e a licença da foto têm até 300 caracteres."); m != "" {
			return m
		}
	}
	for _, p := range []**string{&t.Born, &t.Died} {
		if *p == nil {
			continue
		}
		v := cleanLine(**p)
		if v != "" && !profileDateRe.MatchString(v) {
			return "A data deve ser 1962, 1962-11 ou 1962-11-12."
		}
		*p = &v
	}
	if t.Bio != nil {
		v := cleanBlock(*t.Bio)
		if utf8.RuneCountInString(v) > maxProfileBio {
			return "A biografia tem até 6000 caracteres."
		}
		t.Bio = &v
	}
	return ""
}

// storedProfile is the row of a profile, as far as writing by hand needs to know it.
type storedProfile struct {
	exists                                         bool
	description, born, died, place, bio            string
	bioLanguage, bioTitle, bioURL, credit, license sql.NullString
	licenseURL, pageURL                            sql.NullString
}

func loadStoredProfile(r *http.Request, tx *sql.Tx, id int) (storedProfile, error) {
	var p storedProfile
	var born, died, place sql.NullString
	err := tx.QueryRowContext(r.Context(), `
		SELECT description, born, died, born_place, bio, bio_language, bio_title, bio_url, image_credit, image_license, image_license_url, image_page_url
		FROM person_profile WHERE person_id = $1 FOR UPDATE`, id).
		Scan(&p.description, &born, &died, &place, &p.bio, &p.bioLanguage, &p.bioTitle, &p.bioURL, &p.credit, &p.license, &p.licenseURL, &p.pageURL)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.exists, p.born, p.died, p.place = true, born.String, died.String, place.String
	return p, nil
}

// writeProfile writes by hand the fields that were sent (and the two choices to hide, if they came with them), making the profile the staff's own.
// A person that has no profile gets one. What the staff did not touch stays what it was, the Wikipedia source of a biography included.
func (h *PeopleHandler) writeProfile(w http.ResponseWriter, r *http.Request, id int, hidden, imageHidden *bool, t profileTexts) {
	if msg := t.validate(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving the profile", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var known bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM person WHERE id = $1)`, id).Scan(&known); err != nil || !known {
		if err != nil {
			log.Println("Error looking for a person:", err)
			http.Error(w, "Error saving the profile", http.StatusInternalServerError)
			return
		}
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	cur, err := loadStoredProfile(r, tx, id)
	if err != nil {
		log.Println("Error reading a profile:", err)
		http.Error(w, "Error saving the profile", http.StatusInternalServerError)
		return
	}
	pick := func(sent *string, was string) string {
		if sent != nil {
			return *sent
		}
		return was
	}
	next := cur
	next.description = pick(t.Description, cur.description)
	next.born, next.died, next.place = pick(t.Born, cur.born), pick(t.Died, cur.died), pick(t.BornPlace, cur.place)
	next.bio = pick(t.Bio, cur.bio)
	if t.Bio != nil && *t.Bio != cur.bio {
		// A biography that was written is not Wikipedia's any more.
		next.bioLanguage, next.bioTitle, next.bioURL = sql.NullString{}, sql.NullString{}, sql.NullString{}
	}
	next.credit = sql.NullString{String: pick(t.ImageCredit, cur.credit.String), Valid: true}
	next.license = sql.NullString{String: pick(t.ImageLicense, cur.license.String), Valid: true}
	if next.credit.String != cur.credit.String || next.license.String != cur.license.String {
		// The links were of the credit and the license that were there.
		next.licenseURL, next.pageURL = sql.NullString{}, sql.NullString{}
	}
	actor := currentUserID(r)
	if cur.exists {
		_, err = tx.ExecContext(r.Context(), `
			UPDATE person_profile SET description = $2, born = NULLIF($3, ''), died = NULLIF($4, ''), born_place = NULLIF($5, ''), place_read = TRUE,
			       bio = $6, bio_language = $7, bio_title = $8, bio_url = $9, bio_state = CASE WHEN $6 = '' THEN 'none' ELSE 'done' END,
			       image_credit = NULLIF($10, ''), image_license = NULLIF($11, ''), image_license_url = $12, image_page_url = $13,
			       hidden = COALESCE($14, hidden), image_hidden = COALESCE($15, image_hidden), manual = TRUE, updated_by = NULLIF($16, '')::uuid
			WHERE person_id = $1`,
			id, next.description, next.born, next.died, next.place, next.bio, next.bioLanguage, next.bioTitle, next.bioURL,
			next.credit.String, next.license.String, next.licenseURL, next.pageURL, hidden, imageHidden, actor)
	} else {
		_, err = tx.ExecContext(r.Context(), `
			INSERT INTO person_profile (person_id, wikidata_id, description, born, died, born_place, place_read, bio, bio_state,
			                            image_credit, image_license, hidden, image_hidden, manual, updated_by)
			VALUES ($1, NULL, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), TRUE, $6, CASE WHEN $6 = '' THEN 'none' ELSE 'done' END,
			        NULLIF($7, ''), NULLIF($8, ''), COALESCE($9, FALSE), COALESCE($10, FALSE), TRUE, NULLIF($11, '')::uuid)`,
			id, next.description, next.born, next.died, next.place, next.bio, next.credit.String, next.license.String, hidden, imageHidden, actor)
	}
	if err != nil {
		log.Println("Error writing a profile:", err)
		http.Error(w, "Error saving the profile", http.StatusInternalServerError)
		return
	}
	var fields []string
	for name, sent := range map[string]*string{"description": t.Description, "born": t.Born, "died": t.Died, "bornPlace": t.BornPlace, "bio": t.Bio, "imageCredit": t.ImageCredit, "imageLicense": t.ImageLicense} {
		if sent != nil {
			fields = append(fields, name)
		}
	}
	if err := audit.Record(r.Context(), tx, actor, "person.profile.write", "person", strconv.Itoa(id), map[string]any{"fields": fields, "created": !cur.exists}); err != nil {
		log.Println("Could not audit person.profile.write:", err)
		http.Error(w, "Error saving the profile", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error saving the profile", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// photoTypes are the images a photo of a person can be, by what the bytes say they are.
var photoTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// SetPhoto answers POST /admin/people/{id}/profile/photo (staff): the photo of the person, sent as a file in "image" with the "credit" and the
// "license" it is shown under. A person with no profile gets one; the photo that was there is not theirs any more, nor its credit.
func (h *PeopleHandler) SetPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	if h.CoversDir == "" {
		http.Error(w, "Photos cannot be kept here", http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProfilePhotoBytes+(64<<10))
	if err := r.ParseMultipartForm(maxProfilePhotoBytes); err != nil {
		http.Error(w, "A foto tem até 5 MB.", http.StatusRequestEntityTooLarge)
		return
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Envie a foto no campo image.", http.StatusBadRequest)
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	ext, ok := photoTypes[http.DetectContentType(head[:n])]
	if !ok {
		http.Error(w, "A foto deve ser JPEG, PNG ou WebP.", http.StatusBadRequest)
		return
	}
	texts := profileTexts{}
	credit, license := r.FormValue("credit"), r.FormValue("license")
	texts.ImageCredit, texts.ImageLicense = &credit, &license
	if msg := texts.validate(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(h.CoversDir, 0o755); err != nil {
		log.Println("Error preparing the folder of the photos:", err)
		http.Error(w, "Error saving the photo", http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("person_%d_m%d%s", id, time.Now().UnixNano(), ext)
	dest := filepath.Join(h.CoversDir, name)
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		log.Println("Error creating the photo:", err)
		http.Error(w, "Error saving the photo", http.StatusInternalServerError)
		return
	}
	_, err = io.Copy(out, io.MultiReader(strings.NewReader(string(head[:n])), file))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
		log.Println("Error writing the photo:", err)
		http.Error(w, "Error saving the photo", http.StatusInternalServerError)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		os.Remove(dest)
		http.Error(w, "Error saving the photo", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var known bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM person WHERE id = $1)`, id).Scan(&known); err != nil || !known {
		os.Remove(dest)
		if err != nil {
			http.Error(w, "Error saving the photo", http.StatusInternalServerError)
			return
		}
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	var previous sql.NullString
	if err := tx.QueryRowContext(r.Context(), `SELECT image_path FROM person_profile WHERE person_id = $1 FOR UPDATE`, id).Scan(&previous); err != nil && !errors.Is(err, sql.ErrNoRows) {
		os.Remove(dest)
		http.Error(w, "Error saving the photo", http.StatusInternalServerError)
		return
	}
	actor := currentUserID(r)
	url := "/covers/" + name
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO person_profile (person_id, wikidata_id, description, place_read, bio, bio_state, image_path, image_credit, image_license, manual, updated_by)
		VALUES ($1, NULL, '', TRUE, '', 'none', $2, NULLIF($3, ''), NULLIF($4, ''), TRUE, NULLIF($5, '')::uuid)
		ON CONFLICT (person_id) DO UPDATE SET image_path = EXCLUDED.image_path, image_credit = EXCLUDED.image_credit, image_license = EXCLUDED.image_license,
		       image_license_url = NULL, image_page_url = NULL, image_hidden = FALSE, place_read = TRUE, manual = TRUE, updated_by = EXCLUDED.updated_by,
		       bio_state = CASE WHEN person_profile.bio_state = 'pending' THEN 'none' ELSE person_profile.bio_state END`,
		id, url, credit, license, actor)
	if err == nil {
		err = audit.Record(r.Context(), tx, actor, "person.profile.photo", "person", strconv.Itoa(id), map[string]any{"bytes": n})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		os.Remove(dest)
		log.Println("Error saving a photo:", err)
		http.Error(w, "Error saving the photo", http.StatusInternalServerError)
		return
	}
	// The photo that staff sent before is not kept; the ones the worker downloaded are not touched here (the worker owns them).
	if previous.Valid && strings.HasPrefix(previous.String, fmt.Sprintf("/covers/person_%d_m", id)) && previous.String != url {
		os.Remove(filepath.Join(h.CoversDir, filepath.Base(previous.String)))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// ResetProfile answers DELETE /admin/people/{id}/profile (staff): the profile is thrown away, and the one the person has on Wikidata, if any, is
// read again. What was written by hand is lost, so the page asks first.
func (h *PeopleHandler) ResetProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error discarding the profile", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var photo sql.NullString
	err = tx.QueryRowContext(r.Context(), `DELETE FROM person_profile WHERE person_id = $1 RETURNING image_path`, id).Scan(&photo)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}
	if err == nil {
		// Without this a lookup remembered as done would keep the worker from reading it again.
		_, err = tx.ExecContext(r.Context(), `
			DELETE FROM authority_lookups WHERE source = 'wikidata'
			  AND key IN (SELECT value FROM person_authority WHERE person_id = $1 AND scheme = 'wikidata')`, id)
	}
	if err == nil {
		err = audit.Record(r.Context(), tx, currentUserID(r), "person.profile.reset", "person", strconv.Itoa(id), map[string]any{})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Println("Error discarding a profile:", err)
		http.Error(w, "Error discarding the profile", http.StatusInternalServerError)
		return
	}
	if photo.Valid && h.CoversDir != "" && strings.HasPrefix(photo.String, fmt.Sprintf("/covers/person_%d_m", id)) {
		os.Remove(filepath.Join(h.CoversDir, filepath.Base(photo.String)))
	}
	w.WriteHeader(http.StatusNoContent)
}
