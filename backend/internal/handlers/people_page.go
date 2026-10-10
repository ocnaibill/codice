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
	"github.com/ocnaibill/codice/backend/internal/people"
)

// PersonPage is what the page of a person says (#186): who they are, the names they go by, in which roles the library has works of
// theirs, and the collections those works are in. The works themselves are the list of works with `person=` and `role=`.
type PersonPage struct {
	ID int `json:"id"`
	// Name is the name as it is stored; DisplayName is how the account that asks is shown it.
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Aliases     []string `json:"aliases"`
	// Roles are the roles the person has on works of the library, with how many works, in the order the roles are listed.
	Roles       []PersonRole       `json:"roles"`
	Collections []PersonCollection `json:"collections"`
	// Profile is what Wikidata and Wikipedia say about the person (DEC-146): null when nothing was read, or when staff hid it (staff still see it,
	// marked hidden, to be able to show it again).
	Profile *PersonProfile `json:"profile"`
	// Stats are the works of the person in the library and how far the caller is in them (DEC-157): private to the caller, like their reading.
	Stats PersonStats `json:"stats"`
	// Tags are the tags the works of the person carry most, with how many works each, for "themes" (at most personTags).
	Tags []PersonTag `json:"tags"`
}

// personTags is how many tags a page of a person lists.
const personTags = 8

// PersonStats are numbers about the works of a person. Works is the library's; the rest is the caller's own.
type PersonStats struct {
	// Works is how many works the library has with the person in any role (a work in the trash does not count).
	Works int `json:"works"`
	// Finished is how many of them the caller finished: marked as finished, or with a file read to the end.
	Finished int `json:"finished"`
	// InProgress is how many they have begun and not finished.
	InProgress int `json:"inProgress"`
	// ReadingSeconds is the time they spent reading those works, in all their files.
	ReadingSeconds int `json:"readingSeconds"`
}

// PersonTag is a tag, and how many works of the person carry it.
type PersonTag struct {
	Name  string `json:"name"`
	Works int    `json:"works"`
}

// PersonProfile is the profile of a person: a short description, the years, a biography with where it came from, and a photo with its credit.
type PersonProfile struct {
	WikidataID  string `json:"wikidataId,omitempty"`
	Description string `json:"description"`
	Born        string `json:"born,omitempty"`
	// BornPlace is the name of the place they were born in (DEC-160), from Wikidata; absent when it has none.
	BornPlace string `json:"bornPlace,omitempty"`
	Died      string `json:"died,omitempty"`
	// Bio is the first paragraphs of the page of the person on Wikipedia, under CC BY-SA 4.0: BioSource says whose it is and where it is.
	Bio       string            `json:"bio,omitempty"`
	BioSource *ProfileBioSource `json:"bioSource,omitempty"`
	Image     *ProfileImage     `json:"image,omitempty"`
	// Hidden and ImageHidden are what staff chose; they are only told to staff.
	Hidden      bool `json:"hidden,omitempty"`
	ImageHidden bool `json:"imageHidden,omitempty"`
	// Manual says the profile was written by hand (DEC-167); only staff are told.
	Manual bool `json:"manual,omitempty"`
}

