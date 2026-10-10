package handlers

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (s *catalogStack) sendPhoto(a actor, person int, content []byte, credit, license string) *httptest.ResponseRecorder {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if content != nil {
		fw, _ := mw.CreateFormFile("image", "foto")
		fw.Write(content)
	}
	mw.WriteField("credit", credit)
	mw.WriteField("license", license)
	mw.Close()
	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/people/%d/profile/photo", person), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

var jpegBytes = append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, bytes.Repeat([]byte{1}, 600)...)
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{2}, 600)...)

func TestProfileByHand_AuthorNoProviderKnowsGetsAProfileAndNobodyIsToldWhereItCameFrom(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Livro", "Miguel Nicodelis", "a.epub", "epub")
	miguel := s.personID("Miguel Nicodelis")
	if p, _ := s.personPage(ana, miguel); p.Profile != nil {
		t.Fatalf("nothing yet: %+v", p.Profile)
	}
	body := `{"description":"  Escritor   brasileiro ","born":"1980-05","bornPlace":"São Paulo","bio":"Primeiro parágrafo.\r\n\r\nSegundo."}`
	if code := s.setProfile(admin, miguel, body); code != http.StatusNoContent {
		t.Fatalf("write: %d", code)
	}
	p, _ := s.personPage(ana, miguel)
	got := p.Profile
	if got == nil || got.Description != "Escritor brasileiro" || got.Born != "1980-05" || got.BornPlace != "São Paulo" || got.Died != "" {
		t.Fatalf("the profile: %+v", got)
	}
	if got.Bio != "Primeiro parágrafo.\n\nSegundo." || got.BioSource != nil {
		t.Errorf("a biography written has no source: %q %+v", got.Bio, got.BioSource)
	}
	if got.WikidataID != "" || got.Manual {
		t.Errorf("a reader is not told it was written by hand: %+v", got)
	}
	if a, _ := s.personPage(admin, miguel); a.Profile == nil || !a.Profile.Manual {
		t.Errorf("the staff are: %+v", a.Profile)
	}
	if s.scalar(`SELECT manual::text || '|' || place_read::text || '|' || bio_state || '|' || COALESCE(wikidata_id, '-') FROM person_profile WHERE person_id = $1`, miguel) != "true|true|done|-" {
		t.Errorf("the row: %s", s.scalar(`SELECT manual::text || '|' || place_read::text || '|' || bio_state FROM person_profile WHERE person_id = $1`, miguel))
	}
	if s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.profile.write' AND target_id = $1`, fmt.Sprint(miguel)) != "1" {
		t.Errorf("the audit")
	}
	if s.scalar(`SELECT details->>'created' FROM audit_log WHERE action = 'person.profile.write' ORDER BY id DESC LIMIT 1`) != "true" {
		t.Errorf("the audit says it was made")
	}
}

func TestProfileByHand_ChangesWhatWasSentAndLeavesTheRestAndTheSourceOfAnUntouchedBiography(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)
	if code := s.setProfile(admin, frank, `{"description":"Autor de Duna","died":"1986"}`); code != http.StatusNoContent {
		t.Fatalf("%d", code)
	}
	p, _ := s.personPage(admin, frank)
	got := p.Profile
	if got.Description != "Autor de Duna" || got.Died != "1986" || got.Born != "1920-10-08" {
		t.Errorf("what was sent changed and the rest did not: %+v", got)
	}
	if got.Bio != "Frank Herbert foi um escritor." || got.BioSource == nil || got.BioSource.Language != "pt" {
		t.Errorf("Wikipedia's biography keeps its source when it was not touched: %q %+v", got.Bio, got.BioSource)
	}
	if got.Image == nil || got.Image.URL != "/covers/person_Q7934.jpg" || got.WikidataID != "Q7934" || !got.Manual {
		t.Errorf("the photo and the identifier stay, and it is the staff's now: %+v", got)
	}
	// Writing the biography takes it from Wikipedia.
	if code := s.setProfile(admin, frank, `{"bio":"Meu texto."}`); code != http.StatusNoContent {
		t.Fatal(code)
	}
	if p, _ := s.personPage(ana, frank); p.Profile.Bio != "Meu texto." || p.Profile.BioSource != nil {
		t.Errorf("a written biography is not Wikipedia's: %+v", p.Profile)
	}
	// Empty clears.
	if code := s.setProfile(admin, frank, `{"born":"","bornPlace":"","bio":""}`); code != http.StatusNoContent {
		t.Fatal(code)
	}
	if p, _ := s.personPage(ana, frank); p.Profile.Born != "" || p.Profile.BornPlace != "" || p.Profile.Bio != "" || p.Profile.Description != "Autor de Duna" {
		t.Errorf("cleared: %+v", p.Profile)
	}
	// The choice to hide can come with the texts.
	if code := s.setProfile(admin, frank, `{"description":"Outro","hidden":true}`); code != http.StatusNoContent {
		t.Fatal(code)
	}
	if p, _ := s.personPage(ana, frank); p.Profile != nil {
		t.Errorf("hidden for a reader: %+v", p.Profile)
	}
	// The credit of the photo changing drops the links that were of the other credit.
	if code := s.setProfile(admin, frank, `{"imageCredit":"Arquivo da família","imageLicense":"Uso autorizado"}`); code != http.StatusNoContent {
		t.Fatal(code)
	}
	p2, _ := s.personPage(admin, frank)
	if p2.Profile.Image == nil || p2.Profile.Image.Credit != "Arquivo da família" || p2.Profile.Image.License != "Uso autorizado" || p2.Profile.Image.PageURL != "" {
		t.Errorf("the photo: %+v", p2.Profile.Image)
	}
}

