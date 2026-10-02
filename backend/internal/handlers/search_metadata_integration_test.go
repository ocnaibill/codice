package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// The text a comic or an audio file carries about itself is searched like any other, and the hit opens the file where
// it says: a chapter of an audiobook at its minute, a description from the start (#25).

func TestSearch_AChapterOfAnAudiobookOpensAtItsMinute(t *testing.T) {
	s := newCatalogStack(t)
	book := s.addWork("O Livro de Teste", "Ana", "b.m4b", "m4b")
	file := s.primaryFile(book)
	s.index(file,
		segment{text: "Narrado por uma voz de teste.", section: "Descrição", locator: `{"type":"audio","track":0,"ms":0}`},
		segment{text: "Capítulo 1: A corrida de ontem", section: "Capítulo 1: A corrida de ontem", locator: `{"type":"audio","track":0,"ms":65250}`},
	)
	_, r := s.search(ana, q("corrida"))
	if len(r.Data) != 1 {
		t.Fatalf("hits: %+v", r.Data)
	}
	hit := r.Data[0]
	var loc map[string]any
	json.Unmarshal(hit.Locator, &loc)
	if hit.WorkID != book || hit.FileID != file || hit.Format != "m4b" || hit.Section != "Capítulo 1: A corrida de ontem" ||
		loc["type"] != "audio" || loc["ms"] != float64(65250) || loc["track"] != float64(0) {
		t.Errorf("the chapter: %+v locator %s", hit, hit.Locator)
	}
}

func TestSearch_TheDescriptionOfAComicOpensItsStartAndSaysWhereItCameFrom(t *testing.T) {
	s := newCatalogStack(t)
	comic := s.addWork("Volume Dois", "Autora", "v.cbz", "cbz")
	s.index(s.primaryFile(comic),
		segment{text: "Ana, Bruno e a Corredora Misteriosa", section: "Personagens", locator: `{"type":"image","index":0,"item":"ComicInfo.xml"}`})
	_, r := s.search(ana, q("misteriosa"))
	if len(r.Data) != 1 {
		t.Fatalf("hits: %+v", r.Data)
	}
	var loc map[string]any
	json.Unmarshal(r.Data[0].Locator, &loc)
	if r.Data[0].Section != "Personagens" || loc["type"] != "image" || loc["index"] != float64(0) || loc["item"] != "ComicInfo.xml" {
		t.Errorf("the description of the comic: %+v locator %s", r.Data[0], r.Data[0].Locator)
	}
}

func TestExtractText_TheMigrationQueuesTheComicsAndAudioAlreadyThere(t *testing.T) {
	s := newCatalogStack(t)
	comic := s.addWork("Gibi", "X", "g.cbz", "cbz")
	audio := s.addWork("Áudio", "X", "a.m4b", "m4b")
	upper := s.addWork("Maiúsculas", "X", "u.MP3", "MP3")
	book := s.addWork("Livro", "X", "l.epub", "epub")
	pdf := s.addWork("Documento", "X", "d.pdf", "pdf")
	retired := s.addWork("Aposentado", "X", "r.cbz", "cbz")
	s.exec(`UPDATE works SET retired_at = now() WHERE id = $1`, retired)
	s.exec(`DELETE FROM jobs`)

	// The statement of the migration itself, read from its file.
	file, err := os.ReadFile("../database/migrations/00035_text_of_comics_and_audio.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(file)
	start := strings.Index(text, "INSERT INTO jobs")
	statement := text[start : start+strings.Index(text[start:], ";")]
	s.exec(statement)
	s.exec(statement) // once each, however many times it runs

	got := s.scalar(`SELECT string_agg(work_id::text, ',' ORDER BY work_id) FROM jobs WHERE type = 'extract_text'`)
	want := fmt.Sprintf("%d,%d,%d", comic, audio, upper)
	if got != want {
		t.Errorf("queued for %q, want the comic, the audio and the audio in capitals (%s), not the EPUB (%d), the PDF (%d) or the retired comic (%d)", got, want, book, pdf, retired)
	}
}
