package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestSeriesData_SaysHowTheSeriesStandsAndItsTitleInTheScriptOfItsLanguage(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.seriesOf("Berserk", "chapter")
	path := fmt.Sprintf("/collections/%d", col)
	if d, _ := s.collection(ana, col); d.Collection.PublicationStatus != "" || d.Collection.OriginalTitle != "" {
		t.Fatalf("nobody said: %+v", d.Collection)
	}
	if rec := s.do(admin, "PATCH", path, `{"publicationStatus":"hiatus","originalTitle":"  ベルセルク "}`); rec.Code != 200 {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	d, _ := s.collection(ana, col)
	if d.Collection.PublicationStatus != "hiatus" || d.Collection.OriginalTitle != "ベルセルク" || d.Collection.Name != "Berserk" {
		t.Errorf("the page says: %+v", d.Collection)
	}
	// What is said is the page's for everybody, and the rest of the series is untouched.
	if s.auditCount("collection.describe") != "1" || s.auditCount("collection.rename") != "0" {
		t.Errorf("audit: %s %s", s.auditCount("collection.describe"), s.auditCount("collection.rename"))
	}
	if s.scalar(`SELECT details->>'publicationStatus' || '|' || (details->>'originalTitle') FROM audit_log WHERE action = 'collection.describe'`) != "hiatus|ベルセルク" {
		t.Errorf("the audit says what was set")
	}
	// Each one on its own leaves the other.
	if rec := s.do(admin, "PATCH", path, `{"publicationStatus":"finished"}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if d, _ := s.collection(ana, col); d.Collection.PublicationStatus != "finished" || d.Collection.OriginalTitle != "ベルセルク" {
		t.Errorf("only the status changed: %+v", d.Collection)
	}
	// Along with the name and the direction, in one request.
	if rec := s.do(admin, "PATCH", path, `{"name":"Berserk (Miura)","readingDirection":"rtl","publicationStatus":"ongoing","originalTitle":"ベルセルク"}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if d, _ := s.collection(ana, col); d.Collection.Name != "Berserk (Miura)" || d.Collection.ReadingDirection != "rtl" || d.Collection.PublicationStatus != "ongoing" {
		t.Errorf("all together: %+v", d.Collection)
	}
	// Empty takes each away.
	if rec := s.do(admin, "PATCH", path, `{"publicationStatus":"","originalTitle":""}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if s.scalar(`SELECT (publication_status IS NULL)::text || (original_title IS NULL)::text FROM collections WHERE id = $1`, col) != "truetrue" {
		t.Errorf("empty is NULL")
	}
}

func TestSeriesData_RefusesWhatIsNotAStateOrTooLongAndIsNotForLists(t *testing.T) {
	s := newCatalogStack(t)
	col, _ := s.seriesOf("Berserk", "chapter")
	path := fmt.Sprintf("/collections/%d", col)
	for name, body := range map[string]string{
		"state": `{"publicationStatus":"paused"}`,
		"caps":  `{"publicationStatus":"ONGOING"}`,
		"title": `{"originalTitle":"` + strings.Repeat("あ", 256) + `"}`,
	} {
		if rec := s.do(admin, "PATCH", path, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	list := s.makeList(ana, "Minha")
	for _, body := range []string{`{"publicationStatus":"ongoing"}`, `{"originalTitle":"x"}`} {
		if rec := s.do(ana, "PATCH", fmt.Sprintf("/my/collections/%d", list), body); rec.Code != http.StatusBadRequest {
			t.Errorf("a list: %s %d, want 400", body, rec.Code)
		}
	}
	s.do(admin, "DELETE", path, "")
	if rec := s.do(admin, "PATCH", path, `{"publicationStatus":"ongoing"}`); rec.Code != http.StatusConflict {
		t.Errorf("retired: %d, want 409", rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM collections WHERE id = $1 AND (publication_status IS NOT NULL OR original_title IS NOT NULL)`, col) != "0" {
		t.Errorf("a refused request changed it")
	}
}
