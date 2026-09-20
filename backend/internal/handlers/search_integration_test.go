package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type segment struct{ text, section, locator string }

// writeGeneration does what the worker does up to the moment of publishing: asks for the next
// generation and writes the segments into it. It returns the generation.
func (s *catalogStack) writeGeneration(file int64, segs ...segment) int {
	s.t.Helper()
	var gen int
	if err := s.db.QueryRow(`SELECT text_extraction_begin($1)`, file).Scan(&gen); err != nil {
		s.t.Fatal(err)
	}
	for i, sg := range segs {
		loc := sg.locator
		if loc == "" {
			loc = fmt.Sprintf(`{"type":"pdf","page":%d}`, i)
		}
		s.exec(`INSERT INTO document_segments (file_id, generation, sequence, section, text, locator, locator_version)
		        VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6::jsonb, 1)`, file, gen, i, sg.section, sg.text, loc)
	}
	return gen
}

func (s *catalogStack) publish(file int64, gen int, status string) error {
	_, err := s.db.Exec(`SELECT text_extraction_publish($1, $2, 1, 'abc', $3, 'native', 'pt')`, file, gen, status)
	return err
}

func (s *catalogStack) index(file int64, segs ...segment) {
	s.t.Helper()
	if err := s.publish(file, s.writeGeneration(file, segs...), "ready"); err != nil {
		s.t.Fatal(err)
	}
}

type searchResult struct {
	Data    []SearchHit `json:"data"`
	HasMore bool        `json:"hasMore"`
}

func (s *catalogStack) search(a actor, query string) (int, searchResult) {
	s.t.Helper()
	rec := s.do(a, "GET", "/search?"+query, "")
	var out searchResult
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func q(text string) string { return "q=" + url.QueryEscape(text) }

func TestTextExtraction_APublicationIsOneStepAndTheOlderTextStaysUntilThen(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()

	// A first extraction that is written but not published is not searchable.
	gen := s.writeGeneration(epub, segment{text: "primeira versão do texto sobre Constantinopla"})
	if gen != 1 {
		t.Fatalf("first generation = %d", gen)
	}
	if _, r := s.search(ana, q("constantinopla")); len(r.Data) != 0 {
		t.Fatalf("an unpublished generation must not be found: %+v", r.Data)
	}
	if err := s.publish(epub, gen, "ready"); err != nil {
		t.Fatal(err)
	}
	if _, r := s.search(ana, q("constantinopla")); len(r.Data) != 1 {
		t.Fatalf("published: %+v", r.Data)
	}
	if got := s.scalar(`SELECT status || ':' || generation || ':' || segment_count || ':' || char_count FROM text_extractions WHERE file_id = $1`, epub); got != "ready:1:1:45" {
		t.Errorf("what the extraction says about itself: %s", got)
	}

	// A second one is written next to it. Until it is published the first is what is found.
	gen2 := s.writeGeneration(epub, segment{text: "segunda versão, agora sobre Bizâncio"})
	if gen2 != 2 {
		t.Fatalf("second generation = %d", gen2)
	}
	if _, r := s.search(ana, q("bizancio")); len(r.Data) != 0 {
		t.Errorf("the new text is not visible before it is published: %+v", r.Data)
	}
	if _, r := s.search(ana, q("constantinopla")); len(r.Data) != 1 {
		t.Errorf("the old text stays until the new one replaces it: %+v", r.Data)
	}
	if err := s.publish(epub, gen2, "ready"); err != nil {
		t.Fatal(err)
	}
	if _, r := s.search(ana, q("constantinopla")); len(r.Data) != 0 {
		t.Errorf("the old text is gone once replaced: %+v", r.Data)
	}
	if _, r := s.search(ana, q("bizancio")); len(r.Data) != 1 {
		t.Errorf("the new text: %+v", r.Data)
	}
	if got := s.scalar(`SELECT count(*) FROM document_segments WHERE file_id = $1`, epub); got != "1" {
		t.Errorf("older generations are deleted: %s rows", got)
	}
}

func TestTextExtraction_ADeadWorkerLeavesNothingSearchableAndTheNextOneStartsClean(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()
	s.index(epub, segment{text: "texto publicado sobre Alexandria"})

	// The worker writes half of a new generation and dies.
	s.writeGeneration(epub, segment{text: "meio texto novo sobre Cartago"})
	if _, r := s.search(ana, q("cartago")); len(r.Data) != 0 {
		t.Errorf("half of an extraction is not searchable: %+v", r.Data)
	}
	// The next attempt asks for a generation: it gets the same number, and the leftovers are gone.
	var gen int
	s.db.QueryRow(`SELECT text_extraction_begin($1)`, epub).Scan(&gen)
	if gen != 2 {
		t.Errorf("the next attempt writes generation 2 again, got %d", gen)
	}
	if got := s.scalar(`SELECT count(*) FROM document_segments WHERE file_id = $1 AND generation = 2`, epub); got != "0" {
		t.Errorf("leftovers of the dead attempt: %s", got)
	}
	if _, r := s.search(ana, q("alexandria")); len(r.Data) != 1 {
		t.Errorf("the published text was untouched: %+v", r.Data)
	}
}

func TestTextExtraction_OnlyTheNextGenerationCanBePublishedAndReadyNeedsText(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, _ := s.bookWithTwoFiles()

	gen := s.writeGeneration(epub, segment{text: "algum texto"})
	if err := s.publish(epub, gen+1, "ready"); err == nil {
		t.Error("a generation that is not the next one must be refused")
	}
	if err := s.publish(epub, gen, "ready"); err != nil {
		t.Fatal(err)
	}
	// Ready with nothing written is refused.
	gen2 := s.writeGeneration(epub)
	if err := s.publish(epub, gen2, "ready"); err == nil {
		t.Error("ready needs segments")
	}
	// Empty and unsupported have none, and any written are dropped: the older text goes with them.
	s.writeGeneration(epub, segment{text: "isto não deve ficar"})
	if err := s.publish(epub, gen2, "empty"); err != nil {
		t.Fatal(err)
	}
	if got := s.scalar(`SELECT status || ':' || segment_count FROM text_extractions WHERE file_id = $1`, epub); got != "empty:0" {
		t.Errorf("a scan: %s", got)
	}
	if got := s.scalar(`SELECT count(*) FROM document_segments WHERE file_id = $1`, epub); got != "0" {
		t.Errorf("no text left: %s", got)
	}
}

func TestTextExtraction_AFailureKeepsWhatWasPublished(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: "texto que já estava publicado"})

	s.exec(`SELECT text_extraction_fail($1, 1, 'abc', 'ZipError: bad archive')`, epub)
	if got := s.scalar(`SELECT status || ':' || generation || ':' || error FROM text_extractions WHERE file_id = $1`, epub); got != "ready:1:ZipError: bad archive" {
		t.Errorf("a later failure is noted, and the text stays: %s", got)
	}
	if _, r := s.search(ana, q("publicado")); len(r.Data) != 1 {
		t.Errorf("still searchable: %+v", r.Data)
	}
	// A file that never had text and failed says so.
	s.exec(`SELECT text_extraction_fail($1, 1, NULL, 'corrupt')`, pdf)
	if got := s.scalar(`SELECT status || ':' || generation FROM text_extractions WHERE file_id = $1`, pdf); got != "failed:0" {
		t.Errorf("never published: %s", got)
	}
	// A success afterwards clears the error.
	s.index(pdf, segment{text: "agora leu"})
	if got := s.scalar(`SELECT COALESCE(error, '-') FROM text_extractions WHERE file_id = $1`, pdf); got != "-" {
		t.Errorf("the error goes with the success: %s", got)
	}
}

