package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type deviceNotesAnswer struct {
	Notes []deviceNoteOut
	Gone  []int
}

func (k *kosyncStack) deviceNotes(a actor, file int64, known string) (int, deviceNotesAnswer) {
	k.t.Helper()
	q := ""
	if known != "" {
		q = "?known=" + known
	}
	rec := k.do(a, "GET", fmt.Sprintf("/device/files/%d/notes%s", file, q), "")
	var out deviceNotesAnswer
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (k *kosyncStack) putDeviceNote(a actor, file int64, ref, body string) (int, map[string]any) {
	k.t.Helper()
	rec := k.do(a, "PUT", fmt.Sprintf("/device/files/%d/notes/%s", file, ref), body)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const deviceHighlight = `{"pos0":"/body/DocFragment[2]/body/div/p[7]/text().12","pos1":"/body/DocFragment[2]/body/div/p[7]/text().48","text":"Parágrafo 7 do item 2.","note":"","drawer":"lighten","chapter":"Capítulo 2","datetime":"2026-10-18 14:20:00","device":"Kindle Basic 3"}`

func TestDeviceNotes_AHighlightMadeOnTheDeviceIsKeptWithItsPlaceAndSentAgainChangesTheSameOne(t *testing.T) {
	k := newKosyncStack(t)
	_, file, _ := k.book("Duna", "dune.epub", "epub")

	code, out := k.putDeviceNote(ana, file, "ref00001", deviceHighlight)
	if code != 201 || out["created"] != true {
		t.Fatalf("PUT: %d %v", code, out)
	}
	var kind, quote, body, color, loc, pos0 string
	k.db.QueryRow(`SELECT kind, quote, body, color, locator::text, device_pos0 FROM notes WHERE user_id = $1 AND file_id = $2 AND device_ref = 'ref00001'`, idAna, file).Scan(&kind, &quote, &body, &color, &loc, &pos0)
	if kind != "highlight" || quote != "Parágrafo 7 do item 2." || body != "" || color != "terracotta" {
		t.Errorf("the note: %s %q %q %s", kind, quote, body, color)
	}
	// the 7th paragraph of the second item, as a CFI the web opens at (see the kosync test of the paragraph)
	if !strings.Contains(loc, "epubcfi(/6/4!/4/2/16)") || pos0 != "/body/DocFragment[2]/body/div/p[7]/text().12" {
		t.Errorf("the place: %s, exact start %q", loc, pos0)
	}

	// the person gives it a color in the web; then writes a note on it on the device: it is the same note, with the same color
	k.exec(`UPDATE notes SET color = 'sage' WHERE device_ref = 'ref00001'`)
	edited := strings.Replace(deviceHighlight, `"note":""`, `"note":"Isto vale uma releitura."`, 1)
	code, out = k.putDeviceNote(ana, file, "ref00001", edited)
	if code != 200 || out["created"] != false {
		t.Fatalf("PUT again: %d %v", code, out)
	}
	k.db.QueryRow(`SELECT kind, body, color FROM notes WHERE user_id = $1 AND file_id = $2 AND device_ref = 'ref00001'`, idAna, file).Scan(&kind, &body, &color)
	var n int
	k.db.QueryRow(`SELECT count(*) FROM notes WHERE user_id = $1 AND file_id = $2`, idAna, file).Scan(&n)
	if n != 1 || kind != "note" || body != "Isto vale uma releitura." || color != "sage" {
		t.Errorf("sent again: %d notes, %s %q %s", n, kind, body, color)
	}
}

func TestDeviceNotes_TheListIsWhatTheCodiceHasAndSaysWhichKnownNotesAreGone(t *testing.T) {
	k := newKosyncStack(t)
	work, file, _ := k.book("Duna", "dune.epub", "epub")
	k.putDeviceNote(ana, file, "ref00001", deviceHighlight)
	// a note made in the web, in the 10th paragraph of the third item, and a bookmark (not a thing for the device)
	k.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color)
	        VALUES ($1, $2, $3, 'highlight', 'Parágrafo 10 do item 3.', '', '{}', '{"type":"epub","cfi":"epubcfi(/6/6!/4/2/22/1:5)","href":"Text/tres.xhtml","progression":0.5}'::jsonb, 1, 'indigo'),
	               ($1, $2, $3, 'bookmark', NULL, '', '{}', '{"type":"epub","cfi":"epubcfi(/6/6!/4/2/2)","href":"Text/tres.xhtml"}'::jsonb, 1, 'sepia')`, idAna, work, file)
	var deviceID, webID int
	k.db.QueryRow(`SELECT id FROM notes WHERE device_ref = 'ref00001'`).Scan(&deviceID)
	k.db.QueryRow(`SELECT id FROM notes WHERE quote = 'Parágrafo 10 do item 3.'`).Scan(&webID)

	code, got := k.deviceNotes(ana, file, fmt.Sprintf("%d,%d,99999", deviceID, webID))
	if code != 200 || len(got.Notes) != 2 {
		t.Fatalf("list: %d %+v", code, got)
	}
	d, w := got.Notes[0], got.Notes[1]
	if !d.FromDevice || d.Ref != "ref00001" || d.Pos0 != "/body/DocFragment[2]/body/div/p[7]/text().12" || d.Pos1 == "" || d.Place != "" {
		t.Errorf("the one the device made comes back as it gave it: %+v", d)
	}
	if w.FromDevice || w.Pos0 != "" || w.Place != "/body/DocFragment[3]/body/div/p[10]" || w.Quote != "Parágrafo 10 do item 3." || w.Color != "indigo" {
		t.Errorf("the one made in the web comes with the paragraph by name: %+v", w)
	}
	if len(got.Gone) != 1 || got.Gone[0] != 99999 {
		t.Errorf("gone: %v", got.Gone)
	}
	// someone else's notes are not listed, and what the other deleted is not "known" by this one
	if _, bob := k.deviceNotes(bob, file, ""); len(bob.Notes) != 0 {
		t.Errorf("bob sees %d notes of ana", len(bob.Notes))
	}
}

func TestDeviceNotes_ADeviceDeletesOnlyWhatItMadeAndOnlyItsOwners(t *testing.T) {
	k := newKosyncStack(t)
	work, file, _ := k.book("Duna", "dune.epub", "epub")
	k.putDeviceNote(ana, file, "ref00001", deviceHighlight)
	k.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, color) VALUES ($1, $2, $3, 'highlight', 'da web', '', '{}', 'sepia')`, idAna, work, file)

	// the same reference of another person is another note
	k.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, color, device_ref) VALUES ($1, $2, $3, 'highlight', 'do bob', '', '{}', 'sepia', 'ref00001')`, idBob, work, file)
	if rec := k.do(bob, "DELETE", fmt.Sprintf("/device/files/%d/notes/ref00001", file), ""); rec.Code != 204 {
		t.Fatalf("bob deletes his: %d", rec.Code)
	}
	var n int
	k.db.QueryRow(`SELECT count(*) FROM notes WHERE user_id = $1 AND device_ref = 'ref00001'`, idAna).Scan(&n)
	if n != 1 {
		t.Fatalf("ana's note is still there: %d", n)
	}
	// a reference that does not exist, and a note that came from the web (it has no reference to delete it by)
	if rec := k.do(ana, "DELETE", fmt.Sprintf("/device/files/%d/notes/nada0000", file), ""); rec.Code != 404 {
		t.Errorf("a reference that is nobody's: %d", rec.Code)
	}
	if rec := k.do(ana, "DELETE", fmt.Sprintf("/device/files/%d/notes/ref00001", file), ""); rec.Code != 204 {
		t.Errorf("ana deletes hers: %d", rec.Code)
	}
	k.db.QueryRow(`SELECT count(*) FROM notes WHERE user_id = $1`, idAna).Scan(&n)
	if n != 1 {
		t.Errorf("the note of the web stays: %d", n)
	}
}

func TestDeviceNotes_WhatIsNotAnEpubOrNotAValidRequestIsRefused(t *testing.T) {
	k := newKosyncStack(t)
	_, pdf, _ := k.book("Manual", "manual.pdf", "pdf")
	_, epub, _ := k.book("Duna", "dune.epub", "epub")
	if code, _ := k.putDeviceNote(ana, pdf, "ref00001", deviceHighlight); code != 422 {
		t.Errorf("a pdf: %d", code)
	}
	if code, _ := k.putDeviceNote(ana, epub, "no", deviceHighlight); code != 400 {
		t.Errorf("a reference that is too short: %d", code)
	}
	if code, _ := k.putDeviceNote(ana, epub, "ref00001", `{"pos0":"x","text":"  ","note":""}`); code != 400 {
		t.Errorf("a note with nothing in it: %d", code)
	}
	if code, _ := k.putDeviceNote(ana, 999999, "ref00001", deviceHighlight); code != 404 {
		t.Errorf("a file that does not exist: %d", code)
	}
	// a place that leads nowhere is kept all the same, with no place
	if code, _ := k.putDeviceNote(ana, epub, "ref00002", `{"pos0":"/body/DocFragment[99]/body/p[1]/text().0","pos1":"","text":"fora","note":""}`); code != 201 {
		t.Errorf("a place that leads nowhere: %d", code)
	}
	var loc *string
	k.db.QueryRow(`SELECT locator::text FROM notes WHERE device_ref = 'ref00002'`).Scan(&loc)
	if loc != nil {
		t.Errorf("no place was made up: %s", *loc)
	}
}

func TestDeviceNotes_ANoteOfTheWebTheDeviceChangesBecomesItsOwnAndKeepsItsColor(t *testing.T) {
	k := newKosyncStack(t)
	work, file, _ := k.book("Duna", "dune.epub", "epub")
	k.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color)
	        VALUES ($1, $2, $3, 'highlight', 'Parágrafo 10 do item 3.', '', '{vital}', '{"type":"epub","cfi":"epubcfi(/6/6!/4/2/22/1:5)","href":"Text/tres.xhtml","progression":0.5}'::jsonb, 1, 'indigo')`, idAna, work, file)
	var webID int
	k.db.QueryRow(`SELECT id FROM notes WHERE quote = 'Parágrafo 10 do item 3.'`).Scan(&webID)

	body := fmt.Sprintf(`{"id":%d,"pos0":"/body/DocFragment[3]/body/div/p[10]/text().0","pos1":"/body/DocFragment[3]/body/div/p[10]/text().23","text":"Parágrafo 10 do item 3.","note":"escrita no Kindle","device":"Kindle"}`, webID)
	code, out := k.putDeviceNote(ana, file, "ref00009", body)
	if code != 200 || int(out["id"].(float64)) != webID || out["created"] != false {
		t.Fatalf("PUT with the number: %d %v", code, out)
	}
	var n int
	var ref, color, note string
	var tags string
	k.db.QueryRow(`SELECT count(*) FROM notes WHERE user_id = $1 AND file_id = $2`, idAna, file).Scan(&n)
	k.db.QueryRow(`SELECT device_ref, color, body, tags::text FROM notes WHERE id = $1`, webID).Scan(&ref, &color, &note, &tags)
	if n != 1 || ref != "ref00009" || color != "indigo" || note != "escrita no Kindle" || tags != "{vital}" {
		t.Errorf("the same note: %d notes, ref %q, color %s, body %q, tags %s", n, ref, color, note, tags)
	}
	// a number that is nobody's (deleted meanwhile): it is made as new
	if code, out = k.putDeviceNote(ana, file, "ref00010", strings.Replace(body, fmt.Sprintf(`"id":%d`, webID), `"id":99999`, 1)); code != 201 {
		t.Errorf("a number that does not exist: %d %v", code, out)
	}
	// deleting by the number: the note the device received and is deleting (made in the web), but not a bookmark, not another file's, not another person's
	k.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color)
	        VALUES ($1, $2, $3, 'bookmark', NULL, '', '{}', '{"type":"epub","cfi":"epubcfi(/6/6)","href":"Text/tres.xhtml"}'::jsonb, 1, 'sepia')`, idAna, work, file)
	var mark int
	k.db.QueryRow(`SELECT id FROM notes WHERE kind = 'bookmark'`).Scan(&mark)
	k.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, color) VALUES ($1, $2, $3, 'highlight', 'do bob', '', '{}', 'sepia')`, idBob, work, file)
	var bobs int
	k.db.QueryRow(`SELECT id FROM notes WHERE user_id = $1`, idBob).Scan(&bobs)
	for _, id := range []int{mark, bobs} {
		if rec := k.do(ana, "DELETE", fmt.Sprintf("/device/files/%d/notes/nada0000?id=%d", file, id), ""); rec.Code != 404 {
			t.Errorf("deleting note %d by number: %d", id, rec.Code)
		}
	}
	if rec := k.do(ana, "DELETE", fmt.Sprintf("/device/files/%d/notes/nada0000?id=%d", file, webID), ""); rec.Code != 204 {
		t.Errorf("deleting the one that is hers by number: %d", rec.Code)
	}
}

func TestDeviceNotes_APlaceAsKOReaderWritesItInAHighlightIsTheSameParagraph(t *testing.T) {
	k := newKosyncStack(t)
	_, file, _ := k.book("Duna", "dune.epub", "epub")
	// a highlight of KOReader writes every step with its index, even the first: body[1]/div[1]/p[7]/text()[1].12
	body := `{"pos0":"/body[1]/DocFragment[2]/body[1]/div[1]/p[7]/text()[1].12","pos1":"/body[1]/DocFragment[2]/body[1]/div[1]/p[7]/text()[1].30","text":"Parágrafo 7 do item 2.","note":""}`
	if code, _ := k.putDeviceNote(ana, file, "ref00007", body); code != 201 {
		t.Fatalf("PUT: %d", code)
	}
	var loc string
	k.db.QueryRow(`SELECT COALESCE(locator::text, '') FROM notes WHERE device_ref = 'ref00007'`).Scan(&loc)
	if !strings.Contains(loc, "epubcfi(/6/4!/4/2/16)") {
		t.Errorf("the paragraph is the 7th of the second item: %s", loc)
	}
}
