package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/sessions"
)

type codeAnswer struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int
	Interval                int
}

func (s *catalogStack) deviceCode(name string) codeAnswer {
	s.t.Helper()
	rec := s.do(actor{}, "POST", "/device/code", `{"client":"codice-ereader/0.1","platform":"kindle","name":"`+name+`"}`)
	if rec.Code != 200 {
		s.t.Fatalf("POST /device/code: %d %s", rec.Code, rec.Body.String())
	}
	var a codeAnswer
	json.Unmarshal(rec.Body.Bytes(), &a)
	return a
}

type pollAnswer struct {
	Error    string
	Interval int
	Token    string
	TokenID  string
	Username string
	Config   map[string]any
}

// poll asks for the access, after the gap the server asks the device to keep (the gap is taken out of the way by moving the last ask back).
func (s *catalogStack) poll(deviceCode string, waited bool) (int, pollAnswer) {
	s.t.Helper()
	if waited {
		s.exec(`UPDATE device_pairings SET last_poll_at = now() - interval '10 seconds' WHERE last_poll_at IS NOT NULL`)
	}
	rec := s.do(actor{}, "POST", "/device/token", `{"deviceCode":"`+deviceCode+`"}`)
	var a pollAnswer
	json.Unmarshal(rec.Body.Bytes(), &a)
	return rec.Code, a
}

func TestDevice_ARequestIsAnsweredWithTheCodeToShowAndTheOneToAskWith(t *testing.T) {
	s := newCatalogStack(t)
	a := s.deviceCode("Kindle da sala")
	if len(a.DeviceCode) < 32 || a.ExpiresIn != 600 || a.Interval != 5 {
		t.Errorf("answer = %+v", a)
	}
	if len(a.UserCode) != 9 || a.UserCode[4] != '-' {
		t.Errorf("the code to show comes in two groups: %q", a.UserCode)
	}
	plain := strings.ReplaceAll(a.UserCode, "-", "")
	if a.VerificationURI != "https://codice.example/?pair" || a.VerificationURIComplete != "https://codice.example/?pair="+plain {
		t.Errorf("addresses = %q %q", a.VerificationURI, a.VerificationURIComplete)
	}
	// the code to ask with is kept only as a hash
	if s.scalar(`SELECT count(*)::text FROM device_pairings WHERE encode(device_code_hash, 'hex') = $1`, a.DeviceCode) != "0" {
		t.Error("the code of the device is kept as it is")
	}
	if rec := s.do(actor{}, "GET", "/device/info", ""); !strings.Contains(rec.Body.String(), `"pairing":true`) {
		t.Errorf("info = %s", rec.Body.String())
	}
}