func TestSearch_IgnoresCaseAndAccentsAndShowsTheTextAsItWas(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, _ := s.bookWithTwoFiles()
	s.index(epub,
		segment{text: "Nada acontece sem AÇÃO. A ação e a reação são iguais.", section: "Capítulo 1", locator: `{"type":"epub","href":"c1.xhtml","progression":0.25}`},
		segment{text: "Um trecho sem relação alguma com o assunto."},
	)

	for _, query := range []string{"acao", "AÇÃO", "Ação", "aCaO"} {
		code, r := s.search(ana, q(query))
		if code != 200 || len(r.Data) != 1 {
			t.Fatalf("%q: %d %+v", query, code, r.Data)
		}
	}
	_, r := s.search(ana, q("acao"))
	hit := r.Data[0]
	if hit.WorkID != work || hit.FileID != epub || hit.Format != "epub" || hit.WorkTitle != "Duna" || hit.WorkAuthor != "Frank Herbert" ||
		hit.Section != "Capítulo 1" || hit.Origin != "native" || hit.LocatorVersion != 1 {
		t.Errorf("where the hit is: %+v", hit)
	}
	// The locator is the address inside the file, ready to open it there.
	var loc map[string]any
	json.Unmarshal(hit.Locator, &loc)
	if loc["href"] != "c1.xhtml" || loc["type"] != "epub" {
		t.Errorf("locator = %s", hit.Locator)
	}
	// The snippet is the text as extracted, accents and all, and the matches point at the words in it.
	if !strings.Contains(hit.Snippet, "AÇÃO") || strings.ContainsAny(hit.Snippet, "\x02\x03") {
		t.Errorf("snippet = %q", hit.Snippet)
	}
	if len(hit.Matches) != 2 {
		t.Fatalf("two words match: %v in %q", hit.Matches, hit.Snippet)
	}
	runes := []rune(hit.Snippet)
	found := []string{string(runes[hit.Matches[0][0]:hit.Matches[0][1]]), string(runes[hit.Matches[1][0]:hit.Matches[1][1]])}
	if found[0] != "AÇÃO" || found[1] != "ação" {
		t.Errorf("the words marked are the ones in the text, with their accents: %q", found)
	}
}

