package handlers

import (
	"fmt"
	"strings"
	"testing"
)

// The language a scan is read in, and the staff saying the automatic choice was wrong (#24).

func (s *catalogStack) setOCRLanguage(a actor, file, body string) (int, string) {
	s.t.Helper()
	rec := s.do(a, "POST", fmt.Sprintf("/admin/files/%s/ocr/language", file), body)
	return rec.Code, rec.Body.String()
}

// ocrOn turns OCR on with an engine that has English, Portuguese and Spanish.
func (s *catalogStack) ocrOn() {
	s.t.Helper()
	s.ocrWorker(`["eng","por","spa"]`)
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled": true, "language": "por+eng"}')`)
}

func TestOCRList_SaysTheLanguageTheFileIsReadInAndHowItWasChosen(t *testing.T) {
	s := newCatalogStack(t)
	_, file := s.scanned("Escaneado", 1, 2)
	if it := s.ocrList()[0]; it.Language != "" || it.LanguageSource != "" {
		t.Errorf("not looked at yet: %+v", it)
	}
	s.exec(`INSERT INTO ocr_files (file_id, source_sha256, language, source) VALUES ($1, $2, 'eng', 'detected')`, file, shaA)
	if it := s.ocrList()[0]; it.Language != "eng" || it.LanguageSource != "detected" {
		t.Errorf("%+v", it)
	}
	// What was decided of another version of the file is not of this one.
	s.exec(`UPDATE ocr_files SET source_sha256 = $1`, shaB)
	if it := s.ocrList()[0]; it.Language != "" || it.LanguageSource != "" {
		t.Errorf("another version: %+v", it)
	}
}

func TestOCRSetLanguage_TheStaffCorrectsTheLanguageAndTheFileIsReadAgainFromTheStart(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrOn()
	work, file := s.scanned("Escaneado", 1, 2, 3)
	s.exec(`INSERT INTO ocr_files (file_id, source_sha256, language, source) VALUES ($1, $2, 'por', 'detected')`, file, shaA)
	s.readPage(file, 0, "done", shaA, "por")
	s.readPage(file, 1, "failed", shaA, "por")
	other, otherFile := s.scanned("Outro", 1)
	s.readPage(otherFile, 0, "done", s.scalar(fmt.Sprintf(`SELECT sha256 FROM files WHERE id = %s`, otherFile)), "por")

	code, body := s.setOCRLanguage(admin, file, `{"language":" eng "}`)
	if code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	if got := s.scalar(fmt.Sprintf(`SELECT language || ':' || source || ':' || source_sha256 FROM ocr_files WHERE file_id = %s`, file)); got != "eng:manual:"+shaA {
		t.Errorf("the decision: %s", got)
	}
	if got := s.scalar(fmt.Sprintf(`SELECT count(*) FROM ocr_pages WHERE file_id = %s`, file)); got != "0" {
		t.Errorf("the pages read in the wrong language are kept: %s", got)
	}
	if got := s.scalar(fmt.Sprintf(`SELECT count(*) FROM ocr_pages WHERE file_id = %s`, otherFile)); got != "1" {
		t.Errorf("the pages of another file were dropped: %s", got)
	}
	if got := s.scalar(fmt.Sprintf(`SELECT type || ':' || priority || ':' || payload::text || ':' || state FROM jobs WHERE work_id = %d`, work)); got != "ocr:5:{}:pending" {
		t.Errorf("the job (it reads every page, not only the ones that failed): %s", got)
	}
	if got := s.scalar(fmt.Sprintf(`SELECT count(*) FROM jobs WHERE work_id = %d`, other)); got != "0" {
		t.Errorf("a job for another work: %s", got)
	}
	if !strings.Contains(body, `"language":"eng"`) || !strings.Contains(body, `"queued":true`) {
		t.Errorf("the answer: %s", body)
	}
	if got := s.scalar(`SELECT (details->>'previous') || ':' || (details->>'language') FROM audit_log WHERE action = 'ocr.language'`); got != "por:eng" {
		t.Errorf("audit: %s", got)
	}
	it := s.ocrList()[0]
	if it.Language != "eng" || it.LanguageSource != "manual" || it.State != "queued" {
		t.Errorf("the list: %+v", it)
	}
}