func TestDevice_TheFullFlowGivesTheAccessOnceToWhoApproved(t *testing.T) {
	s := newCatalogStack(t)
	a := s.deviceCode("Kindle da sala")

	code, p := s.poll(a.DeviceCode, false)
	if code != 400 || p.Error != "authorization_pending" {
		t.Fatalf("before the approval: %d %+v", code, p)
	}
	// asking again at once is too fast
	if code, p = s.poll(a.DeviceCode, false); p.Error != "slow_down" || p.Interval <= 5 {
		t.Errorf("a second ask at once: %d %+v", code, p)
	}

	// the person sees who is asking, and may name the device
	rec := s.do(ana, "GET", "/auth/device/"+strings.ToLower(a.UserCode), "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Kindle da sala") || !strings.Contains(rec.Body.String(), `"platform":"kindle"`) {
		t.Fatalf("show: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.do(ana, "POST", "/auth/device/"+a.UserCode+"/approve", `{"name":"Kindle da Ana"}`); rec.Code != 204 {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	// approved, it can no longer be shown or approved again
	if rec := s.do(bob, "POST", "/auth/device/"+a.UserCode+"/approve", `{}`); rec.Code != 404 {
		t.Errorf("approving twice: %d", rec.Code)
	}

	code, p = s.poll(a.DeviceCode, true)
	if code != 200 || len(p.Token) != 16 || strings.ContainsAny(p.Token, " -") || p.Username != "ana" {
		t.Fatalf("after the approval: %d %+v", code, p)
	}
	token := p.Token
	if p.Config["version"] != float64(1) {
		t.Errorf("the config comes with the access: %v", p.Config)
	}
	// the access is the short one of that account, called what the person called it, and it works the way KOReader uses it
	store := &sessions.Store{DB: s.db}
	if id, _, err := store.VerifyAppToken(t.Context(), "ana", p.Token); err != nil || id != idAna {
		t.Errorf("as the Basic password of the catalog: %v %q", err, id)
	}
	sum := md5.Sum([]byte(p.Token))
	if access, err := store.VerifyKosync(t.Context(), "ana", hex.EncodeToString(sum[:])); err != nil || access.Name != "Kindle da Ana" || access.UserID != idAna {
		t.Errorf("as the key of KOSync: %v %+v", err, access)
	}
	// delivered once
	if code, p = s.poll(a.DeviceCode, true); code != 400 || p.Error != "expired_token" || p.Token != "" {
		t.Errorf("a second delivery: %d %+v", code, p)
	}
	if s.scalar(`SELECT count(*)::text FROM app_tokens WHERE user_id = $1 AND scope = 'koreader'`, idAna) != "1" {
		t.Error("one access for one pairing")
	}
	// neither code nor the access is in the audit
	for _, secret := range []string{a.DeviceCode, token} {
		if s.scalar(`SELECT count(*)::text FROM audit_log WHERE details::text LIKE '%' || $1 || '%'`, secret) != "0" {
			t.Errorf("a secret is in the audit: %q", secret)
		}
	}
	for _, action := range []string{"device.pair.request", "device.pair.approve", "device.pair.token"} {
		if s.scalar(`SELECT count(*)::text FROM audit_log WHERE action = $1`, action) != "1" {
			t.Errorf("no audit entry for %s", action)
		}
	}
}

func TestDevice_ADeniedOrExpiredRequestGivesNothing(t *testing.T) {
	s := newCatalogStack(t)
	a := s.deviceCode("")
	if rec := s.do(ana, "POST", "/auth/device/"+a.UserCode+"/deny", ""); rec.Code != 204 {
		t.Fatalf("deny: %d", rec.Code)
	}
	if code, p := s.poll(a.DeviceCode, false); code != 400 || p.Error != "access_denied" {
		t.Errorf("denied: %d %+v", code, p)
	}
	b := s.deviceCode("")
	s.exec(`UPDATE device_pairings SET expires_at = now() - interval '1 minute' WHERE user_code = $1`, strings.ReplaceAll(b.UserCode, "-", ""))
	if code, p := s.poll(b.DeviceCode, false); p.Error != "expired_token" {
		t.Errorf("expired: %d %+v", code, p)
	}
	if rec := s.do(ana, "POST", "/auth/device/"+b.UserCode+"/approve", `{}`); rec.Code != 404 {
		t.Errorf("approving an expired one: %d", rec.Code)
	}
	if rec := s.do(ana, "GET", "/auth/device/"+b.UserCode, ""); rec.Code != 404 {
		t.Errorf("showing an expired one: %d", rec.Code)
	}
	// a code that is not one, or never was
	for _, bad := range []string{"ABCD", "00000000", "ZZZZ-ZZZZ"} {
		if rec := s.do(ana, "GET", "/auth/device/"+bad, ""); rec.Code != 404 {
			t.Errorf("code %q: %d", bad, rec.Code)
		}
	}
	if code, p := s.poll("0123456789abcdef0123456789abcdef0123456789abcdef", false); p.Error != "expired_token" {
		t.Errorf("a code that never was: %d %+v", code, p)
	}
	if s.scalar(`SELECT count(*)::text FROM app_tokens`) != "0" {
		t.Error("an access was made for a request nobody approved")
	}
}

func TestDevice_AnAddressDoesNotKeepMoreThanFiveRequestsWaiting(t *testing.T) {
	s := newCatalogStack(t)
	for i := 0; i < 5; i++ {
		s.deviceCode("")
	}
	rec := s.do(actor{}, "POST", "/device/code", `{}`)
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), "too_many_requests") {
		t.Errorf("the sixth: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDevice_WhatTheDeviceSaysAboutItselfIsCleaned(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(actor{}, "POST", "/device/code", `{"platform":"Kindle; DROP","client":"x\ny","name":"`+strings.Repeat("ã", 200)+`\u0007\n"}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT platform FROM device_pairings`); got != "other" {
		t.Errorf("platform = %q", got)
	}
	if got := s.scalar(`SELECT char_length(name)::text FROM device_pairings`); got != "60" {
		t.Errorf("name length = %s", got)
	}
	if s.scalar(`SELECT count(*)::text FROM device_pairings WHERE name ~ '[[:cntrl:]]' OR client ~ '[[:cntrl:]]'`) != "0" {
		t.Error("control characters were kept")
	}
}

func TestDevice_TheOwnerCanTurnPairingOff(t *testing.T) {
	s := newCatalogStack(t)
	if rec := s.do(admin, "PUT", "/admin/device-pairing", `{}`); rec.Code != 400 {
		t.Errorf("no value: %d", rec.Code)
	}
	if rec := s.do(admin, "PUT", "/admin/device-pairing", `{"enabled":false}`); rec.Code != 200 {
		t.Fatalf("turn off: %d", rec.Code)
	}
	if rec := s.do(actor{}, "GET", "/device/info", ""); !strings.Contains(rec.Body.String(), `"pairing":false`) {
		t.Errorf("info = %s", rec.Body.String())
	}
	if rec := s.do(actor{}, "POST", "/device/code", `{}`); rec.Code != 403 || !strings.Contains(rec.Body.String(), "pairing_disabled") {
		t.Errorf("asking while off: %d %s", rec.Code, rec.Body.String())
	}
	if s.scalar(`SELECT count(*)::text FROM audit_log WHERE action = 'device.pairing.configure'`) != "1" {
		t.Error("no audit entry")
	}
	s.do(admin, "PUT", "/admin/device-pairing", `{"enabled":true}`)
	if rec := s.do(actor{}, "POST", "/device/code", `{}`); rec.Code != 200 {
		t.Errorf("asking after turning on: %d", rec.Code)
	}
}

func TestDevice_TheConfigIsVersionedAndAnUnchangedOneIsNotSentAgain(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(ana, "GET", "/device/config", "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var cfg struct {
		Version  int
		Instance map[string]string
		OPDS     map[string]string
		KOSync   map[string]string
		Features map[string]bool
		MinPlug  string `json:"minPlugin"`
	}
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.Version != 1 || cfg.OPDS["url"] != "https://codice.example/opds/v1.2/catalog" || cfg.KOSync["url"] != "https://codice.example/kosync" || cfg.MinPlug == "" {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.Features["sendToKindle"] {
		t.Error("e-mail is off")
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	req := httptest.NewRequest("GET", "/device/config", nil)
	req.Header.Set("If-None-Match", etag)
	again := httptest.NewRecorder()
	s.router.ServeHTTP(again, req)
	if again.Code != 304 || again.Body.Len() != 0 {
		t.Errorf("same content: %d", again.Code)
	}
	// what the instance offers changes with the instance: e-mail turned on
	s.mailOn(25)
	rec = s.do(ana, "GET", "/device/config", "")
	if rec.Header().Get("ETag") == etag || !strings.Contains(rec.Body.String(), `"sendToKindle":true`) {
		t.Errorf("after turning e-mail on: %s %s", rec.Header().Get("ETag"), rec.Body.String())
	}
}

type homeAnswer struct {
	Version int
	User    struct{ Name string }
	Stats   struct {
		WorksTotal          int
		InProgressCount     int
		InProgressBreakdown FormatBreakdown
		CompletedBreakdown  FormatBreakdown
		TotalReadingSeconds int
	}
	Continue  []DeviceCard
	Favorites []DeviceCard
	Notes     []deviceQuote
	Shelves   []struct {
		ID    int64
		Title string
		Total int
		Works []DeviceCard
	}
}

func (s *catalogStack) home(a actor) homeAnswer {
	s.t.Helper()
	rec := s.do(a, "GET", "/device/home", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /device/home: %d %s", rec.Code, rec.Body.String())
	}
	var h homeAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &h); err != nil {
		s.t.Fatal(err)
	}
	return h
}

func TestDevice_TheHomeIsOneAnswerInTheOrderOfTheWebHome(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	rosa := s.addWork("O Nome da Rosa", "Umberto Eco", "rosa.epub", "epub")
	s.addWork("Outro", "Alguém", "outro.epub", "epub")
	fiction := s.makeCategory("Ficção", nil)
	other := s.makeCategory("Vazia", nil)
	s.exec(`INSERT INTO work_categories (work_id, category_id, source) VALUES ($1, $2, 'manual'), ($3, $2, 'manual')`, dune, fiction, rosa)

	// ana is reading Duna; and keeps Rosa as a favorite
	file := s.primaryFile(dune)
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), `{"locator":{"type":"epub","cfi":"epubcfi(/6/4)","href":"a.xhtml","progression":0.3},"percent":40,"device":"Web"}`); rec.Code != 200 {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	s.exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, idAna, rosa)

	h := s.home(ana)
	if h.Version != 1 || h.User.Name != "ana" || h.Stats.WorksTotal != 3 {
		t.Errorf("greeting and numbers = %+v", h)
	}
	if h.Stats.InProgressCount != 1 || h.Stats.InProgressBreakdown.Livros != 1 {
		t.Errorf("what is being read comes with its breakdown by format: %+v", h.Stats)
	}
	if len(h.Continue) != 1 || h.Continue[0].Title != "Duna" || h.Continue[0].Percent != 40 || h.Continue[0].FileID != file || !h.Continue[0].InProgress {
		t.Errorf("continue = %+v", h.Continue)
	}
	if len(h.Favorites) != 1 || h.Favorites[0].Title != "O Nome da Rosa" || h.Favorites[0].ID != rosa || h.Favorites[0].FileID != s.primaryFile(rosa) {
		t.Errorf("favorites = %+v", h.Favorites)
	}
	// the library as shelves: a category with works, with the works; a category with none is not a shelf
	if len(h.Shelves) != 1 || h.Shelves[0].ID != fiction || h.Shelves[0].Title != "Ficção" || len(h.Shelves[0].Works) != 2 || h.Shelves[0].Total != 2 {
		t.Errorf("shelves = %+v (empty one: %d)", h.Shelves, other)
	}
	for _, c := range h.Shelves[0].Works {
		if c.Cover == "" || c.FileID == 0 || c.Format != "epub" {
			t.Errorf("a card must be able to be drawn and opened: %+v", c)
		}
	}
}

