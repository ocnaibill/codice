package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lib/pq"
)

type retiredPage struct {
	Data       []RetiredWork `json:"data"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	TotalPages int           `json:"totalPages"`
}

func (s *catalogStack) retired(query string) retiredPage {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/retired-works"+query, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /admin/retired-works%s: %d %s", query, rec.Code, rec.Body.String())
	}
	var out retiredPage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatal(err)
	}
	return out
}

func (s *catalogStack) retire(work int) {
	s.t.Helper()
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d", work), ""); rec.Code != 200 {
		s.t.Fatalf("retire %d: %d %s", work, rec.Code, rec.Body.String())
	}
}

func TestRetiredWorks_AWorkThatWasRetiredIsListedWithWhoWhenAndWhatItKeeps(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	other := s.addWork("Neuromancer", "William Gibson", "n.epub", "epub")
	if got := s.retired(""); got.Total != 0 || len(got.Data) != 0 || got.TotalPages != 0 {
		t.Fatalf("nothing retired yet: %+v", got)
	}
	// A second file of another format, with sizes.
	s.addFile(int(s.primaryEdition(duna)), "pdf", "duna.pdf", "managed")
	s.exec(`UPDATE files SET size_bytes = 1000 WHERE id = $1`, s.primaryFile(duna))
	s.exec(`UPDATE files SET size_bytes = 250 WHERE edition_id = (SELECT id FROM editions WHERE work_id = $1 AND is_primary) AND format = 'pdf'`, duna)
	before := time.Now().Add(-time.Minute)
	s.retire(duna)

	got := s.retired("")
	if got.Total != 1 || len(got.Data) != 1 {
		t.Fatalf("the list = %+v", got)
	}
	w := got.Data[0]
	if w.ID != duna || w.Title != "Duna" || w.Author != "Frank Herbert" {
		t.Errorf("the work = %+v", w)
	}
	if w.RetiredBy != "adm" || w.RetiredAt.Before(before) {
		t.Errorf("who and when = %q at %v", w.RetiredBy, w.RetiredAt)
	}
	if w.Files != 2 || w.SizeBytes != 1250 || len(w.Formats) != 2 || w.Formats[0] != "EPUB" || w.Formats[1] != "PDF" {
		t.Errorf("what it keeps = %d files, %d bytes, %v", w.Files, w.SizeBytes, w.Formats)
	}
	// What was not retired is not listed, and what is restored leaves.
	for _, x := range got.Data {
		if x.ID == other {
			t.Errorf("a work that was not retired is listed: %+v", x)
		}
	}
	if rec := s.do(admin, "POST", fmt.Sprintf("/works/%d/restore", duna), ""); rec.Code != 200 {
		t.Fatalf("restore: %d", rec.Code)
	}
	if got := s.retired(""); got.Total != 0 {
		t.Errorf("after restoring: %+v", got)
	}
}

func TestRetiredWorks_TheLatestComesFirstAndThePagesCountTheWhole(t *testing.T) {
	s := newCatalogStack(t)
	var ids []int
	for i := 1; i <= 5; i++ {
		id := s.addWork(fmt.Sprintf("Obra %d", i), "Autor", fmt.Sprintf("o%d.epub", i), "epub")
		ids = append(ids, id)
		s.retire(id)
		s.exec(`UPDATE works SET retired_at = now() - make_interval(hours => $2) WHERE id = $1`, id, 10-i) // 5 is the latest
	}
	first := s.retired("?limit=2")
	if first.Total != 5 || first.TotalPages != 3 || first.Limit != 2 || first.Page != 1 || len(first.Data) != 2 || first.Data[0].ID != ids[4] || first.Data[1].ID != ids[3] {
		t.Errorf("first page = %+v", first)
	}
	third := s.retired("?limit=2&page=3")
	if len(third.Data) != 1 || third.Data[0].ID != ids[0] || third.Page != 3 {
		t.Errorf("third page = %+v", third)
	}
	if beyond := s.retired("?limit=2&page=4"); len(beyond.Data) != 0 || beyond.Total != 5 {
		t.Errorf("beyond the last page = %+v", beyond)
	}
	// What is asked wrong is what is usual: page one, twenty to a page, and never more than a hundred.
	for _, q := range []string{"?page=0", "?page=abc", "?page=-2"} {
		if got := s.retired(q); got.Page != 1 || got.Limit != 20 || len(got.Data) != 5 {
			t.Errorf("%s: %+v", q, got)
		}
	}
	if got := s.retired("?limit=101"); got.Limit != 20 {
		t.Errorf("limit 101: %d", got.Limit)
	}
	if got := s.retired("?limit=100"); got.Limit != 100 {
		t.Errorf("limit 100: %d", got.Limit)
	}
	if got := s.retired("?limit=0"); got.Limit != 20 {
		t.Errorf("limit 0: %d", got.Limit)
	}
}

func TestRetiredWorks_ThoseWithNoFileNoAuthorOrNoOneWhoRetiredThemAreListedAllTheSame(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO works (original_title, retired_at) VALUES ('Sem nada', now())`)
	got := s.retired("")
	if got.Total != 1 || len(got.Data) != 1 {
		t.Fatalf("the list = %+v", got)
	}
	w := got.Data[0]
	if w.Title != "Sem nada" || w.Author != "" || w.RetiredBy != "" || w.Files != 0 || w.SizeBytes != 0 || w.Formats == nil || len(w.Formats) != 0 {
		t.Errorf("the work = %+v (formats %v)", w, w.Formats)
	}
}