func TestProfileByHand_RefusesWhatDoesNotFitAndSavesNothing(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)
	for name, body := range map[string]string{
		"date":        `{"born":"12/11/1962"}`,
		"month":       `{"born":"1962-13"}`,
		"day":         `{"died":"1986-02-30x"}`,
		"description": `{"description":"` + strings.Repeat("a", 501) + `"}`,
		"place":       `{"bornPlace":"` + strings.Repeat("a", 256) + `"}`,
		"bio":         `{"bio":"` + strings.Repeat("a", 6001) + `"}`,
		"credit":      `{"imageCredit":"` + strings.Repeat("a", 301) + `"}`,
	} {
		if code := s.setProfile(admin, frank, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if s.scalar(`SELECT manual::text || description FROM person_profile WHERE person_id = $1`, frank) != "falseescritor de ficção científica americano (1920-1986)" {
		t.Errorf("a refused request changed the profile")
	}
	// A person that is not there is not found, and an accepted date can be before the common era.
	if code := s.setProfile(admin, 999999, `{"description":"x"}`); code != http.StatusNotFound {
		t.Errorf("no such person: %d", code)
	}
	if code := s.setProfile(admin, frank, `{"born":"-0384","died":"-0322-10-07"}`); code != http.StatusNoContent {
		t.Errorf("before the common era: %d", code)
	}
}

func TestProfileByHand_APhotoIsSentAsAFileAndTheOneBeforeIsNotKept(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Livro", "Miguel Nicodelis", "a.epub", "epub")
	miguel := s.personID("Miguel Nicodelis")
	covers := filepath.Join(s.storage, "covers")

	if rec := s.sendPhoto(admin, miguel, jpegBytes, "Foto do autor", "Uso autorizado"); rec.Code != http.StatusOK {
		t.Fatalf("photo: %d %s", rec.Code, rec.Body.String())
	}
	p, _ := s.personPage(ana, miguel)
	if p.Profile == nil || p.Profile.Image == nil || !strings.HasPrefix(p.Profile.Image.URL, fmt.Sprintf("/covers/person_%d_m", miguel)) || !strings.HasSuffix(p.Profile.Image.URL, ".jpg") {
		t.Fatalf("the photo: %+v", p.Profile)
	}
	first := p.Profile.Image.URL
	if p.Profile.Image.Credit != "Foto do autor" || p.Profile.Image.License != "Uso autorizado" {
		t.Errorf("credit: %+v", p.Profile.Image)
	}
	if data, err := os.ReadFile(filepath.Join(covers, filepath.Base(first))); err != nil || !bytes.Equal(data, jpegBytes) {
		t.Errorf("the file was not kept as sent: %v", err)
	}
	// A new photo is shown even when the one before was hidden: it is another photo.
	if code := s.setProfile(admin, miguel, `{"imageHidden":true}`); code != http.StatusNoContent {
		t.Fatalf("hide: %d", code)
	}
	// Another photo replaces it, takes its credit, and the old file goes.
	if rec := s.sendPhoto(admin, miguel, pngBytes, "", ""); rec.Code != http.StatusOK {
		t.Fatalf("second: %d", rec.Code)
	}
	q, _ := s.personPage(ana, miguel)
	if q.Profile.Image.URL == first || !strings.HasSuffix(q.Profile.Image.URL, ".png") || q.Profile.Image.Credit != "" {
		t.Errorf("the second photo: %+v", q.Profile.Image)
	}
	if _, err := os.Stat(filepath.Join(covers, filepath.Base(first))); !os.IsNotExist(err) {
		t.Errorf("the first photo stayed on the disk: %v", err)
	}
	if a, _ := s.personPage(admin, miguel); a.Profile.ImageHidden || a.Profile.Image == nil {
		t.Errorf("the new photo stayed hidden: %+v", a.Profile)
	}
}

func TestProfileByHand_ThePhotoTheWorkerDownloadedIsTheWorkersAndIsNotRemovedWhenAnotherIsSent(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)
	covers := filepath.Join(s.storage, "covers")
	if err := os.MkdirAll(covers, 0o755); err != nil {
		t.Fatal(err)
	}
	workers := filepath.Join(covers, "person_Q7934.jpg")
	if err := os.WriteFile(workers, jpegBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := s.sendPhoto(admin, frank, pngBytes, "Minha", ""); rec.Code != http.StatusOK {
		t.Fatalf("%d", rec.Code)
	}
	if _, err := os.Stat(workers); err != nil {
		t.Errorf("the file of the worker went: %v", err)
	}
	p, _ := s.personPage(ana, frank)
	if p.Profile.Image.URL == "/covers/person_Q7934.jpg" || p.Profile.Image.PageURL != "" || p.Profile.Image.Credit != "Minha" || p.Profile.WikidataID != "Q7934" {
		t.Errorf("the new photo, and nothing of the old credit: %+v", p.Profile)
	}
	// Throwing the profile away does not touch what is the worker's either: the file of the worker is only deleted when it was sent here.
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/admin/people/%d/profile", frank), ""); rec.Code != http.StatusNoContent {
		t.Fatal(rec.Code)
	}
	if _, err := os.Stat(workers); err != nil {
		t.Errorf("the file of the worker went with the profile: %v", err)
	}
}

