package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func (s *catalogStack) put(id int, body string) int {
	s.t.Helper()
	return s.do(admin, "PUT", fmt.Sprintf("/works/%d", id), body).Code
}

func (s *catalogStack) meta(id int) *WorkMetadata {
	s.t.Helper()
	w, code := s.detail(admin, id)
	if code != 200 || w.Metadata == nil {
		s.t.Fatalf("detail: code=%d metadata=%v", code, w.Metadata)
	}
	return w.Metadata
}

func TestEditWork_SavesEveryFieldAndOnlyLocksWhatChanged(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Dune", "F. Herbert", "d.epub", "epub")
	s.exec(`UPDATE works SET description = 'Descrição do arquivo', series = 'Dune Chronicles', series_index = 1 WHERE id = $1`, id)
	s.exec(`UPDATE editions SET isbn = '111', publisher = 'Velha Editora', language = 'en' WHERE work_id = $1 AND is_primary`, id)

	// Change several fields, leave ISBN and description as they are.
	body := `{"title":"Duna","author":"Frank Herbert","tags":[],"series":"Crônicas de Duna","series_index":2.5,
	          "isbn":"111","publisher":"Aleph","language":"pt","publication_date":"2017","description":"Descrição do arquivo"}`
	if code := s.put(id, body); code != 200 {
		t.Fatalf("update: %d", code)
	}

	m := s.meta(id)
	if m.Series != "Crônicas de Duna" || m.SeriesIndex != 2.5 || m.Publisher != "Aleph" || m.Language != "pt" || m.PublicationDate != "2017" {
		t.Errorf("metadata = %+v", m)
	}
	if m.ISBN != "111" || m.Description != "Descrição do arquivo" {
		t.Errorf("untouched fields changed: %+v", m)
	}
	wantLocked := map[string]bool{"title": true, "author": true, "series": true, "publisher": true, "language": true, "publication_date": true}
	for field, want := range map[string]bool{"title": true, "author": true, "series": true, "publisher": true, "language": true, "publication_date": true,
		"isbn": false, "description": false, "cover": false} {
		if m.Locks[field] != want {
			t.Errorf("lock %s = %v, want %v", field, m.Locks[field], want)
		}
	}
	_ = wantLocked
	for _, f := range []string{"title", "author", "series", "publisher", "language", "publication_date"} {
		if m.Sources[f] != "manual" {
			t.Errorf("source of %s = %q, want manual", f, m.Sources[f])
		}
	}
	if _, has := m.Sources["isbn"]; has {
		t.Error("an unchanged field must not get a provenance entry")
	}
	// The edition (what the catalog calls language, publisher and date) follows.
	if got := s.scalar(`SELECT language || '/' || publisher || '/' || publication_date FROM editions WHERE work_id = $1 AND is_primary`, id); got != "pt/Aleph/2017" {
		t.Errorf("primary edition = %q", got)
	}
	// One audit entry lists what changed, and only that.
	if got := s.scalar(`SELECT (details->'publisher'->>'to') || '|' || (details ? 'isbn')::text FROM audit_log WHERE action = 'work.update'`); got != "Aleph|false" {
		t.Errorf("audit details = %q", got)
	}
}

func TestEditWork_AbsentFieldsAreKeptAndEmptyOnesAreCleared(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.exec(`UPDATE works SET description = 'texto' WHERE id = $1`, id)
	s.exec(`UPDATE editions SET isbn = '111', publisher = 'Aleph' WHERE work_id = $1 AND is_primary`, id)

	// The old client sent only title, author and tags: nothing else may be lost.
	s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":["a"]}`)
	if m := s.meta(id); m.ISBN != "111" || m.Publisher != "Aleph" || m.Description != "texto" {
		t.Errorf("fields absent from the request were lost: %+v", m)
	}

	// An explicit empty string clears a field, and it stays confirmed.
	s.put(id, `{"title":"Duna","author":"Frank Herbert","tags":[],"isbn":""}`)
	m := s.meta(id)
	if m.ISBN != "" || !m.Locks["isbn"] {
		t.Errorf("cleared isbn = %q locked=%v", m.ISBN, m.Locks["isbn"])
	}
	if got := s.scalar(`SELECT COALESCE(isbn, 'NULL') FROM editions WHERE work_id = $1 AND is_primary`, id); got != "NULL" {
		t.Errorf("a cleared field is stored as %q, want NULL", got)
	}
	if m.Publisher != "Aleph" {
		t.Error("clearing one field touched another")
	}
}

