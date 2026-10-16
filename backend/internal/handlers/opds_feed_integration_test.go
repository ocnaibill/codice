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
