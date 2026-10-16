package handlers

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

type atomLink struct {
	Rel    string `xml:"rel,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr"`
	Length string `xml:"length,attr"`
}

type atomFeed struct {
	Links   []atomLink `xml:"link"`
	Total   int        `xml:"totalResults"`
	PerPage int        `xml:"itemsPerPage"`
	Start   int        `xml:"startIndex"`
	Entries []struct {
		Title   string     `xml:"title"`
		Content string     `xml:"content"`
		Links   []atomLink `xml:"link"`
	} `xml:"entry"`
}

func (f atomFeed) link(rel string) (atomLink, bool) {
	for _, l := range f.Links {
		if l.Rel == rel {
			return l, true
		}
	}
	return atomLink{}, false
}

// opdsFeed reads a feed as a client does: it must be well formed, and every prefix it uses must be declared (a feed that uses one it did not
// declare is not well formed XML with namespaces, and a strict client refuses it, which is what the search feed did).
func (s *catalogStack) opdsFeed(path string) (atomFeed, string) {
	s.t.Helper()
	rec := s.do(ana, "GET", path, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var feed atomFeed
	if err := xml.NewDecoder(strings.NewReader(body)).Decode(&feed); err != nil {
		s.t.Fatalf("GET %s is not XML: %v\n%s", path, err, body)
	}
	for _, m := range regexp.MustCompile(`</?([A-Za-z][\w]*):[\w]+`).FindAllStringSubmatch(body, -1) {
		if !strings.Contains(body, "xmlns:"+m[1]+"=") {
			s.t.Fatalf("GET %s uses the prefix %q without declaring it:\n%s", path, m[1], body)
		}
	}
	return feed, body
}

func TestOPDS_EveryFeedIsWellFormedAndDeclaresWhatItUses(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna & os \"Filhos\" <de> Arrakis", "Frank Herbert", "dune.epub", "epub")
	for _, path := range []string{"/opds/catalog", "/opds/recent", "/opds/search?q=duna", "/opds/search?q=nada-disso"} {
		s.opdsFeed(path)
	}
	feed, body := s.opdsFeed("/opds/search?q=duna")
	if feed.Total != 1 || len(feed.Entries) != 1 || feed.Entries[0].Title != `Duna & os "Filhos" <de> Arrakis` {
		t.Errorf("search = %+v\n%s", feed, body)
	}
	if empty, _ := s.opdsFeed("/opds/search?q=nada-disso"); empty.Total != 0 || len(empty.Entries) != 0 {
		t.Errorf("a search with no result: %+v", empty)
	}
}

func TestOPDS_AFeedIsWalkedPageByPage(t *testing.T) {
	s := newCatalogStack(t)
	for i := 1; i <= 65; i++ {
		s.addWork(fmt.Sprintf("Livro %02d", i), "Autor", fmt.Sprintf("l%02d.epub", i), "epub")
	}
	first, _ := s.opdsFeed("/opds/recent")
	if first.Total != 65 || first.PerPage != 30 || first.Start != 1 || len(first.Entries) != 30 {
		t.Fatalf("page 1: total %d, per page %d, start %d, entries %d", first.Total, first.PerPage, first.Start, len(first.Entries))
	}
	if first.Entries[0].Title != "Livro 65" {
		t.Errorf("the newest comes first: %q", first.Entries[0].Title)
	}
	if _, ok := first.link("previous"); ok {
		t.Error("the first page has a previous one")
	}
	next, ok := first.link("next")
	if !ok || !strings.HasSuffix(next.Href, "/opds/v1.2/recent?page=2") {
		t.Fatalf("next = %+v", next)
	}
	third, _ := s.opdsFeed("/opds/recent?page=3")
	if third.Start != 61 || len(third.Entries) != 5 || third.Entries[4].Title != "Livro 01" {
		t.Errorf("page 3: start %d, entries %d", third.Start, len(third.Entries))
	}
	if _, ok := third.link("next"); ok {
		t.Error("the last page has a next one")
	}
	if prev, ok := third.link("previous"); !ok || !strings.HasSuffix(prev.Href, "?page=2") {
		t.Errorf("previous = %+v", prev)
	}
	if last, _ := first.link("last"); !strings.HasSuffix(last.Href, "?page=3") {
		t.Errorf("last = %+v", last)
	}
	// a nonsense page is the first, and one past the end is empty but still a feed
	for _, path := range []string{"/opds/recent?page=0", "/opds/recent?page=abc", "/opds/recent?page=-4"} {
		if f, _ := s.opdsFeed(path); f.Start != 1 || len(f.Entries) != 30 {
			t.Errorf("%s: start %d, entries %d", path, f.Start, len(f.Entries))
		}
	}
	if f, _ := s.opdsFeed("/opds/recent?page=9"); len(f.Entries) != 0 || f.Total != 65 {
		t.Errorf("past the end: %d entries, total %d", len(f.Entries), f.Total)
	}
	// the search keeps its term on every page it links to
	searched, _ := s.opdsFeed("/opds/search?q=livro")
	if n, ok := searched.link("next"); !ok || !strings.Contains(n.Href, "q=livro") || !strings.Contains(n.Href, "page=2") {
		t.Errorf("search next = %+v", n)
	}
}