func TestSearch_UnderstandsPhrasesExclusionsAndAlternatives(t *testing.T) {
	s := newCatalogStack(t)
	_, epub, pdf := s.bookWithTwoFiles()
	s.index(epub,
		segment{text: "O rei morreu e o povo chorou na praça."},
		segment{text: "O povo riu na praça, e o rei sorriu."},
	)
	s.index(pdf, segment{text: "A rainha governou o povo por muitos anos."})

	count := func(query string) int { _, r := s.search(ana, q(query)); return len(r.Data) }
	cases := map[string]int{
		`povo`:                 3,
		`rei povo`:             2, // all words
		`"o povo chorou"`:      1, // a phrase
		`"povo chorou o"`:      0, // in that order
		`povo -rei`:            1, // excluding one
		`rei OR rainha`:        3, // either
		`inexistente`:          0,
		`o`:                    3, // no word is dropped as too common: the search is not language aware
		`!!! ???`:              0, // no words at all
		`chorou   PRAÇA  POVO`: 1, // spaces and order do not matter outside a phrase
	}
	for query, want := range cases {
		if got := count(query); got != want {
			t.Errorf("%q: %d hits, want %d", query, got, want)
		}
	}
}

func TestSearch_OnlyWhatCanStillBeOpened(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	s.index(epub, segment{text: "passagem sobre o labirinto"})
	s.index(pdf, segment{text: "outra passagem sobre o labirinto"})
	count := func() int { _, r := s.search(ana, q("labirinto")); return len(r.Data) }

	if count() != 2 {
		t.Fatalf("both files: %d", count())
	}
	// A file that is gone from the disk cannot be opened: a hit on it would lead nowhere.
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, pdf)
	if count() != 1 {
		t.Errorf("a missing file is not offered: %d", count())
	}
	s.exec(`UPDATE files SET availability = 'available' WHERE id = $1`, pdf)
	// A retired work is out of the library.
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	if count() != 0 {
		t.Errorf("a retired work is not searched: %d", count())
	}
	s.exec(`UPDATE works SET retired_at = NULL WHERE id = $1`, work)
	// And the text goes with the file.
	s.exec(`DELETE FROM files WHERE id = $1`, pdf)
	if count() != 1 {
		t.Errorf("a deleted file takes its text with it: %d", count())
	}
	if got := s.scalar(`SELECT count(*) FROM text_extractions WHERE file_id = $1`, pdf); got != "0" {
		t.Errorf("and its extraction record: %s", got)
	}
}