func TestEditWork_ExplicitLocksProtectOrReleaseUnchangedFields(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "d.epub", "epub")
	s.exec(`UPDATE editions SET publisher = 'Aleph' WHERE work_id = $1 AND is_primary`, id)
	base := `"title":"Duna","author":"Frank Herbert","tags":[]`

	// Protect an unchanged field.
	s.put(id, `{`+base+`,"publisher_lock":true}`)
	if !s.meta(id).Locks["publisher"] {
		t.Error("an explicit lock on an unchanged field was ignored")
	}
	// Release it again.
	s.put(id, `{`+base+`,"publisher_lock":false}`)
	if s.meta(id).Locks["publisher"] {
		t.Error("an explicit unlock was ignored")
	}
	// A changed field is confirmed even if the client asks for it unlocked:
	// a manual edit is a confirmation (RN-008).
	s.put(id, `{`+base+`,"publisher":"Outra","publisher_lock":false}`)
	if m := s.meta(id); m.Publisher != "Outra" || !m.Locks["publisher"] {
		t.Errorf("a changed field must end up locked: %+v", m)
	}
	// Absent lock flags leave locks as they are.
	s.put(id, `{`+base+`}`)
	if !s.meta(id).Locks["publisher"] {
		t.Error("omitting the flag released the lock")
	}
}

