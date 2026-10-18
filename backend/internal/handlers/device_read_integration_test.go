package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/testdb"
)

func (s *authStack) deviceGet(user, secret, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, nil)
	if secret != "" {
		req.SetBasicAuth(user, secret)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestDeviceRead_TheAccessOfTheDeviceOpensTheHandlersOfTheWebAndOnlyForItsOwner(t *testing.T) {
	s := newAuthStack(t)
	anaID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	bobID := s.addUserWithPassword(t, "bob", "reader", "pw-bob")
	anaSession := s.login(t, "ana", "s3cret")

	dune, _, file := testdb.AddWork(t, s.db, testdb.Work{Title: "Duna", Path: "duna.epub", Format: "epub", Author: "Frank Herbert"})
	rosa, _, rosaFile := testdb.AddWork(t, s.db, testdb.Work{Title: "O Nome da Rosa", Path: "rosa.epub", Format: "epub", Author: "Umberto Eco"})
	if _, err := s.db.Exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, anaID, rosa); err != nil {
		t.Fatal(err)
	}
	for _, n := range []struct {
		user string
		work int
		file int64
		text string
	}{{anaID, dune, file, "frase de ana"}, {bobID, rosa, rosaFile, "frase de bob"}} {
		if _, err := s.db.Exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color)
			VALUES ($1, $2, $3, 'highlight', $4, '', '{}', '{"type":"epub","cfi":"epubcfi(/6/4)","href":"a.xhtml","progression":0.1}'::jsonb, 1, 'sepia')`,
			n.user, n.work, n.file, n.text); err != nil {
			t.Fatal(err)
		}
	}

	// the short access of a reader, as it is typed
	rec := s.req("POST", "/auth/app-tokens", anaSession, `{"name":"Kindle","scope":"koreader"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create the access: %d %s", rec.Code, rec.Body.String())
	}
	var created createAppTokenResponse
	json.Unmarshal(rec.Body.Bytes(), &created)
	secret := strings.ReplaceAll(created.Token, " ", "")

	// the library, filtered the way the web filters it
	rec = s.deviceGet("ana", secret, "/device/works?favorite=true")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "O Nome da Rosa") || strings.Contains(rec.Body.String(), "Duna") {
		t.Errorf("favorites with the access of the device: %d %s", rec.Code, rec.Body.String())
	}
	if rec = s.deviceGet("ana", secret, "/device/works"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Duna") {
		t.Errorf("all works: %d", rec.Code)
	}
	// the notes are the owner's and nobody else's
	rec = s.deviceGet("ana", secret, "/device/notes")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "frase de ana") || strings.Contains(rec.Body.String(), "frase de bob") {
		t.Errorf("notes: %d %s", rec.Code, rec.Body.String())
	}

	// not with a wrong secret, not with the password of the account, not for another user, not with nothing
	for _, c := range []struct{ user, secret string }{{"ana", "errada"}, {"ana", "s3cret"}, {"bob", secret}, {"", ""}} {
		if rec := s.deviceGet(c.user, c.secret, "/device/works"); rec.Code != http.StatusUnauthorized {
			t.Errorf("%q / %q: %d, want 401", c.user, c.secret, rec.Code)
		}
	}
	// and it is only for reading
	req := httptest.NewRequest("POST", "/device/works", strings.NewReader(`{}`))
	req.SetBasicAuth("ana", secret)
	post := httptest.NewRecorder()
	s.router.ServeHTTP(post, req)
	if post.Code != http.StatusMethodNotAllowed && post.Code != http.StatusNotFound {
		t.Errorf("writing through the read routes: %d", post.Code)
	}
}