func TestRetiredWorks_ThePersonWhoRetiredItIsNamedByTheNameTheyChose(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`UPDATE users SET display_name = 'Dona Adm' WHERE id = $1`, idAdmin)
	s.retire(duna)
	if got := s.retired(""); got.Data[0].RetiredBy != "Dona Adm" {
		t.Errorf("retired by %q", got.Data[0].RetiredBy)
	}
	s.exec(`DELETE FROM users WHERE id = $1`, idAdmin)
	if got := s.retired(""); got.Data[0].RetiredBy != "" {
		t.Errorf("the account is gone: %q", got.Data[0].RetiredBy)
	}
}

func TestRetiredWorks_SendingTheFilesToTheTrashKeepsTheWorkListedUntilTheyAreDeletedForGood(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	// The file is on the disk, as the trash needs it to be.
	os.WriteFile(filepath.Join(s.storage, "duna.epub"), []byte("bytes of the book"), 0o644)
	s.exec(`UPDATE files SET size_bytes = 17 WHERE id = $1`, s.primaryFile(duna))
	s.retire(duna)
	if w := s.retired("").Data[0]; w.InTrash != 0 || w.SizeBytes != 17 {
		t.Fatalf("before: %+v", w)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/works/%d?purge=true", duna), ""); rec.Code != 200 {
		t.Fatalf("purge: %d %s", rec.Code, rec.Body.String())
	}
	// Its files are in the trash, where they can be recovered, and the work is still retired: it says so.
	got := s.retired("")
	if got.Total != 1 || got.Data[0].ID != duna || got.Data[0].Files != 1 || got.Data[0].InTrash != 1 || got.Data[0].SizeBytes != 0 {
		t.Fatalf("after the purge: %+v", got)
	}
	var trash struct {
		Items []struct {
			ID        int64
			WorkTitle string
		}
	}
	json.Unmarshal(s.do(admin, "GET", "/admin/trash", "").Body.Bytes(), &trash)
	if len(trash.Items) != 1 || trash.Items[0].WorkTitle != "Duna" {
		t.Fatalf("the trash = %+v", trash)
	}
	// Recovering the file from the trash leaves it as it was: not in the trash.
	if rec := s.do(admin, "POST", fmt.Sprintf("/admin/trash/%d/restore", trash.Items[0].ID), ""); rec.Code != 200 {
		t.Fatalf("recover: %d", rec.Code)
	}
	if w := s.retired("").Data[0]; w.InTrash != 0 || w.SizeBytes != 17 {
		t.Errorf("after recovering the file: %+v", w)
	}
	// Deleting its files for good takes the work off the list too, since the server keeps none of its bytes.
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d?purge=true", duna), "")
	json.Unmarshal(s.do(admin, "GET", "/admin/trash", "").Body.Bytes(), &trash)
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/admin/trash/%d?confirm=true", trash.Items[0].ID), ""); rec.Code != 204 {
		t.Fatalf("delete for good: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.retired(""); got.Total != 0 {
		t.Errorf("after the files were deleted for good: %+v", got)
	}
}

func TestRetiredWorks_TheOnesRetiredAtTheSameMomentComeLatestFirstByTheirNumber(t *testing.T) {
	s := newCatalogStack(t)
	a := s.addWork("A", "Autor", "a.epub", "epub")
	b := s.addWork("B", "Autor", "b.epub", "epub")
	c := s.addWork("C", "Autor", "c.epub", "epub")
	for _, id := range []int{a, b, c} {
		s.retire(id)
	}
	s.exec(`UPDATE works SET retired_at = '2026-10-01 10:00:00+00' WHERE id = ANY($1)`, pq.Array([]int{a, b, c}))
	got := s.retired("")
	if len(got.Data) != 3 || got.Data[0].ID != c || got.Data[1].ID != b || got.Data[2].ID != a {
		t.Errorf("the order = %v", []int{got.Data[0].ID, got.Data[1].ID, got.Data[2].ID})
	}
}

func TestRetiredWorks_TheAuthorIsTheAuthorsAndNotTheTranslator(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	s.exec(`INSERT INTO person (name) VALUES ('Maria Tradutora')`)
	s.exec(`INSERT INTO work_contributors (work_id, person_id, role, position) SELECT $1, id, 'translator', 0 FROM person WHERE name = 'Maria Tradutora'`, duna)
	s.retire(duna)
	if got := s.retired("").Data[0].Author; got != "Frank Herbert" {
		t.Errorf("author = %q", got)
	}
}

func TestRetiredWorks_TheFormatsAreSaidOnceEachInOrder(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.pdf", "pdf")
	ed := int(s.primaryEdition(duna))
	s.addFile(ed, "epub", "duna.epub", "managed")
	s.addFile(ed, "epub", "duna2.epub", "managed")
	s.addFile(ed, "cbz", "duna.cbz", "managed")
	s.retire(duna)
	w := s.retired("").Data[0]
	if w.Files != 4 || fmt.Sprint(w.Formats) != "[CBZ EPUB PDF]" {
		t.Errorf("%d files, formats %v", w.Files, w.Formats)
	}
}
