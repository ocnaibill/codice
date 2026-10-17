package clippings

import (
	"strings"
	"testing"
	"time"
)

const portuguese = "\ufeffO Nome da Rosa (Eco, Umberto)\r\n" +
	"- Seu destaque na página 12 | posição 345-346 | Adicionado: domingo, 4 de outubro de 2026 21:15:30\r\n\r\n" +
	"O nome da rosa é um romance.\r\n==========\r\n" +
	"O Nome da Rosa (Eco, Umberto)\r\n" +
	"- Sua anotação na posição 346 | Adicionado: domingo, 4 de outubro de 2026 21:16:00\r\n\r\n" +
	"Voltar a este trecho.\r\n==========\r\n" +
	"O Nome da Rosa (Eco, Umberto)\r\n" +
	"- Seu marcador na posição 400 | Adicionado: domingo, 4 de outubro de 2026 21:20:00\r\n\r\n\r\n==========\r\n" +
	"Duna (Herbert, Frank)\r\n" +
	"- Seu destaque na posição 90-91 | Adicionado: segunda-feira, 5 de outubro de 2026 08:00:00\r\n\r\n" +
	"<Você atingiu o limite de recortes deste item>\r\n==========\r\n"

const english = "Dune (Frank Herbert)\n" +
	"- Your Highlight on page 12 | Location 345-346 | Added on Monday, October 5, 2026 12:30:00 PM\n\n" +
	"I must not fear.\n==========\n" +
	"Dune (Frank Herbert)\n" +
	"- Your Highlight on Location 345-348 | Added on Monday, October 5, 2026 12:31:00 PM\n\n" +
	"I must not fear. Fear is the mind-killer.\n==========\n" +
	"Dune (Frank Herbert)\n" +
	"- Your Note on Location 500 | Added on Tuesday, October 6, 2026 12:05:00 AM\n\n" +
	"standalone thought\n==========\n"

func TestParseKindle_APortugueseFileWithANoteABookmarkAndAClippingLimit(t *testing.T) {
	res := ParseKindle(portuguese)
	if res.Format != Kindle || len(res.Entries) != 1 {
		t.Fatalf("%d entries, want 1: %+v", len(res.Entries), res.Entries)
	}
	e := res.Entries[0]
	if e.Title != "O Nome da Rosa" || e.Author != "Eco, Umberto" || e.Kind != "note" {
		t.Errorf("entry = %+v", e)
	}
	if e.Text != "O nome da rosa é um romance." || e.Note != "Voltar a este trecho." {
		t.Errorf("the passage and the note joined to it: %q / %q", e.Text, e.Note)
	}
	if e.Place != "12" || e.Location != "345-346" {
		t.Errorf("page %q, location %q", e.Place, e.Location)
	}
	want := time.Date(2026, 10, 4, 21, 15, 30, 0, time.UTC)
	if e.At == nil || !e.At.Equal(want) {
		t.Errorf("date = %v, want %v", e.At, want)
	}
	if res.Skipped["bookmarks"] != 1 || res.Skipped["limit"] != 1 {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestParseKindle_AnEnglishFileKeepsTheLongerOfARepeatedHighlightAndAStandaloneNote(t *testing.T) {
	res := ParseKindle(english)
	if len(res.Entries) != 2 {
		t.Fatalf("%d entries: %+v", len(res.Entries), res.Entries)
	}
	if res.Entries[0].Text != "I must not fear. Fear is the mind-killer." || res.Entries[0].Kind != "highlight" {
		t.Errorf("the highlight kept again with more text replaces the shorter one: %+v", res.Entries[0])
	}
	if res.Entries[1].Kind != "note" || res.Entries[1].Text != "" || res.Entries[1].Note != "standalone thought" {
		t.Errorf("a note on its own: %+v", res.Entries[1])
	}
	if res.Skipped["repeated"] != 1 {
		t.Errorf("skipped = %v", res.Skipped)
	}
	// PM and AM: 12:30 PM is 12:30, and 12:05 AM is 00:05
	if res.Entries[0].At == nil || res.Entries[0].At.Hour() != 12 || res.Entries[1].At == nil || res.Entries[1].At.Hour() != 0 {
		t.Errorf("hours: %v %v", res.Entries[0].At, res.Entries[1].At)
	}
}

func TestParseKindle_TheDateOfEveryLanguage(t *testing.T) {
	for text, want := range map[string]time.Time{
		"Adicionado em quarta-feira, 7 de outubro de 2026 09:05:01": time.Date(2026, 10, 7, 9, 5, 1, 0, time.UTC),
		"Añadido el lunes, 5 de octubre de 2026 19:30:00":           time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC),
		"Ajouté le lundi 5 octobre 2026 19:30:00":                   time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC),
		"Hinzugefügt am Montag, 5. Oktober 2026 19:30:00":           time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC),
		"Aggiunto il lunedì 5 ottobre 2026 19:30:00":                time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC),
		"Added on Friday, January 31, 2025 11:59:59 PM":             time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC),
		"Adicionado: sexta-feira, 1 de março de 2024 00:00:00":      time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	} {
		got := parseKindleDate(text)
		if got == nil || !got.Equal(want) {
			t.Errorf("%q = %v, want %v", text, got, want)
		}
	}
	for _, text := range []string{"", "no date here", "Added on someday"} {
		if got := parseKindleDate(text); got != nil {
			t.Errorf("%q = %v, want nil", text, got)
		}
	}
}