func TestOPDS_AWorkOffersEveryFileItHasWithTheRightTypeAndTheEpubFirst(t *testing.T) {
	s := newCatalogStack(t)
	id := s.addWork("Duna", "Frank Herbert", "dune.pdf", "pdf") // the primary file is the PDF
	var edition int
	s.db.QueryRow(`SELECT edition_id FROM work_primary WHERE work_id = $1`, id).Scan(&edition)
	epub := s.addFile(edition, "epub", "dune.epub", "managed")
	audio := s.addFile(edition, "m4b", "dune.m4b", "managed")
	missing := s.addFile(edition, "cbz", "gone.cbz", "managed")
	s.exec(`UPDATE files SET availability = 'missing' WHERE id = $1`, missing)
	s.exec(`UPDATE files SET size_bytes = 1234 WHERE id = $1`, epub)
	s.exec(`UPDATE works SET description = 'Um planeta de areia.' WHERE id = $1`, id)
	s.exec(`UPDATE editions SET language = 'pt-BR' WHERE id = $1`, edition)
	_ = audio

	feed, body := s.opdsFeed("/opds/recent")
	if len(feed.Entries) != 1 {
		t.Fatalf("entries = %d", len(feed.Entries))
	}
	var got []string
	for _, l := range feed.Entries[0].Links {
		if l.Rel == "http://opds-spec.org/acquisition" {
			got = append(got, l.Type)
		}
	}
	if strings.Join(got, ",") != "application/epub+zip,application/pdf,audio/mp4" {
		t.Errorf("acquisition types = %v (a missing file is not offered, and the EPUB goes first)\n%s", got, body)
	}
	for _, l := range feed.Entries[0].Links {
		if l.Type == "application/epub+zip" && l.Length != "1234" {
			t.Errorf("the size of the EPUB: %q", l.Length)
		}
	}
	if feed.Entries[0].Content != "Um planeta de areia." || !strings.Contains(body, "<dc:language>pt-BR</dc:language>") {
		t.Errorf("summary and language are missing:\n%s", body)
	}
}

func TestOPDS_ACoverIsToldWhatItIsAndAWorkWithNoFileHasNoDownload(t *testing.T) {
	s := newCatalogStack(t)
	jpg := s.addWork("Com capa", "A", "a.epub", "epub")
	s.addWork("Sem capa", "B", "b.epub", "epub")
	noFile := s.addWork("Sem arquivo", "C", "c.epub", "epub")
	s.exec(`UPDATE editions SET cover_url = '/covers/a.png' WHERE work_id = $1 AND is_primary`, jpg)
	s.exec(`UPDATE files SET availability = 'missing' WHERE edition_id IN (SELECT id FROM editions WHERE work_id = $1)`, noFile)

	feed, _ := s.opdsFeed("/opds/recent")
	types := map[string]string{}
	acquisitions := map[string]int{}
	for _, e := range feed.Entries {
		for _, l := range e.Links {
			switch l.Rel {
			case "http://opds-spec.org/image":
				types[e.Title] = l.Type
			case "http://opds-spec.org/acquisition":
				acquisitions[e.Title]++
			}
		}
	}
	if types["Com capa"] != "image/png" || types["Sem capa"] != "image/svg+xml" {
		t.Errorf("cover types = %v", types)
	}
	if acquisitions["Sem arquivo"] != 0 || acquisitions["Com capa"] != 1 {
		t.Errorf("downloads = %v", acquisitions)
	}
}

// titles are what a feed shows, in its order: its books, or its sections.
func entryTitles(f atomFeed) []string {
	var out []string
	for _, e := range f.Entries {
		out = append(out, e.Title)
	}
	return out
}