func TestDevice_TheHomeIsOfWhoAsksAndShowsAShortQuote(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	file := s.primaryFile(dune)
	s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", file), `{"locator":{"type":"epub","cfi":"epubcfi(/6/4)","href":"a.xhtml","progression":0.3},"percent":40,"device":"Web"}`)
	long := strings.Repeat("palavra ", 100)
	s.exec(`INSERT INTO notes (user_id, work_id, file_id, kind, quote, body, tags, locator, locator_version, color)
		VALUES ($1, $2, $3, 'highlight', $4, '', '{}', '{"type":"epub","cfi":"epubcfi(/6/4!/4/2)","href":"a.xhtml","progression":0.3}'::jsonb, 1, 'sepia')`,
		idAna, dune, file, long)
	h := s.home(ana)
	if len(h.Notes) != 1 || h.Notes[0].Title != "Duna" || len([]rune(h.Notes[0].Text)) > 281 || !strings.HasSuffix(h.Notes[0].Text, "…") {
		t.Errorf("notes = %+v", h.Notes)
	}
	// bob has read nothing and has no notes: none of ana's reading or notes is his
	other := s.home(bob)
	if len(other.Continue) != 0 || len(other.Notes) != 0 || len(other.Favorites) != 0 {
		t.Errorf("bob's home = %+v", other)
	}
}