func (s *catalogStack) addCandidate(workID int, field, value, source string) int {
	s.t.Helper()
	var id int
	if err := s.db.QueryRow(`INSERT INTO metadata_candidates (work_id, field, value, source) VALUES ($1, $2, $3, $4) RETURNING id`,
		workID, field, value, source).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

type candidateList struct{ Data []Candidate }

func (s *catalogStack) candidates(id int) []Candidate {
	s.t.Helper()
	rec := s.do(admin, "GET", fmt.Sprintf("/works/%d/candidates", id), "")
	if rec.Code != 200 {
		s.t.Fatalf("candidates: %d", rec.Code)
	}
	var l candidateList
	json.Unmarshal(rec.Body.Bytes(), &l)
	return l.Data
}

func (s *catalogStack) decide(workID, candID int, verb string) int {
	return s.do(admin, "POST", "/works/"+strconv.Itoa(workID)+"/candidates/"+strconv.Itoa(candID)+"/"+verb, "").Code
}

func TestCandidates_AcceptRejectAndSettle(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("dune.epub", "", "d.epub", "epub")
	other := s.addWork("Outra", "X", "o.epub", "epub")
	s.do(ana, "POST", fmt.Sprintf("/works/%d/notes", id), `{"quote":"da Ana"}`)

	title1 := s.addCandidate(id, "title", "Duna", "openlibrary")
	title2 := s.addCandidate(id, "title", "Dune", "google_books")
	author := s.addCandidate(id, "author", "Frank Herbert", "openlibrary")
	idx := s.addCandidate(id, "series_index", "1", "comicvine")
	tags := s.addCandidate(id, "tags", `["Sci-Fi","Clássico"]`, "openlibrary")
	isbn := s.addCandidate(id, "isbn", "9788576573135", "google_books")
	foreign := s.addCandidate(other, "isbn", "999", "google_books")

	list := s.candidates(id)
	if len(list) != 6 {
		t.Fatalf("pending candidates = %d, want 6", len(list))
	}
	for _, c := range list {
		if c.Field == "title" && c.Current != "dune.epub" {
			t.Errorf("the current value should be listed beside the suggestion: %+v", c)
		}
	}

	// Readers cannot see or decide them (route-level guard is tested elsewhere;
	// here the handlers themselves must not touch another work's candidate).
	if code := s.decide(id, foreign, "accept"); code != 404 {
		t.Errorf("accepting another work's candidate: %d, want 404", code)
	}
	if code := s.decide(id, 999999, "accept"); code != 404 {
		t.Errorf("unknown candidate: %d, want 404", code)
	}

	// Accept a title: applied, locked, provenance names the provider, notes follow,
	// and the rival suggestion for the same field is dismissed.
	if code := s.decide(id, title1, "accept"); code != 200 {
		t.Fatalf("accept title: %d", code)
	}
	m := s.meta(id)
	if !m.Locks["title"] || m.Sources["title"] != "openlibrary" {
		t.Errorf("after accepting: locked=%v source=%q", m.Locks["title"], m.Sources["title"])
	}
	if got := s.scalar(`SELECT original_title FROM works WHERE id = $1`, id); got != "Duna" {
		t.Errorf("title = %q", got)
	}
	if got := s.scalar(`SELECT source_title FROM notes WHERE work_id = $1`, id); got != "Duna" {
		t.Errorf("the note reference must follow an accepted title: %q", got)
	}
	if got := s.scalar(`SELECT state FROM metadata_candidates WHERE id = $1`, title2); got != "rejected" {
		t.Errorf("the rival title suggestion is %q, want rejected", got)
	}
	if code := s.decide(id, title1, "accept"); code != 404 {
		t.Errorf("deciding twice: %d, want 404", code)
	}

	// Author, numeric field, tags and ISBN.
	s.decide(id, author, "accept")
	s.decide(id, idx, "accept")
	s.decide(id, tags, "accept")
	if got := s.scalar(`SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id WHERE c.work_id = $1 AND c.role = 'author' AND c.position = 0`, id); got != "Frank Herbert" {
		t.Errorf("author = %q", got)
	}
	if got := s.scalar(`SELECT series_index::text FROM works WHERE id = $1`, id); got != "1" {
		t.Errorf("series_index = %q", got)
	}
	if got := s.scalar(`SELECT string_agg(t.name, ',' ORDER BY t.name) FROM work_tags wt JOIN tags t ON t.id = wt.tag_id WHERE wt.work_id = $1`, id); got != "Clássico,Sci-Fi" {
		t.Errorf("tags = %q", got)
	}
	if got := s.scalar(`SELECT source_author FROM notes WHERE work_id = $1`, id); got != "Frank Herbert" {
		t.Errorf("note author reference = %q", got)
	}

	// Reject: nothing changes, and the decision is remembered.
	if code := s.decide(id, isbn, "reject"); code != 200 {
		t.Fatalf("reject: %d", code)
	}
	if got := s.scalar(`SELECT COALESCE(isbn, 'NULL') FROM editions WHERE work_id = $1 AND is_primary`, id); got != "NULL" {
		t.Errorf("a rejected suggestion was applied: %q", got)
	}
	if m := s.meta(id); m.Locks["isbn"] {
		t.Error("rejecting must not lock the field")
	}
	if left := s.candidates(id); len(left) != 0 {
		t.Errorf("pending after all decisions: %+v", left)
	}
	// The same value from the same source cannot be proposed again (unique key).
	if _, err := s.db.Exec(`INSERT INTO metadata_candidates (work_id, field, value, source) VALUES ($1, 'isbn', '9788576573135', 'google_books')`, id); err == nil {
		t.Error("a rejected suggestion could be proposed again")
	}

	// The audit trail names the decisions.
	if got := s.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'metadata.%'`); got != "metadata.accept,metadata.accept,metadata.accept,metadata.accept,metadata.reject" {
		t.Errorf("audit trail = %q", got)
	}
}

func TestNormalizeTag(t *testing.T) {
	for in, want := range map[string]string{
		"  Ficção   científica ": "Ficção científica",
		"":                       "",
		"Translated by Ebook Translator: https://translator.bookfere.com": "Translated by Ebook Translator",
		strings.Repeat("x", 80):                           strings.Repeat("x", 50),
		strings.Repeat("ç", 60):                           strings.Repeat("ç", 50), // characters, not bytes
		strings.Repeat("x", 49) + ",,,,":                  strings.Repeat("x", 49),
		strings.TrimSpace(strings.Repeat("palavra ", 30)): "palavra palavra palavra palavra palavra palavra",
	} {
		if got := normalizeTag(in); got != want {
			t.Errorf("normalizeTag(%.40q) = %q, want %q", in, got, want)
		}
	}
}

func TestCandidates_AcceptingATagTooLongForTheColumnStoresItShortened(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("dune.epub", "", "d.epub", "epub")
	long := s.addCandidate(id, "tags", `["Translated by Ebook Translator: https://translator.bookfere.com","Sci-Fi"]`, "openlibrary")
	if code := s.decide(id, long, "accept"); code != 200 {
		t.Fatalf("accept: %d", code)
	}
	got := s.scalar(`SELECT string_agg(t.name, '|' ORDER BY t.name) FROM work_tags wt JOIN tags t ON t.id = wt.tag_id WHERE wt.work_id = $1`, id)
	if got != "Sci-Fi|Translated by Ebook Translator" {
		t.Errorf("tags = %q", got)
	}
}