func (s *catalogStack) opdsFeedAs(a actor, path string) atomFeed {
	s.t.Helper()
	rec := s.do(a, "GET", path, "")
	if rec.Code != 200 {
		s.t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	var feed atomFeed
	if err := xml.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&feed); err != nil {
		s.t.Fatalf("GET %s is not XML: %v", path, err)
	}
	return feed
}

func TestOPDS_TheRootLeadsToEverySectionOfTheCatalog(t *testing.T) {
	s := newCatalogStack(t)
	feed, _ := s.opdsFeed("/opds/catalog")
	want := []string{"Continuar lendo", "Adicionados recentemente", "Favoritos", "Minhas listas", "Séries", "Autores", "Categorias"}
	if got := entryTitles(feed); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("sections = %v, want %v", got, want)
	}
	if l, ok := feed.link("search"); !ok || !strings.Contains(l.Href, "{searchTerms}") {
		t.Errorf("search link = %+v", l)
	}
	for _, e := range feed.Entries {
		if len(e.Links) != 1 || e.Links[0].Rel != "subsection" || !strings.Contains(e.Links[0].Href, "/opds/v1.2/") {
			t.Errorf("%s leads nowhere: %+v", e.Title, e.Links)
		}
	}
	for path, kind := range map[string]string{"Minhas listas": "navigation", "Séries": "navigation", "Autores": "navigation", "Categorias": "navigation", "Favoritos": "acquisition"} {
		for _, e := range feed.Entries {
			if e.Title == path && !strings.Contains(e.Links[0].Type, "kind="+kind) {
				t.Errorf("%s is %s, want %s", path, e.Links[0].Type, kind)
			}
		}
	}
}

func TestOPDS_ReadingAndFavoritesAreTheCallersOwn(t *testing.T) {
	s := newCatalogStack(t)
	began := s.addWork("Começado", "A", "1.epub", "epub")
	older := s.addWork("Começado antes", "A", "2.epub", "epub")
	done := s.addWork("Terminado", "A", "3.epub", "epub")
	marked := s.addWork("Marcado como lido", "A", "4.epub", "epub")
	untouched := s.addWork("Intocado", "A", "5.epub", "epub")
	_ = untouched
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, updated_at) VALUES ($1, $2, 'p', 30, now() - interval '3 days')`, idAna, s.primaryFile(older))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, updated_at) VALUES ($1, $2, 'p', 10, now())`, idAna, s.primaryFile(began))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at) VALUES ($1, $2, 'fim', 100, now())`, idAna, s.primaryFile(done))
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, 'p', 50)`, idAna, s.primaryFile(marked))
	s.exec(`INSERT INTO work_reading_state (user_id, work_id) VALUES ($1, $2)`, idAna, marked)
	s.exec(`INSERT INTO reading_progress (user_id, file_id, position, percent_complete) VALUES ($1, $2, 'p', 70)`, idBob, s.primaryFile(untouched))

	if got := entryTitles(s.opdsFeedAs(ana, "/opds/reading")); strings.Join(got, "|") != "Começado|Começado antes" {
		t.Errorf("ana is reading %v: what she began, not what she finished or marked, the last one first, and not what bob reads", got)
	}
	if got := entryTitles(s.opdsFeedAs(bob, "/opds/reading")); strings.Join(got, "|") != "Intocado" {
		t.Errorf("bob is reading %v", got)
	}

	s.exec(`INSERT INTO favorites (user_id, work_id, created_at) VALUES ($1, $2, now() - interval '1 day'), ($1, $3, now())`, idAna, began, done)
	s.exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, idBob, marked)
	if got := entryTitles(s.opdsFeedAs(ana, "/opds/favorites")); strings.Join(got, "|") != "Terminado|Começado" {
		t.Errorf("ana's favorites = %v, the last marked first", got)
	}
	if got := entryTitles(s.opdsFeedAs(bob, "/opds/favorites")); strings.Join(got, "|") != "Marcado como lido" {
		t.Errorf("bob's favorites = %v", got)
	}
}

