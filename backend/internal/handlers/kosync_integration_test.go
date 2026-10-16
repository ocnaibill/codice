package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/sessions"
)

// kosyncMD5 is the hash a device knows a book by (util.partialMD5): samples of 1 KiB at the start and at 1 KiB, 4 KiB, 16 KiB...
func kosyncMD5(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := md5.New()
	for i := -1; i <= 10; i++ {
		off := 0
		if i >= 0 {
			off = 1024 << (2 * i)
		}
		if off >= len(data) {
			break
		}
		end := off + 1024
		if end > len(data) {
			end = len(data)
		}
		h.Write(data[off:end])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// epubWith writes an EPUB with the items of the reading order given (the third is outside the flow of reading).
func epubWith(t *testing.T, path string, items ...string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	add("mimetype", "application/epub+zip")
	add("META-INF/container.xml", `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	var manifest, spine strings.Builder
	for i, href := range items {
		fmt.Fprintf(&manifest, `<item id="i%d" href="%s" media-type="application/xhtml+xml"/>`, i, href)
		linear := ""
		if i == 2 {
			linear = ` linear="no"`
		}
		fmt.Fprintf(&spine, `<itemref idref="i%d"%s/>`, i, linear)
		var paragraphs strings.Builder
		for p := 1; p <= 20; p++ {
			fmt.Fprintf(&paragraphs, "<p>Parágrafo %d do item %d. %s</p>", p, i+1, strings.Repeat("texto ", 40))
		}
		add("OEBPS/"+strings.ReplaceAll(href, "%20", " "),
			`<?xml version="1.0"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head><body><div><h1>Título</h1>`+paragraphs.String()+`</div></body></html>`)
	}
	add("OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><manifest>`+manifest.String()+`</manifest><spine>`+spine.String()+`</spine></package>`)
	zw.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type kosyncStack struct {
	*catalogStack
	typed string // what the person types as the password
}

func newKosyncStack(t *testing.T) *kosyncStack {
	s := newCatalogStack(t)
	store := &sessions.Store{DB: s.db}
	_, shown, err := store.CreateScopedAppToken(t.Context(), idAna, "Kindle", sessions.ScopeKoreader)
	if err != nil {
		t.Fatal(err)
	}
	return &kosyncStack{catalogStack: s, typed: strings.ReplaceAll(shown, " ", "")}
}

func (k *kosyncStack) call(method, target, user, typed, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if user != "" {
		req.Header.Set("x-auth-user", user)
		req.Header.Set("x-auth-key", sessions.KosyncKey(typed))
	}
	rec := httptest.NewRecorder()
	k.router.ServeHTTP(rec, req)
	return rec
}

func (k *kosyncStack) put(doc, progress string, percentage float64, device, deviceID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"document": doc, "progress": progress, "percentage": percentage, "device": device, "device_id": deviceID})
	return k.call("PUT", "/kosync/syncs/progress", "ana", k.typed, string(body))
}

func (k *kosyncStack) get(doc string) map[string]any {
	k.t.Helper()
	rec := k.call("GET", "/kosync/syncs/progress/"+doc, "ana", k.typed, "")
	if rec.Code != 200 {
		k.t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// book adds a work with a file on disk and gives it the hash a device would know it by.
func (k *kosyncStack) book(title, name, format string) (work int, file int64, doc string) {
	k.t.Helper()
	full := filepath.Join(k.storage, name)
	if format == "epub" {
		epubWith(k.t, full, "Text/um.xhtml", "Text/dois%20capitulo.xhtml", "Text/tres.xhtml")
	} else {
		os.WriteFile(full, bytes.Repeat([]byte(title+"-"), 3000), 0o644)
	}
	work = k.addWork(title, "Autor", name, format)
	file = k.primaryFile(work)
	doc = kosyncMD5(k.t, full)
	k.exec(`UPDATE files SET koreader_md5 = $1 WHERE id = $2`, doc, file)
	return
}

func TestKosync_OnlyTheShortTokenOfTheRightPersonOpens(t *testing.T) {
	k := newKosyncStack(t)
	groups := []string{k.typed[0:4], k.typed[4:8], k.typed[8:12], k.typed[12:16]}
	for name, typed := range map[string]string{
		"as it is typed lower": k.typed, "in capitals": strings.ToUpper(k.typed), "first letter in a capital": strings.ToUpper(k.typed[:1]) + k.typed[1:],
		"in groups": strings.Join(groups, " "), "with dashes": strings.Join(groups, "-"), "in capitals, in groups": strings.ToUpper(strings.Join(groups, " ")),
	} {
		if rec := k.call("GET", "/kosync/users/auth", "ana", typed, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "OK") {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	for name, c := range map[string]struct{ user, typed string }{
		"no credentials": {"", ""}, "a wrong password": {"ana", "aaaaaaaaaaaaaaaa"}, "another person": {"bob", k.typed},
		"the token of another way": {"ana", k.typed[:15]},
	} {
		if rec := k.call("GET", "/kosync/users/auth", c.user, c.typed, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, rec.Code)
		}
	}
	// a long token (an OPDS client) does not open the sync, and a revoked one stops
	store := &sessions.Store{DB: k.db}
	_, long, _ := store.CreateAppToken(t.Context(), idAna, "Moon")
	if rec := k.call("GET", "/kosync/users/auth", "ana", long, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("a long token: %d", rec.Code)
	}
	k.exec(`UPDATE app_tokens SET revoked_at = now() WHERE scope = 'koreader'`)
	if rec := k.call("GET", "/kosync/users/auth", "ana", k.typed, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("a revoked token: %d", rec.Code)
	}
	if rec := k.call("POST", "/kosync/users/create", "", "", `{"username":"x","password":"y"}`); rec.Code != http.StatusPaymentRequired {
		t.Errorf("registering: %d, want 402", rec.Code)
	}
	// nothing that identifies the typed password is kept in the clear
	var stored string
	k.db.QueryRow(`SELECT array_to_string(kosync_hashes, ',') FROM app_tokens WHERE scope = 'koreader'`).Scan(&stored)
	if strings.Contains(stored, sessions.KosyncKey(k.typed)) || stored == "" {
		t.Error("the key is kept as it is, or the hashes are missing")
	}
}

func TestKosync_ADeviceSaysWhereItIsAndGetsItBackAsItSaidIt(t *testing.T) {
	k := newKosyncStack(t)
	work, file, doc := k.book("Duna", "dune.epub", "epub")
	// the person had finished it and marked the work as finished: reading on takes the mark off
	k.exec(`INSERT INTO work_reading_state (user_id, work_id) VALUES ($1, $2)`, idAna, work)

	xpointer := "/body/DocFragment[2]/body/div/p[166]/text().0"
	if rec := k.put(doc, xpointer, 0.4, "KindleBasic3", "DEVICE1"); rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	var loc, device, position string
	var percent float64
	var revision int
	k.db.QueryRow(`SELECT locator::text, device, position, percent_complete, revision FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, file).
		Scan(&loc, &device, &position, &percent, &revision)
	if !strings.Contains(loc, `"href": "Text/dois%20capitulo.xhtml"`) || !strings.Contains(loc, `"type": "epub"`) {
		t.Errorf("locator = %s: the second item of the reading order, as the OPF writes it (the reader names a chapter so)", loc)
	}
	if device != "Kindle" || percent != 40 || position == "" {
		t.Errorf("device %q, percent %v, position %q: the device is named as the person named the access", device, percent, position)
	}
	if k.scalar(`SELECT count(*)::text FROM work_reading_state WHERE user_id = $1`, idAna) != "0" {
		t.Error("reading on did not take the finished mark off")
	}

	got := k.get(doc)
	if got["progress"] != xpointer || got["percentage"] != 0.4 || got["device"] != "KindleBasic3" || got["device_id"] != "DEVICE1" || got["timestamp"] == nil {
		t.Errorf("GET = %v: what the device said, as it said it, so that it knows it is its own", got)
	}

	// the person reads on in the web, further on: the place is the web's, made into what a device understands
	body := `{"locator":{"type":"epub","cfi":"epubcfi(/6/8)","href":"Text/tres.xhtml","progression":0.5},"percent":70,"device":"Web"}`
	if rec := k.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), body); rec.Code != 200 {
		t.Fatalf("web PUT: %d %s", rec.Code, rec.Body.String())
	}
	got = k.get(doc)
	if got["progress"] != "/body/DocFragment[3]" || got["device"] != "Códice" || got["device_id"] != "codice-web" || got["percentage"] != 0.7 {
		t.Errorf("GET after the web = %v", got)
	}

	// the device writes again: its own place is back
	k.put(doc, "/body/DocFragment[1]/body/div/p[2]/text().0", 0.05, "KindleBasic3", "DEVICE1")
	if got = k.get(doc); got["progress"] != "/body/DocFragment[1]/body/div/p[2]/text().0" || got["device"] != "KindleBasic3" {
		t.Errorf("GET after the device = %v", got)
	}
	if k.scalar(`SELECT revision::text FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, file) != "3" {
		t.Error("each write is a revision (the device, the web, the device)")
	}
	// it is the person's own: nobody else sees it
	if rec := k.call("GET", "/kosync/syncs/progress/"+doc, "bob", "x", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("another person: %d", rec.Code)
	}
}

func TestKosync_APlaceTheLibraryCannotKeepIsAcceptedAndNotKept(t *testing.T) {
	k := newKosyncStack(t)
	_, file, doc := k.book("Duna", "dune.epub", "epub")
	for name, progress := range map[string]string{"an xpointer with no item": "/body/p[3]", "an item that is not there": "/body/DocFragment[99]/body/p", "nothing": ""} {
		if rec := k.put(doc, progress, 0.3, "K", "D"); rec.Code != http.StatusAccepted {
			t.Errorf("%s: %d, want 202", name, rec.Code)
		}
	}
	if rec := k.put(strings.Repeat("ab", 16), "/body/DocFragment[1]", 0.3, "K", "D"); rec.Code != http.StatusAccepted {
		t.Errorf("a book the library does not have: %d, want 202", rec.Code)
	}
	if k.scalar(`SELECT count(*)::text FROM reading_progress WHERE file_id = $1`, file) != "0" {
		t.Error("a place that could not be understood was kept")
	}
	if got := k.get(doc); len(got) != 0 {
		t.Errorf("GET with no progress = %v, want {}", got)
	}
	if got := k.get(strings.Repeat("cd", 16)); len(got) != 0 {
		t.Errorf("GET of a book the library does not have = %v", got)
	}
	for _, target := range []string{"/kosync/syncs/progress/not-a-hash", "/kosync/syncs/progress/" + strings.Repeat("g", 32)} {
		if rec := k.call("GET", target, "ana", k.typed, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", target, rec.Code)
		}
	}
	if rec := k.call("PUT", "/kosync/syncs/progress", "ana", k.typed, `{"document":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a bad document: %d", rec.Code)
	}
}

func TestKosync_APdfAndAComicAreKeptByTheirPage(t *testing.T) {
	k := newKosyncStack(t)
	_, pdfFile, pdf := k.book("Manual", "manual.pdf", "pdf")
	_, cbzFile, cbz := k.book("Quadrinho", "hq.cbz", "cbz")
	k.put(pdf, "12", 0.5, "KindleBasic3", "D")
	k.put(cbz, "7", 0.2, "KindleBasic3", "D")
	if k.scalar(`SELECT locator::text FROM reading_progress WHERE file_id = $1`, pdfFile) != `{"page": 11, "type": "pdf"}` {
		t.Errorf("pdf locator = %s", k.scalar(`SELECT locator::text FROM reading_progress WHERE file_id = $1`, pdfFile))
	}
	if loc := k.scalar(`SELECT locator::text FROM reading_progress WHERE file_id = $1`, cbzFile); !strings.Contains(loc, `"index": 6`) {
		t.Errorf("comic locator = %s", loc)
	}
	// from the web, the page goes back as the device counts it (from 1)
	k.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", pdfFile), `{"locator":{"type":"pdf","page":19},"percent":30,"device":"Web"}`)
	if got := k.get(pdf); got["progress"] != "20" || got["device"] != "Códice" {
		t.Errorf("pdf from the web = %v", got)
	}
	if got := k.get(cbz); got["progress"] != "7" || got["device"] != "KindleBasic3" {
		t.Errorf("comic = %v", got)
	}
}

func TestKosync_TheParagraphIsKeptNotOnlyTheChapterAndComesBackTheSame(t *testing.T) {
	k := newKosyncStack(t)
	_, file, doc := k.book("Duna", "dune.epub", "epub")

	// the device is at the 7th paragraph of the second item: the reader opens at the same element
	xpointer := "/body/DocFragment[2]/body/div/p[7]/text().12"
	if rec := k.put(doc, xpointer, 0.4, "KindleBasic3", "DEVICE1"); rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	var loc, position string
	k.db.QueryRow(`SELECT locator::text, position FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, file).Scan(&loc, &position)
	// body is the 2nd child of html (after head: /4), the div its 1st child (/2), the 7th p is the 8th child of the div (the h1 comes first): /16
	if want := "epubcfi(/6/4!/4/2/16)"; position != want || !strings.Contains(loc, want) {
		t.Errorf("position %q, locator %s, want the CFI %s", position, loc, want)
	}
	if !strings.Contains(loc, `"href": "Text/dois%20capitulo.xhtml"`) || !strings.Contains(loc, `"progression": 0.`) {
		t.Errorf("the chapter and how far into it: %s", loc)
	}
	if got := k.get(doc); got["progress"] != xpointer {
		t.Errorf("GET = %v, the device is given what it said", got["progress"])
	}

	// the person reads on in the web, and the reader says where with a CFI: the device is told the same paragraph, by name
	web := `{"locator":{"type":"epub","cfi":"epubcfi(/6/6[x]!/4/2/22/1:5)","href":"Text/tres.xhtml","progression":0.5},"percent":70,"device":"Web"}`
	if rec := k.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), web); rec.Code != 200 {
		t.Fatalf("web PUT: %d %s", rec.Code, rec.Body.String())
	}
	if got := k.get(doc); got["progress"] != "/body/DocFragment[3]/body/div/p[10]" || got["device"] != "Códice" {
		t.Errorf("GET after the web = %v, want the 10th paragraph of the third item", got)
	}
	// a CFI that points at the chapter and not into it, or beyond what the chapter has, is the start of the chapter
	for cfi, want := range map[string]string{
		"epubcfi(/6/6!/4/2/9998/1:0)": "/body/DocFragment[3]",
		"epubcfi(/6/6!/4/2/2)":        "/body/DocFragment[3]/body/div/h1",
		"epubcfi(/6/6)":               "/body/DocFragment[3]",
	} {
		body := fmt.Sprintf(`{"locator":{"type":"epub","cfi":%q,"href":"Text/tres.xhtml"},"percent":71,"device":"Web"}`, cfi)
		k.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), body)
		if got := k.get(doc); got["progress"] != want {
			t.Errorf("%s: %v, want %s", cfi, got["progress"], want)
		}
	}
	// a path of the device that leads nowhere is the start of the chapter, kept all the same
	k.put(doc, "/body/DocFragment[2]/body/div/p[999]/text().0", 0.5, "KindleBasic3", "DEVICE1")
	k.db.QueryRow(`SELECT locator::text, position FROM reading_progress WHERE user_id = $1 AND file_id = $2`, idAna, file).Scan(&loc, &position)
	if position != "Text/dois%20capitulo.xhtml" || strings.Contains(loc, "cfi") {
		t.Errorf("a path that leads nowhere: position %q, locator %s", position, loc)
	}
}

