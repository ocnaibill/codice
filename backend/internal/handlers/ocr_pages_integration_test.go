package handlers

import (
	"fmt"
	"testing"
)

// What OCR read of the pages of a scanned PDF is kept per page (#24): the table says what a page may be.

func (s *catalogStack) keepPage(file string, page int, state, text string) error {
	_, err := s.db.Exec(`INSERT INTO ocr_pages (file_id, page, state, text, engine) VALUES ($1, $2, $3, $4, 'tesseract')`, file, page, state, text)
	return err
}

func TestOCRPages_APageIsDoneWithTextBlankWithoutOrFailedWithWhy(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Escaneado", "Ana", "e.pdf", "pdf")
	file := s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, work))

	for _, c := range []struct {
		page        int
		state, text string
		ok          bool
	}{
		{0, "done", "um texto", true},
		{1, "blank", "", true},
		{2, "failed", "", true},
		{3, "done", "", false},       // done is text
		{4, "reading", "x", false},   // a state that does not exist
		{-1, "done", "texto", false}, // pages are numbered from 0
	} {
		err := s.keepPage(file, c.page, c.state, c.text)
		if (err == nil) != c.ok {
			t.Errorf("page %d %s %q: err = %v, want ok = %v", c.page, c.state, c.text, err, c.ok)
		}
	}
	if err := s.keepPage(file, 0, "done", "outra vez"); err == nil {
		t.Error("a page is kept once: it is replaced, not repeated")
	}
}

func TestOCRPages_WhatWasReadGoesWithTheFileAndNeedsAnEngine(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Escaneado", "Ana", "e.pdf", "pdf")
	file := s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, work))
	if err := s.keepPage(file, 0, "done", "texto"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO ocr_pages (file_id, page, state, text, engine) VALUES ($1, 1, 'done', 'x', NULL)`, file); err == nil {
		t.Error("a page read by no engine was accepted")
	}
	s.exec(`DELETE FROM files WHERE id = $1`, file)
	if got := s.scalar(`SELECT count(*) FROM ocr_pages`); got != "0" {
		t.Errorf("the pages of a file that is gone: %s", got)
	}
}