// ProfileBioSource is the page a biography is taken from.
type ProfileBioSource struct {
	Language string `json:"language"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	License  string `json:"license"`
}

// ProfileImage is the photo of a person, kept on the server, with who made it and under which license it is shown.
type ProfileImage struct {
	URL        string `json:"url"`
	Credit     string `json:"credit,omitempty"`
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	PageURL    string `json:"pageUrl,omitempty"`
}

// readProfile is the profile of a person as the one who asks may see it: a hidden one is not there for who is not staff, and a hidden photo is
// not there for anybody (staff are told it is hidden, so that they can show it).
func readProfile(r *http.Request, db *sql.DB, personID int) (*PersonProfile, error) {
	var (
		p                                            PersonProfile
		born, died, bioLang, bioTitle, bioURL        sql.NullString
		bornPlace                                    sql.NullString
		path, credit, license, licenseURL, imagePage sql.NullString
		bio                                          string
	)
	err := db.QueryRowContext(r.Context(), `
		SELECT COALESCE(wikidata_id, ''), description, born, died, born_place, bio, bio_language, bio_title, bio_url,
		       image_path, image_credit, image_license, image_license_url, image_page_url, hidden, image_hidden, manual
		FROM person_profile WHERE person_id = $1`, personID).Scan(&p.WikidataID, &p.Description, &born, &died, &bornPlace, &bio, &bioLang, &bioTitle, &bioURL,
		&path, &credit, &license, &licenseURL, &imagePage, &p.Hidden, &p.ImageHidden, &p.Manual)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	staff := isStaffRequest(r)
	if p.Hidden && !staff {
		return nil, nil
	}
	p.Born, p.Died, p.BornPlace = born.String, died.String, bornPlace.String
	// A biography written by hand has no source to cite (editing it clears the language); one from Wikipedia always does, even in a profile
	// that was touched by hand in some other field.
	if bio != "" && (bioLang.Valid || p.Manual) {
		p.Bio = bio
		if bioLang.Valid {
			p.BioSource = &ProfileBioSource{Language: bioLang.String, Title: bioTitle.String, URL: bioURL.String, License: "CC BY-SA 4.0"}
		}
	}
	if path.Valid && path.String != "" && (!p.ImageHidden || staff) {
		p.Image = &ProfileImage{URL: path.String, Credit: credit.String, License: license.String, LicenseURL: licenseURL.String, PageURL: imagePage.String}
	}
	if !staff {
		p.Hidden, p.ImageHidden, p.Manual = false, false, false
	}
	return &p, nil
}

// PersonRole is a role of a person and how many works the library has of theirs with it.
type PersonRole struct {
	Role  string `json:"role"`
	Works int    `json:"works"`
}

// PersonCollection is an official collection with works of the person, and how many of those.
type PersonCollection struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Works int    `json:"works"`
}

// Page answers GET /people/{id}. The works that are retired do not count, for anybody.
func (h *PeopleHandler) Page(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	order := people.OrderFor(r.Context(), h.DB, currentUserID(r)).Effective
	page := PersonPage{Aliases: []string{}, Roles: []PersonRole{}, Collections: []PersonCollection{}}
	err = h.DB.QueryRowContext(r.Context(), `SELECT p.id, p.name, `+displayNameSQL("p", "$2")+` FROM person p WHERE p.id = $1`, id, order).
		Scan(&page.ID, &page.Name, &page.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Person not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("Error reading a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}

	// The other names they are written with, except the one they go by.
	rows, err := h.DB.QueryContext(r.Context(), `SELECT alias FROM person_alias WHERE person_id = $1 AND alias <> $2 ORDER BY lower(alias), alias`, id, page.Name)
	if err != nil {
		log.Println("Error reading the aliases of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Aliases = append(page.Aliases, a)
	}
	rows.Close()

	rows, err = h.DB.QueryContext(r.Context(), `
		SELECT c.role, count(DISTINCT c.work_id)
		FROM work_contributors c JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		WHERE c.person_id = $1
		GROUP BY c.role
		ORDER BY array_position($2::text[], c.role::text)`, id, "{"+strings.Join(roleOrder, ",")+"}")
	if err != nil {
		log.Println("Error reading the roles of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var pr PersonRole
		if err := rows.Scan(&pr.Role, &pr.Works); err != nil {
			rows.Close()
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Roles = append(page.Roles, pr)
	}
	rows.Close()

	rows, err = h.DB.QueryContext(r.Context(), `
		SELECT col.id, col.name, count(DISTINCT cw.work_id)
		FROM work_contributors c
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		JOIN collection_works cw ON cw.work_id = w.id AND cw.official
		JOIN collections col ON col.id = cw.collection_id AND col.retired_at IS NULL AND col.kind = 'official'
		WHERE c.person_id = $1
		GROUP BY col.id, col.name
		ORDER BY lower(col.name), col.id`, id)
	if err != nil {
		log.Println("Error reading the collections of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pc PersonCollection
		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Works); err != nil {
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Collections = append(page.Collections, pc)
	}
	if err := h.DB.QueryRowContext(r.Context(), `
		WITH pw AS (
			SELECT DISTINCT c.work_id FROM work_contributors c JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL WHERE c.person_id = $1
		), st AS (
			SELECT pw.work_id,
			       EXISTS (SELECT 1 FROM work_reading_state s WHERE s.user_id = $2::uuid AND s.work_id = pw.work_id)
			         OR EXISTS (SELECT 1 FROM reading_progress r JOIN files f ON f.id = r.file_id JOIN editions e ON e.id = f.edition_id
			                    WHERE e.work_id = pw.work_id AND r.user_id = $2::uuid AND r.completed_at IS NOT NULL) AS finished,
			       EXISTS (SELECT 1 FROM reading_progress r JOIN files f ON f.id = r.file_id JOIN editions e ON e.id = f.edition_id
			               WHERE e.work_id = pw.work_id AND r.user_id = $2::uuid AND `+hasPosition("r")+`) AS begun
			FROM pw
		)
		SELECT count(*), count(*) FILTER (WHERE finished), count(*) FILTER (WHERE begun AND NOT finished),
		       COALESCE((SELECT sum(r.reading_seconds) FROM reading_progress r JOIN files f ON f.id = r.file_id JOIN editions e ON e.id = f.edition_id
		                 WHERE r.user_id = $2::uuid AND e.work_id IN (SELECT work_id FROM pw)), 0)
		FROM st`, id, currentUserID(r)).Scan(&page.Stats.Works, &page.Stats.Finished, &page.Stats.InProgress, &page.Stats.ReadingSeconds); err != nil {
		log.Println("Error reading the numbers of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}

	page.Tags = []PersonTag{}
	trows, err := h.DB.QueryContext(r.Context(), `
		SELECT t.name, count(DISTINCT wt.work_id) AS works
		FROM work_contributors c
		JOIN works w ON w.id = c.work_id AND w.retired_at IS NULL
		JOIN work_tags wt ON wt.work_id = w.id JOIN tags t ON t.id = wt.tag_id
		WHERE c.person_id = $1
		GROUP BY t.name ORDER BY works DESC, lower(t.name), t.name LIMIT $2`, id, personTags)
	if err != nil {
		log.Println("Error reading the tags of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	for trows.Next() {
		var pt PersonTag
		if err := trows.Scan(&pt.Name, &pt.Works); err != nil {
			trows.Close()
			http.Error(w, "Error reading the person", http.StatusInternalServerError)
			return
		}
		page.Tags = append(page.Tags, pt)
	}
	trows.Close()

	if page.Profile, err = readProfile(r, h.DB, id); err != nil {
		log.Println("Error reading the profile of a person:", err)
		http.Error(w, "Error reading the person", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
