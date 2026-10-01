package handlers

import (
	"fmt"
	"strings"
	"testing"
)

// A comic file can say how it is read (#19): the worker records it, and the sheet of the work passes it on to the
// reader, per file, so the CBZ of a work and its PDF do not share one.
func TestWorkDetail_ReportsHowAComicFileSaysItIsRead(t *testing.T) {
	s := newCatalogStack(t)
	manga := s.addWork("Volume Dois", "Autora", "m.cbz", "cbz")
	strip := s.addWork("Tira longa", "Autora", "t.cbz", "cbz")
	plain := s.addWork("Gibi comum", "Autora", "g.cbz", "cbz")
	fileOf := func(work int) string {
		return s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, work))
	}
	s.exec(`UPDATE files SET declared_mode = 'rtl' WHERE id = $1`, fileOf(manga))
	s.exec(`UPDATE files SET declared_mode = 'webtoon' WHERE id = $1`, fileOf(strip))

	for work, want := range map[int]string{manga: "rtl", strip: "webtoon", plain: ""} {
		w, _ := s.detail(ana, work)
		if got := w.Editions[0].Files[0].DeclaredMode; got != want {
			t.Errorf("work %d: declaredMode = %q, want %q", work, got, want)
		}
	}

	// The reader reads it by this name; a file that says nothing does not carry the field at all.
	raw := func(work int) string { return s.do(ana, "GET", fmt.Sprintf("/works/%d", work), "").Body.String() }
	if body := raw(manga); !strings.Contains(body, `"declaredMode":"rtl"`) {
		t.Errorf("the JSON of a right-to-left comic has no declaredMode: %.300s", body)
	}
	if body := raw(plain); strings.Contains(body, "declaredMode") {
		t.Errorf("a comic that says nothing carries declaredMode: %.300s", body)
	}
}

func TestDeclaredMode_OnlyKnownModesAreKept(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Volume", "Autora", "v.cbz", "cbz")
	file := s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, work))
	if _, err := s.db.Exec(`UPDATE files SET declared_mode = 'ltr' WHERE id = $1`, file); err == nil {
		t.Error("a mode the file cannot declare was accepted")
	}
	if _, err := s.db.Exec(`UPDATE files SET declared_mode = NULL WHERE id = $1`, file); err != nil {
		t.Errorf("clearing the mode: %v", err)
	}
}