func TestProfileByHand_RefusesAPhotoThatIsNotOneAndOneThatIsTooBig(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Livro", "Miguel Nicodelis", "a.epub", "epub")
	miguel := s.personID("Miguel Nicodelis")
	for name, content := range map[string][]byte{
		"a gif":      append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 100)...),
		"a document": []byte("%PDF-1.7 not a photo"),
		"a script":   []byte("<script>alert(1)</script>"),
	} {
		if rec := s.sendPhoto(admin, miguel, content, "", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if rec := s.sendPhoto(admin, miguel, nil, "", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no file: %d, want 400", rec.Code)
	}
	if rec := s.sendPhoto(admin, miguel, append(jpegBytes, bytes.Repeat([]byte{3}, 6<<20)...), "", ""); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("too big: %d, want 413", rec.Code)
	}
	if rec := s.sendPhoto(admin, 999999, jpegBytes, "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("no such person: %d, want 404", rec.Code)
	}
	if rec := s.sendPhoto(admin, miguel, jpegBytes, strings.Repeat("a", 301), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a credit that is too long: %d, want 400", rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM person_profile`) != "0" {
		t.Errorf("a refused photo made a profile")
	}
	if entries, _ := os.ReadDir(filepath.Join(s.storage, "covers")); len(entries) != 0 {
		t.Errorf("a refused photo stayed on the disk: %d", len(entries))
	}
}

func TestProfileByHand_CanBeThrownAwayAndTheIdentifierIsReadAgain(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "a.epub", "epub")
	frank := s.personID("Frank Herbert")
	s.giveProfile(frank)
	s.exec(`INSERT INTO person_authority (person_id, scheme, value, source) VALUES ($1, 'wikidata', 'Q7934', 'teste')`, frank)
	s.exec(`INSERT INTO authority_lookups (source, key, state) VALUES ('wikidata', 'Q7934', 'done')`)
	if rec := s.sendPhoto(admin, frank, jpegBytes, "", ""); rec.Code != http.StatusOK {
		t.Fatalf("%d", rec.Code)
	}
	photo := s.scalar(`SELECT image_path FROM person_profile WHERE person_id = $1`, frank)
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/admin/people/%d/profile", frank), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: %d", rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM person_profile WHERE person_id = $1`, frank) != "0" {
		t.Errorf("the profile stayed")
	}
	if s.scalar(`SELECT count(*) FROM authority_lookups WHERE key = 'Q7934'`) != "0" {
		t.Errorf("the lookup is remembered as done, and the worker would not read it again")
	}
	if s.scalar(`SELECT count(*) FROM person_authority WHERE person_id = $1 AND scheme = 'wikidata'`, frank) != "1" {
		t.Errorf("the identifier is what brings the profile back: it stays")
	}
	if _, err := os.Stat(filepath.Join(s.storage, "covers", filepath.Base(photo))); !os.IsNotExist(err) {
		t.Errorf("the photo that was sent stayed: %v", err)
	}
	if rec := s.do(admin, "DELETE", fmt.Sprintf("/admin/people/%d/profile", frank), ""); rec.Code != http.StatusNotFound {
		t.Errorf("twice: %d, want 404", rec.Code)
	}
	if s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'person.profile.reset'`) != "1" {
		t.Errorf("audit")
	}
}