func TestKosync_TheWebIsToldWhichDeviceStoppedThereByTheNameThePersonGaveIt(t *testing.T) {
	k := newKosyncStack(t)
	_, file, doc := k.book("Duna", "dune.epub", "epub")
	k.put(doc, "/body/DocFragment[2]/body/div/p[7]", 0.4, "KindleBasic3", "DEVICE1")

	var st ProgressState
	rec := k.do(ana, "GET", fmt.Sprintf("/progress/files/%d", file), "")
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Device != "Kindle" || st.Source != "koreader" || st.Revision != 1 {
		t.Errorf("progress for the web = %+v: the device named by the person, and that it came from a device that syncs", st)
	}
	// what the device is told back stays its own model, and the person reading on the web takes the source away
	if got := k.get(doc); got["device"] != "KindleBasic3" {
		t.Errorf("GET = %v", got)
	}
	k.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), `{"locator":{"type":"epub","cfi":"epubcfi(/6/4!/4/2/6)","href":"Text/dois%20capitulo.xhtml"},"percent":41,"device":"Web (Linux)"}`)
	rec = k.do(ana, "GET", fmt.Sprintf("/progress/files/%d", file), "")
	st = ProgressState{}
	json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Device != "Web (Linux)" || st.Source != "" {
		t.Errorf("after the web = %+v", st)
	}
}