func TestDevice_AWorkListsItsFilesWithTheMainOneAndTheLastRead(t *testing.T) {
	s := newCatalogStack(t)
	dune := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	enEdition := s.addEdition(dune, "en")
	pdf := s.addFile(enEdition, "pdf", "duna-en.pdf", "managed")
	main := s.primaryFile(dune)
	// ana read the PDF last
	if rec := s.do(ana, "PUT", fmt.Sprintf("/progress/files/%d", pdf), `{"locator":{"type":"pdf","page":10},"percent":20,"device":"Web"}`); rec.Code != 200 {
		t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	rec := s.do(ana, "GET", fmt.Sprintf("/device/works/%d/files", dune), "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Title string
		Files []deviceFile
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Title != "Duna" || len(got.Files) != 2 {
		t.Fatalf("files = %+v", got)
	}
	by := map[int64]deviceFile{}
	for _, f := range got.Files {
		by[f.ID] = f
	}
	if !by[main].Primary || by[main].Last || by[main].Format != "epub" {
		t.Errorf("the main file = %+v", by[main])
	}
	if by[pdf].Primary || !by[pdf].Last || by[pdf].Language != "en" || by[pdf].Percent != 20 {
		t.Errorf("the pdf = %+v", by[pdf])
	}
	if rec := s.do(ana, "GET", "/device/works/99999/files", ""); rec.Code != 404 {
		t.Errorf("no such work: %d", rec.Code)
	}
	// the card of the home says there is a choice
	h := s.home(ana)
	if len(h.Continue) != 1 || h.Continue[0].FileCount != 2 {
		t.Errorf("continue = %+v", h.Continue)
	}
}