func TestSearch_FiltersPagesAndRefusesWhatItCannotRead(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	other := s.addWork("Outro", "X", "outro.epub", "epub")
	otherFile := s.primaryFile(other)
	var segs []segment
	for i := 0; i < 5; i++ {
		segs = append(segs, segment{text: fmt.Sprintf("dragão número %d na caverna", i)})
	}
	s.index(epub, segs...)
	s.index(pdf, segment{text: "um dragão no PDF"})
	s.index(otherFile, segment{text: "o dragão de outra obra"})

	if _, r := s.search(ana, q("dragao")+"&workId="+fmt.Sprint(work)); len(r.Data) != 6 {
		t.Errorf("one work: %d", len(r.Data))
	}
	if _, r := s.search(ana, q("dragao")+"&fileId="+fmt.Sprint(pdf)); len(r.Data) != 1 || r.Data[0].Format != "pdf" {
		t.Errorf("one file: %+v", r.Data)
	}
	// Paging says whether there is more, without a count of everything.
	_, first := s.search(ana, q("dragao")+"&limit=3")
	if len(first.Data) != 3 || !first.HasMore {
		t.Errorf("first page: %d more=%v", len(first.Data), first.HasMore)
	}
	_, last := s.search(ana, q("dragao")+"&limit=3&offset=6")
	if len(last.Data) != 1 || last.HasMore {
		t.Errorf("last page: %d more=%v", len(last.Data), last.HasMore)
	}
	seen := map[int64]bool{}
	for _, off := range []string{"0", "3", "6"} {
		_, page := s.search(ana, q("dragao")+"&limit=3&offset="+off)
		for _, h := range page.Data {
			if seen[h.SegmentID] {
				t.Errorf("segment %d on two pages", h.SegmentID)
			}
			seen[h.SegmentID] = true
		}
	}
	if len(seen) != 7 {
		t.Errorf("all seven, once each: %d", len(seen))
	}

	for name, query := range map[string]string{
		"nothing to search for": "q=",
		"only spaces":           "q=%20%20",
		"no q":                  "workId=1",
		"too long":              q(strings.Repeat("a", 201)),
		"a workId that is not":  q("x") + "&workId=abc",
		"a negative fileId":     q("x") + "&fileId=-1",
	} {
		if code, _ := s.search(ana, query); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
}

func TestSearch_TheDetailSaysWhatBecameOfEachFilesText(t *testing.T) {
	s := newCatalogStack(t)
	work, epub, pdf := s.bookWithTwoFiles()
	status := func() map[int64]string {
		w, _ := s.detail(ana, work)
		out := map[int64]string{}
		for _, e := range w.Editions {
			for _, f := range e.Files {
				out[f.ID] = fmt.Sprintf("%s/%d", f.TextStatus, f.TextSegments)
			}
		}
		return out
	}
	if got := status(); got[epub] != "/0" || got[pdf] != "/0" {
		t.Errorf("nothing looked at yet says nothing: %v", got)
	}
	s.index(epub, segment{text: "a"}, segment{text: "b"}, segment{text: "c"})
	if err := s.publish(pdf, s.writeGeneration(pdf), "empty"); err != nil {
		t.Fatal(err)
	}
	if got := status(); got[epub] != "ready/3" || got[pdf] != "empty/0" {
		t.Errorf("text status = %v", got)
	}
}

func TestExtractText_ReprocessingIsAStaffRequestForOneJob(t *testing.T) {
	s := newCatalogStack(t)
	work, _, _ := s.bookWithTwoFiles()
	url := fmt.Sprintf("/admin/works/%d/extract-text", work)

	rec := s.do(admin, "POST", url, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("queued: %d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT type || ':' || (payload->>'force') || ':' || priority FROM jobs WHERE work_id = $1 AND type = 'extract_text' AND state = 'pending' AND created_by IS NOT NULL`, work); got != "extract_text:true:5" {
		t.Errorf("the job: %q", got)
	}
	// Asking again while it waits is the same request.
	if rec := s.do(admin, "POST", url, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "alreadyQueued") {
		t.Errorf("asking twice: %d %s", rec.Code, rec.Body)
	}
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE work_id = $1 AND type = 'extract_text' AND state = 'pending'`, work); got != "1" {
		t.Errorf("one job: %s", got)
	}
	if rec := s.do(admin, "POST", "/admin/works/999999/extract-text", ""); rec.Code != http.StatusNotFound {
		t.Errorf("no such work: %d", rec.Code)
	}
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, work)
	s.exec(`DELETE FROM jobs WHERE type = 'extract_text'`)
	if rec := s.do(admin, "POST", url, ""); rec.Code != http.StatusNotFound {
		t.Errorf("a retired work: %d", rec.Code)
	}
}

func TestExtractText_IsQueuedByTheQueueAfterAnIngestAndBehindWhatPeopleAskedFor(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Novo", "X", "novo.epub", "epub")
	s.exec(`DELETE FROM jobs`)

	s.exec(`INSERT INTO jobs (type, work_id, payload, priority) VALUES ('ingest', $1, '{}', 5)`, work)
	if got := s.scalar(`SELECT count(*) FROM jobs WHERE type = 'extract_text'`); got != "0" {
		t.Fatalf("nothing is extracted before the file is analysed: %s", got)
	}
	s.exec(`UPDATE jobs SET state = 'succeeded' WHERE type = 'ingest' AND work_id = $1`, work)
	if got := s.scalar(`SELECT state || ':' || priority FROM jobs WHERE type = 'extract_text' AND work_id = $1`, work); got != "pending:-10" {
		t.Errorf("extraction is queued behind the rest: %q", got)
	}
	// The other follow-ups are still made.
	if got := s.scalar(`SELECT string_agg(type, ',' ORDER BY type) FROM jobs WHERE work_id = $1 AND type <> 'ingest'`, work); got != "dedupe,extract_text,organize" {
		t.Errorf("follow-ups: %s", got)
	}
}

func TestExtractText_TheMigrationQueuesTheFilesAlreadyThere(t *testing.T) {
	s := newCatalogStack(t)
	work := s.addWork("Antigo", "X", "antigo.epub", "epub")
	retired := s.addWork("Aposentado", "X", "aposentado.epub", "epub")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retired)
	s.exec(`DELETE FROM jobs`)

	// The statement of the migration, as it is: the works of the library, and no others, once each.
	backfill := `INSERT INTO jobs (type, work_id, payload, priority)
		SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
		FROM editions e JOIN files f ON f.edition_id = e.id
		JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
		ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING`
	s.exec(backfill)
	s.exec(backfill)
	if got := s.scalar(`SELECT string_agg(work_id::text, ',') FROM jobs WHERE type = 'extract_text'`); got != fmt.Sprint(work) {
		t.Errorf("queued for %q, want only the work in the library", got)
	}
}