func TestParseKindle_WhatIsNotAnEntryIsLeftAloneAndAnEntryWithoutADateIsKept(t *testing.T) {
	res := ParseKindle("garbage\n==========\nTitle (A)\n- Seu destaque na posição 1\n\ntext\n==========\n\n")
	if len(res.Entries) != 1 || res.Entries[0].At != nil || res.Entries[0].Title != "Title" {
		t.Errorf("%+v", res.Entries)
	}
	if got := ParseKindle(""); len(got.Entries) != 0 {
		t.Error("an empty file has entries")
	}
}

const koreaderSingle = `{
 "title": "As Foices", "author": "Neal Shusterman", "exported": 1792000000, "file": "/mnt/us/koreader/As foices.epub", "number_of_pages": 400,
 "md5sum": "abc", "created_on": 1792000000, "version": "1.0",
 "entries": [
  {"sort":"highlight","page":12,"time":1791900000,"text":"Citra tem razão","chapter":"Capítulo 2","color":"yellow","pn_xp":"/body/DocFragment[6]/body/div/p[166]/text().0"},
  {"sort":"highlight","page":"13","time":"2026-10-13 10:20:30","text":"Outro trecho","note":"anotação minha","pn_xp":"/body/DocFragment[7]/body/div/p[3]"},
  {"sort":"bookmark","page":14,"time":1791900001},
  {"sort":"highlight","page":15,"time":1791900002,"text":"  "}
 ]}`

func TestParseKOReader_OneBookWithItsPlacesAndNotes(t *testing.T) {
	res, err := ParseKOReader([]byte(koreaderSingle))
	if err != nil || res.Format != KOReader || len(res.Entries) != 2 {
		t.Fatalf("%v %d", err, len(res.Entries))
	}
	a, b := res.Entries[0], res.Entries[1]
	if a.Title != "As Foices" || a.Author != "Neal Shusterman" || a.Kind != "highlight" || a.Text != "Citra tem razão" || a.Color != "yellow" || a.Chapter != "Capítulo 2" {
		t.Errorf("first = %+v", a)
	}
	if a.Place != "/body/DocFragment[6]/body/div/p[166]/text().0" || a.At == nil || a.At.Unix() != 1791900000 {
		t.Errorf("place %q, time %v", a.Place, a.At)
	}
	if b.Kind != "note" || b.Note != "anotação minha" || b.At == nil || b.At.Format("2006-01-02 15:04:05") != "2026-10-13 10:20:30" {
		t.Errorf("second = %+v", b)
	}
	if res.Skipped["bookmarks"] != 1 || res.Skipped["empty"] != 1 {
		t.Errorf("skipped = %v", res.Skipped)
	}
}

func TestParseKOReader_SeveralBooksAndWhatIsNotAnExport(t *testing.T) {
	res, err := ParseKOReader([]byte(`{"created_on":1,"version":"1","documents":[
		{"title":"A","author":"X","entries":[{"sort":"highlight","text":"um","page":3}]},
		{"title":"B","author":"Y","entries":[{"sort":"highlight","text":"dois","pn_xp":12}]}]}`))
	if err != nil || len(res.Entries) != 2 || res.Entries[1].Title != "B" || res.Entries[1].Place != "12" {
		t.Fatalf("%v %+v", err, res.Entries)
	}
	for _, bad := range []string{"", "not json", "[]", `{"hello":1}`} {
		if _, err := ParseKOReader([]byte(bad)); err == nil {
			t.Errorf("%q is not an export", bad)
		}
	}
}

func TestKey_TwoWritingsOfATitleHaveTheSameKey(t *testing.T) {
	if Key("O Nome da Rosa!") != Key("o  nome   da rosa") || Key("Café") != Key("cafe") {
		t.Error("the key tells them apart")
	}
	if !strings.Contains(GroupKey("Duna", "Herbert, Frank"), "|") {
		t.Error("the key of a book has its author")
	}
}