func TestOCRSetLanguage_AFileNeverLookedAtGetsItsFirstDecision(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrOn()
	_, file := s.scanned("Escaneado", 1)
	if code, body := s.setOCRLanguage(admin, file, `{"language":"por+eng"}`); code != 202 {
		t.Fatalf("%d %s", code, body)
	}
	if got := s.scalar(`SELECT language || ':' || source FROM ocr_files`); got != "por+eng:manual" {
		t.Errorf("%s", got)
	}
	if got := s.scalar(`SELECT details->>'previous' FROM audit_log WHERE action = 'ocr.language'`); got != "" {
		t.Errorf("previous: %q", got)
	}
}

func TestOCRSetLanguage_NotWhileTheWorkIsInTheQueueOrBeingRead(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrOn()
	work, file := s.scanned("Escaneado", 1)
	s.readPage(file, 0, "done", shaA, "por")
	for _, state := range []string{"pending", "running"} {
		s.exec(`DELETE FROM jobs`)
		s.exec(`INSERT INTO jobs (type, work_id, state) VALUES ('ocr', $1, $2)`, work, state)
		if code, body := s.setOCRLanguage(admin, file, `{"language":"eng"}`); code != 409 || !strings.Contains(body, "fila") {
			t.Errorf("%s: %d %s", state, code, body)
		}
	}
	// Refused as a whole: nothing was decided, nothing was dropped.
	if s.scalar(`SELECT count(*) FROM ocr_files`) != "0" || s.scalar(`SELECT count(*) FROM ocr_pages`) != "1" {
		t.Error("a refused request changed what was kept")
	}
	// A job that ended is no obstacle.
	s.exec(`DELETE FROM jobs`)
	s.exec(`INSERT INTO jobs (type, work_id, state) VALUES ('ocr', $1, 'failed'), ('extract_text', $1, 'running')`, work)
	if code, body := s.setOCRLanguage(admin, file, `{"language":"eng"}`); code != 202 {
		t.Errorf("%d %s", code, body)
	}
}

func TestOCRSetLanguage_OnlyWithOCROnAndALanguageTheEngineHas(t *testing.T) {
	s := newCatalogStack(t)
	_, file := s.scanned("Escaneado", 1)
	if code, body := s.setOCRLanguage(admin, file, `{"language":"eng"}`); code != 409 || !strings.Contains(body, "desligado") {
		t.Errorf("off: %d %s", code, body)
	}
	s.ocrOn()
	if code, body := s.setOCRLanguage(admin, file, `{"language":"por+deu"}`); code != 400 || !strings.Contains(body, "deu") {
		t.Errorf("a language the engine lacks: %d %s", code, body)
	}
	for _, bad := range []string{`{"language":""}`, `{"language":"Português"}`, `{"language":"por+"}`, `{}`, `nope`} {
		if code, _ := s.setOCRLanguage(admin, file, bad); code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}
	if s.scalar(`SELECT count(*) FROM jobs`) != "0" || s.scalar(`SELECT count(*) FROM ocr_files`) != "0" {
		t.Error("a refused request queued or decided something")
	}
}

func TestOCRSetLanguage_OnlyForFilesThatAreScansOfWorksThatAreThere(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrOn()
	work, file := s.scanned("Escaneado", 1)
	digital := s.addWork("Digital", "Ana", "d.pdf", "pdf")
	digitalFile := s.scalar(fmt.Sprintf(`SELECT file_id FROM work_primary WHERE work_id = %d`, digital))
	s.exec(`INSERT INTO text_layers (file_id, page_count, pages_without_text, needs_ocr) VALUES ($1, 5, '{}', FALSE)`, digitalFile)
	for name, id := range map[string]string{"missing": "99999", "not a number": "abc", "not a scan": digitalFile} {
		if code, _ := s.setOCRLanguage(admin, id, `{"language":"eng"}`); code != 404 {
			t.Errorf("%s: %d", name, code)
		}
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if code, _ := s.setOCRLanguage(admin, file, `{"language":"eng"}`); code != 404 {
		t.Errorf("a retired work: %d", code)
	}
	if s.scalar(`SELECT count(*) FROM jobs`) != "0" {
		t.Error("a job was queued")
	}
}

func TestOCRSetLanguage_WithTheEngineStoppedTheLanguageIsTakenAsItComes(t *testing.T) {
	s := newCatalogStack(t)
	_, file := s.scanned("Escaneado", 1)
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled": true}')`)
	// No word from the engine: there is no list to check against, and the worker checks when it reads.
	if code, body := s.setOCRLanguage(admin, file, `{"language":"deu"}`); code != 202 {
		t.Errorf("%d %s", code, body)
	}
}