func TestOPDS_AuthorsAreListedWithTheirWorksAndCounts(t *testing.T) {
	s := newCatalogStack(t)
	s.addWork("Duna", "Frank Herbert", "1.epub", "epub")
	s.addWork("Filhos de Duna", "Frank Herbert", "2.epub", "epub")
	s.addWork("Álgebra", "Álvaro Dias", "3.epub", "epub")
	gone := s.addWork("Aposentado", "Bela Torres", "4.epub", "epub")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, gone)

	feed, _ := s.opdsFeed("/opds/authors")
	if got := entryTitles(feed); strings.Join(got, "|") != "Álvaro Dias|Frank Herbert" {
		t.Fatalf("authors = %v (by name without accents; an author with only a retired work is not listed)", got)
	}
	var frank atomLink
	for _, e := range feed.Entries {
		if e.Title == "Frank Herbert" {
			frank = e.Links[0]
		}
	}
	if !strings.Contains(frank.Href, "/opds/v1.2/authors/") || !strings.Contains(frank.Type, "acquisition") {
		t.Fatalf("link of the author = %+v", frank)
	}
	body := s.do(ana, "GET", "/opds/authors", "").Body.String()
	if !strings.Contains(body, `thr:count="2"`) || !strings.Contains(body, "2 obras") {
		t.Errorf("the count of Frank Herbert is missing:\n%s", body)
	}
	id := frank.Href[strings.LastIndex(frank.Href, "/")+1:]
	if got := entryTitles(s.opdsFeedAs(ana, "/opds/authors/"+id)); strings.Join(got, "|") != "Duna|Filhos de Duna" {
		t.Errorf("works of the author = %v", got)
	}
	for _, path := range []string{"/opds/authors/999999", "/opds/authors/abc", "/opds/authors/0"} {
		if rec := s.do(ana, "GET", path, ""); rec.Code != 404 {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
	// the order of names the account prefers
	s.exec(`UPDATE users SET name_order = 'family_first' WHERE id = $1`, idAna)
	s.exec(`UPDATE person SET family_name = 'Herbert', given_name = 'Frank' WHERE name = 'Frank Herbert'`)
	if got := entryTitles(s.opdsFeedAs(ana, "/opds/authors")); strings.Join(got, "|") != "Álvaro Dias|Herbert, Frank" {
		t.Errorf("authors for who reads surnames first = %v", got)
	}
}

func TestOPDS_ASeriesIsInTheOrderOfItsNumbers(t *testing.T) {
	s := newCatalogStack(t)
	third := s.addWork("Terceiro", "A", "3.epub", "epub")
	first := s.addWork("Primeiro", "A", "1.epub", "epub")
	extra := s.addWork("Sem número", "A", "x.epub", "epub")
	other := s.addWork("De outra", "A", "o.epub", "epub")
	var series, retired, personal int64
	s.db.QueryRow(`INSERT INTO collections (kind, name) VALUES ('official', 'A Saga') RETURNING id`).Scan(&series)
	s.db.QueryRow(`INSERT INTO collections (kind, name, retired_at) VALUES ('official', 'Apagada', now()) RETURNING id`).Scan(&retired)
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Minha', 'manual') RETURNING id`, idAna).Scan(&personal)
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, TRUE, 3), ($1, $3, TRUE, 1), ($1, $4, TRUE, NULL)`, series, third, first, extra)
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, TRUE, 1)`, retired, other)
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 1)`, personal, other)

	list, _ := s.opdsFeed("/opds/series")
	if got := entryTitles(list); strings.Join(got, "|") != "A Saga" {
		t.Fatalf("series = %v (not a retired one, not a personal list)", got)
	}
	href := list.Entries[0].Links[0].Href
	if got := entryTitles(s.opdsFeedAs(ana, "/opds/series/"+href[strings.LastIndex(href, "/")+1:])); strings.Join(got, "|") != "Primeiro|Terceiro|Sem número" {
		t.Errorf("the series in order = %v", got)
	}
	for _, id := range []int64{retired, personal} {
		if rec := s.do(ana, "GET", fmt.Sprintf("/opds/series/%d", id), ""); rec.Code != 404 {
			t.Errorf("series/%d: %d, want 404", id, rec.Code)
		}
	}
}

func TestOPDS_AListIsOnlyItsOwnersAndReadLaterComesFirst(t *testing.T) {
	s := newCatalogStack(t)
	w1 := s.addWork("Um", "A", "1.epub", "epub")
	w2 := s.addWork("Dois", "A", "2.epub", "epub")
	var later, mine, bobs int64
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin, system_key) VALUES ('personal', $1, 'Ler depois', 'manual', 'read_later') RETURNING id`, idAna).Scan(&later)
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Aaa minha', 'manual') RETURNING id`, idAna).Scan(&mine)
	s.db.QueryRow(`INSERT INTO collections (kind, owner_id, name, origin) VALUES ('personal', $1, 'Do Bob', 'manual') RETURNING id`, idBob).Scan(&bobs)
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official, position) VALUES ($1, $2, FALSE, 2), ($1, $3, FALSE, 1)`, later, w1, w2)
	s.exec(`INSERT INTO collection_works (collection_id, work_id, official) VALUES ($1, $2, FALSE)`, bobs, w1)

	if got := entryTitles(s.opdsFeedAs(ana, "/opds/lists")); strings.Join(got, "|") != "Ler depois|Aaa minha" {
		t.Errorf("ana's lists = %v", got)
	}
	if got := entryTitles(s.opdsFeedAs(bob, "/opds/lists")); strings.Join(got, "|") != "Do Bob" {
		t.Errorf("bob's lists = %v", got)
	}
	if got := entryTitles(s.opdsFeedAs(ana, fmt.Sprintf("/opds/lists/%d", later))); strings.Join(got, "|") != "Dois|Um" {
		t.Errorf("Ler depois = %v, in the order of the list", got)
	}
	if rec := s.do(bob, "GET", fmt.Sprintf("/opds/lists/%d", later), ""); rec.Code != 404 {
		t.Errorf("a list of someone else: %d, want 404", rec.Code)
	}
}

func TestOPDS_ACategoryLeadsToItsSubcategoriesThenToTheWorksBeneathIt(t *testing.T) {
	s := newCatalogStack(t)
	ficcao := s.addWork("Neuromancer", "A", "1.epub", "epub")
	ensaio := s.addWork("Um ensaio", "A", "2.epub", "epub")
	var top, ciber, vazia, solta int64
	s.db.QueryRow(`INSERT INTO categories (name) VALUES ('Ficção') RETURNING id`).Scan(&top)
	s.db.QueryRow(`INSERT INTO categories (name, parent_id) VALUES ('Ciberpunk', $1) RETURNING id`, top).Scan(&ciber)
	s.db.QueryRow(`INSERT INTO categories (name, parent_id) VALUES ('Vazia', $1) RETURNING id`, top).Scan(&vazia)
	s.db.QueryRow(`INSERT INTO categories (name) VALUES ('Sem nada') RETURNING id`).Scan(&solta)
	s.exec(`INSERT INTO work_categories (work_id, category_id) VALUES ($1, $2), ($3, $4)`, ficcao, ciber, ensaio, top)

	if got := entryTitles(s.opdsFeedAs(ana, "/opds/categories")); strings.Join(got, "|") != "Ficção" {
		t.Errorf("top categories = %v (one with nothing beneath it is not listed)", got)
	}
	feed := s.opdsFeedAs(ana, fmt.Sprintf("/opds/categories/%d", top))
	if got := entryTitles(feed); strings.Join(got, "|") != "Ciberpunk|Neuromancer|Um ensaio" {
		t.Errorf("Ficção = %v: the subcategory first, then the works of it and beneath it", got)
	}
	if got := entryTitles(s.opdsFeedAs(ana, fmt.Sprintf("/opds/categories/%d", ciber))); strings.Join(got, "|") != "Neuromancer" {
		t.Errorf("Ciberpunk = %v", got)
	}
	if rec := s.do(ana, "GET", "/opds/categories/999999", ""); rec.Code != 404 {
		t.Errorf("a category that is not there: %d", rec.Code)
	}
	_ = vazia
	_ = solta
}

func TestOPDS_ACoverHasAThumbnailTheServerMakes(t *testing.T) {
	s := newCatalogStack(t)
	withCover := s.addWork("Com capa", "A", "a.epub", "epub")
	s.addWork("Sem capa", "B", "b.epub", "epub")
	s.exec(`UPDATE editions SET cover_url = '/covers/cover_1.jpg' WHERE work_id = $1 AND is_primary`, withCover)
	feed, _ := s.opdsFeed("/opds/recent")
	thumbs := map[string]string{}
	for _, e := range feed.Entries {
		for _, l := range e.Links {
			if l.Rel == "http://opds-spec.org/image/thumbnail" {
				thumbs[e.Title] = l.Href
			}
		}
	}
	if len(thumbs) != 1 || !strings.HasSuffix(thumbs["Com capa"], "/covers/cover_1.jpg?w=240") {
		t.Errorf("thumbnails = %v (a work with no cover has the placeholder, which is already small)", thumbs)
	}
}
