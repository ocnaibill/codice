package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/testdb"
)

// seedPerson gives an account a bit of everything that is theirs, and returns what a stranger must never find in their file.
func (s *authStack) seedPerson(t *testing.T, name string, id string, tag string, linked bool) {
	t.Helper()
	work, _, file := testdb.AddWork(t, s.db, testdb.Work{Title: "Livro de " + name, Author: "Autor de " + name, Path: name + "/livro.epub", Format: "epub"})
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	exec(`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`, id, work)
	exec(`INSERT INTO work_ratings (user_id, work_id, stars) VALUES ($1, $2, 4)`, id, work)
	exec(`INSERT INTO notes (user_id, work_id, source_title, source_author, file_id, kind, quote, body, tags, color, locator, locator_version)
		VALUES ($1, $2, $3, $4, $5, 'note', $6, $7, ARRAY[$8, 'comum'], 'sage', '{"type":"epub","href":"c1.xhtml","cfi":"epubcfi(/6/2)"}', 1)`,
		id, work, "Livro de "+name, "Autor de "+name, file, "Citação de "+name, "Minha nota "+name, tag)
	exec(`INSERT INTO reading_progress (user_id, file_id, position, locator, locator_version, percent_complete, completed_at, reading_seconds, device)
		VALUES ($1, $2, '', '{"type":"epub","href":"c3.xhtml"}', 1, 42.5, now(), 600, 'Celular de '||$3)`, id, file, name)
	exec(`INSERT INTO reading_completions (user_id, work_id, file_id, format) VALUES ($1, $2, $3, 'epub')`, id, work, file)
	exec(`INSERT INTO work_reading_state (user_id, work_id, finished_at) VALUES ($1, $2, now())`, id, work)
	exec(`INSERT INTO concepts (user_id, name, description, aliases) VALUES ($1, $2, 'Descrição', ARRAY['apelido'])`, id, "Conceito de "+name)
	exec(`INSERT INTO sessions (user_id, expires_at, user_agent, ip, last_seen_at) VALUES ($1, now() + interval '1 day', $2, $3, now())`, id, "Navegador de "+name, "203.0.113."+strings.Repeat("7", 1))
	exec(`INSERT INTO sessions (user_id, expires_at, user_agent, revoked_at) VALUES ($1, now() + interval '1 day', 'SessaoEncerrada-'||$2, now())`, id, name)
	exec(`INSERT INTO app_tokens (user_id, name, token_hash) VALUES ($1, $2, $3)`, id, "KOReader de "+name, "SEGREDO-DO-TOKEN-"+name)
	exec(`INSERT INTO app_tokens (user_id, name, token_hash, revoked_at) VALUES ($1, $2, 'SEGREDO-REVOGADO-'||$3, now())`, id, "Antigo de "+name, name)
	exec(`INSERT INTO login_events (result, method, user_id, ip) VALUES ('bad_password', 'local', $1, '198.51.100.9')`, id)
	if linked {
		exec(`INSERT INTO external_identities (user_id, provider, subject) VALUES ($1, 'ldap', $2)`, id, "UUID-DO-DIRETORIO-"+name)
	}
	exec(`UPDATE users SET name_order = 'family_first', reader_prefs = '{"theme":"sepia","font":"opendyslexic"}' WHERE id = $1`, id)
}

// zipOf reads a ZIP into name -> content.
func unzipAll(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func buildFor(t *testing.T, s *authStack, userID string) map[string]string {
	t.Helper()
	var buf bytes.Buffer
	if err := BuildDataExport(context.Background(), s.db, userID, &buf, time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return unzipAll(t, buf.Bytes())
}

var exportFiles = []string{
	"LEIA-ME.txt", "anotacoes.json", "anotacoes.md", "aplicativos.json", "avaliacoes.json", "conceitos.json", "conta.json", "entradas.json",
	"favoritos.json", "leitura.json", "relacoes.json", "sessoes.json", "tags.json",
}

func TestDataExport_HoldsEverythingThatIsThePersonsAndNothingOfAnyoneElse(t *testing.T) {
	s := newAuthStack(t)
	ana := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	bia := s.addUserWithPassword(t, "bia", "reader", "s3cret")
	s.db.Exec(`UPDATE users SET display_name = 'Aninha' WHERE id = $1`, ana)
	s.seedPerson(t, "ana", ana, "etiqueta-da-ana", true)
	s.seedPerson(t, "bia", bia, "etiqueta-da-bia", true)

	files := buildFor(t, s, ana)
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != strings.Join(exportFiles, ",") {
		t.Fatalf("files = %v, want %v", names, exportFiles)
	}
	for name, body := range files {
		if strings.HasSuffix(name, ".json") && !json.Valid([]byte(body)) {
			t.Errorf("%s is not valid JSON", name)
		}
	}
	all := strings.Join(func() []string {
		var parts []string
		for _, n := range exportFiles {
			parts = append(parts, files[n])
		}
		return parts
	}(), "\n")

	// What is hers is there.
	for name, wants := range map[string][]string{
		"conta.json":       {`"username": "ana"`, `"displayName": "Aninha"`, `"nameOrder": "family_first"`, `"theme": "sepia"`, `"provider": "ldap"`, `"role": "reader"`},
		"anotacoes.md":     {"Livro de ana", "Minha nota ana", "Citação de ana", "#etiqueta-da-ana"},
		"anotacoes.json":   {`"body": "Minha nota ana"`, `"color": "sage"`, `"count": 1`, `"cfi": "epubcfi(/6/2)"`},
		"favoritos.json":   {`"title": "Livro de ana"`, `"authors": "Autor de ana"`, `"inLibrary": true`},
		"avaliacoes.json":  {`"title": "Livro de ana"`, `"stars": 4`, `"inLibrary": true`},
		"leitura.json":     {`"percent": 42.5`, `"readingSeconds": 600`, `"device": "Celular de ana"`, `"title": "Livro de ana"`, `"completedAt"`, `"finishedAt"`, `"obrasFinalizadas"`, `"conclusoes"`, `"href": "c3.xhtml"`},
		"conceitos.json":   {`"name": "Conceito de ana"`, `"apelido"`},
		"tags.json":        {`"name": "etiqueta-da-ana"`, `"name": "comum"`},
		"sessoes.json":     {`"device": "Navegador de ana"`, `"ip": "203.0.113.7"`},
		"aplicativos.json": {`"name": "KOReader de ana"`, `"name": "Antigo de ana"`, `"revokedAt"`},
		"entradas.json":    {`"result": "bad_password"`, `"ip": "198.51.100.9"`},
		"LEIA-ME.txt":      {`para a conta "ana"`, "04/10/2026 15:30", "NÃO está aqui", "24 horas"},
	} {
		for _, want := range wants {
			if !strings.Contains(files[name], want) {
				t.Errorf("%s does not hold %q:\n%s", name, want, files[name])
			}
		}
	}
	// What is Bia's is not, in any file.
	for _, leak := range []string{"bia", "Bia", "etiqueta-da-bia", "Livro de bia", "Minha nota bia", "Navegador de bia", "KOReader de bia", "Conceito de bia"} {
		if strings.Contains(all, leak) {
			t.Errorf("another account's %q is in the file", leak)
		}
	}
	// No secret, whoever's: no hash of a password or of a token, no directory subject, no session that was ended.
	for _, secret := range []string{"SEGREDO", "password_hash", "passwordHash", "tokenHash", "token_hash", "UUID-DO-DIRETORIO", "SessaoEncerrada", "$2a$", "ssoId", "sso_id"} {
		if strings.Contains(all, secret) {
			t.Errorf("%q is in the file", secret)
		}
	}
	if strings.Contains(files["sessoes.json"], "Encerrada") {
		t.Error("a session that was ended is listed as open")
	}
}

func TestDataExport_ForAnAccountWithNothingEveryListIsEmptyNotMissing(t *testing.T) {
	s := newAuthStack(t)
	id := s.addUserWithPassword(t, "nova", "reader", "s3cret")
	files := buildFor(t, s, id)
	for _, name := range []string{"favoritos.json", "avaliacoes.json", "conceitos.json", "relacoes.json", "tags.json", "sessoes.json", "aplicativos.json", "entradas.json"} {
		if strings.TrimSpace(files[name]) != "[]" {
			t.Errorf("%s = %q, want []", name, files[name])
		}
	}
	var leitura map[string][]any
	if err := json.Unmarshal([]byte(files["leitura.json"]), &leitura); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"progresso", "conclusoes", "obrasFinalizadas", "posicoesEquivalentes"} {
		if v, ok := leitura[k]; !ok || v == nil || len(v) != 0 {
			t.Errorf("leitura.%s = %v, want []", k, v)
		}
	}
	var notes struct {
		Count int   `json:"count"`
		Notes []any `json:"notes"`
	}
	json.Unmarshal([]byte(files["anotacoes.json"]), &notes)
	if notes.Count != 0 || notes.Notes == nil {
		t.Errorf("notes = %+v, want an empty list, not null", notes)
	}
	if !strings.Contains(files["anotacoes.md"], "0 anotações") {
		t.Errorf("anotacoes.md = %q", files["anotacoes.md"])
	}
}

func TestDataExport_AnAccountThatIsGoneFailsInsteadOfMakingAnEmptyFile(t *testing.T) {
	s := newAuthStack(t)
	var buf bytes.Buffer
	if err := BuildDataExport(context.Background(), s.db, "00000000-0000-0000-0000-000000000000", &buf, time.Now()); err == nil {
		t.Error("an account that does not exist got a file")
	}
}

func (s *authStack) exportRequest(t *testing.T, token string) (int, string) {
	t.Helper()
	rec := s.req("POST", "/auth/export", token, "")
	var out struct{ ID string }
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out.ID
}

type exportState struct {
	Export *struct {
		ID        string  `json:"id"`
		State     string  `json:"state"`
		Bytes     int64   `json:"bytes"`
		ExpiresAt *string `json:"expiresAt"`
	} `json:"export"`
}

func (s *authStack) exportState(t *testing.T, token string) exportState {
	t.Helper()
	rec := s.req("GET", "/auth/export", token, "")
	if rec.Code != 200 {
		t.Fatalf("latest: %d", rec.Code)
	}
	var out exportState
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestDataExport_RequestMakesTheFileAndOnlyTheOwnerTakesIt(t *testing.T) {
	s := newAuthStack(t)
	ana := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.addUserWithPassword(t, "bia", "reader", "s3cret")
	s.exports.Dir = filepath.Join(s.exports.Dir, "exportacoes") // made by the job, not by the test
	s.seedPerson(t, "ana", ana, "etiqueta-da-ana", false)
	anaTok := s.login(t, "ana", "s3cret")
	biaTok := s.login(t, "bia", "s3cret")

	if st := s.exportState(t, anaTok); st.Export != nil {
		t.Fatalf("before any request: %+v", st.Export)
	}
	code, id := s.exportRequest(t, anaTok)
	if code != http.StatusAccepted || id == "" {
		t.Fatalf("request: %d %q", code, id)
	}
	if st := s.exportState(t, anaTok); st.Export == nil || st.Export.State != "pending" {
		t.Fatalf("after the request: %+v", st.Export)
	}
	if s.scalar(t, `SELECT count(*) FROM jobs WHERE type = 'export_data' AND max_attempts = 1 AND payload->>'export_id' = $1 AND created_by = (SELECT id FROM users WHERE username = 'ana')`, id) != "1" {
		t.Error("the job is not what was asked")
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'data_export.request' AND actor_id = $1`, ana) != "1" {
		t.Error("the request was not audited, with who asked")
	}
	if rec := s.req("GET", "/auth/export/"+id+"/download", anaTok, ""); rec.Code != http.StatusGone {
		t.Errorf("download before it is ready: %d, want 410", rec.Code)
	}

	if err := s.exports.Prepare(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	st := s.exportState(t, anaTok)
	if st.Export == nil || st.Export.State != "ready" || st.Export.Bytes <= 0 || st.Export.ExpiresAt == nil {
		t.Fatalf("after the job: %+v", st.Export)
	}
	if info, err := os.Stat(filepath.Join(s.exports.Dir, id+".zip")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file: %v %v, want a file only the server can read", info, err)
	}
	if info, err := os.Stat(s.exports.Dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the folder: %v %v, want one only the server can enter", info, err)
	}

	rec := s.req("GET", "/auth/export/"+id+"/download", anaTok, "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" || rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		rec.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="codice-meus-dados-`) {
		t.Fatalf("download: %d %v", rec.Code, rec.Header())
	}
	if files := unzipAll(t, rec.Body.Bytes()); !strings.Contains(files["anotacoes.md"], "Minha nota ana") {
		t.Errorf("the file taken is not hers: %v", files["anotacoes.md"])
	}
	if s.scalar(t, `SELECT (downloaded_at IS NOT NULL)::text FROM data_exports WHERE id = $1`, id) != "true" || s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'data_export.download'`) != "1" {
		t.Error("the download was not noted")
	}

	// Bia cannot take it, or learn that it exists; nobody without a session can.
	if rec := s.req("GET", "/auth/export/"+id+"/download", biaTok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("another account's file: %d, want 404", rec.Code)
	}
	if rec := s.req("GET", "/auth/export/"+id+"/download", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d, want 401", rec.Code)
	}
	if st := s.exportState(t, biaTok); st.Export != nil {
		t.Errorf("Bia sees an export that is not hers: %+v", st.Export)
	}
	if rec := s.req("DELETE", "/auth/export/"+id, biaTok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("another account's delete: %d, want 404", rec.Code)
	}
	if rec := s.req("GET", "/auth/export/nao-e-um-id/download", anaTok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("a malformed id: %d, want 404", rec.Code)
	}
}

func TestDataExport_OneAtATimeAndAFewAnHourAndANewOneReplacesTheOld(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")

	code, first := s.exportRequest(t, tok)
	if code != http.StatusAccepted {
		t.Fatalf("first: %d", code)
	}
	rec := s.req("POST", "/auth/export", tok, "")
	var busy struct{ ID string }
	json.Unmarshal(rec.Body.Bytes(), &busy)
	if rec.Code != http.StatusConflict || busy.ID != first {
		t.Errorf("a second request while one is being made: %d %s, want 409 with the one in progress", rec.Code, rec.Body.String())
	}
	if s.scalar(t, `SELECT count(*) FROM jobs WHERE type = 'export_data'`) != "1" {
		t.Error("a second job was queued")
	}

	s.exports.Prepare(context.Background(), first)
	firstFile := filepath.Join(s.exports.Dir, first+".zip")
	if _, err := os.Stat(firstFile); err != nil {
		t.Fatal(err)
	}
	code, second := s.exportRequest(t, tok)
	if code != http.StatusAccepted || second == first {
		t.Fatalf("second: %d %s", code, second)
	}
	if _, err := os.Stat(firstFile); err == nil {
		t.Error("the old file stayed after a new request replaced it")
	}
	if s.scalar(t, `SELECT count(*) FROM data_exports`) != "2" || s.scalar(t, `SELECT count(*) FROM data_exports WHERE state = 'ready' AND expires_at <= now()`) != "1" {
		t.Error("the first request must stay on record, over, so that the hour is counted")
	}
	s.exports.Prepare(context.Background(), second)
	code, third := s.exportRequest(t, tok)
	if code != http.StatusAccepted {
		t.Fatalf("third: %d", code)
	}
	s.exports.Prepare(context.Background(), third)
	rec = s.req("POST", "/auth/export", tok, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("the fourth in an hour: %d, want 429", rec.Code)
	}
	// An hour later it is allowed again.
	s.db.Exec(`UPDATE data_exports SET requested_at = now() - interval '61 minutes'`)
	if code, _ := s.exportRequest(t, tok); code != http.StatusAccepted {
		t.Errorf("after an hour: %d, want 202", code)
	}
}

func TestDataExport_ADeadRequestDoesNotBlockTheNext(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	_, id := s.exportRequest(t, tok)
	s.db.Exec(`UPDATE data_exports SET requested_at = now() - interval '3 hours' WHERE id = $1`, id)
	if st := s.exportState(t, tok); st.Export == nil || st.Export.State != "failed" {
		t.Errorf("a request that never answered must read as failed: %+v", st.Export)
	}
	if code, _ := s.exportRequest(t, tok); code != http.StatusAccepted {
		t.Errorf("a new request after a dead one: %d", code)
	}
}

func TestDataExport_PastItsDayOrGoneItIsExpiredAndNotGiven(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	_, id := s.exportRequest(t, tok)
	s.exports.Prepare(context.Background(), id)

	s.db.Exec(`UPDATE data_exports SET expires_at = now() - interval '1 minute' WHERE id = $1`, id)
	if st := s.exportState(t, tok); st.Export.State != "expired" {
		t.Errorf("past its day: %+v", st.Export)
	}
	if rec := s.req("GET", "/auth/export/"+id+"/download", tok, ""); rec.Code != http.StatusGone {
		t.Errorf("download past its day: %d, want 410", rec.Code)
	}

	s.db.Exec(`UPDATE data_exports SET expires_at = now() + interval '1 hour' WHERE id = $1`, id)
	if st := s.exportState(t, tok); st.Export.State != "ready" {
		t.Fatalf("within its day: %+v", st.Export)
	}
	os.Remove(filepath.Join(s.exports.Dir, id+".zip")) // a restart took it
	if st := s.exportState(t, tok); st.Export.State != "expired" {
		t.Errorf("the file is gone: %+v", st.Export)
	}
	if rec := s.req("GET", "/auth/export/"+id+"/download", tok, ""); rec.Code != http.StatusGone {
		t.Errorf("download of a file that is gone: %d, want 410", rec.Code)
	}
}

func TestDataExport_ThePersonCanRemoveTheFileBeforeTheDayIsOut(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	_, id := s.exportRequest(t, tok)
	s.exports.Prepare(context.Background(), id)
	file := filepath.Join(s.exports.Dir, id+".zip")
	if rec := s.req("DELETE", "/auth/export/"+id, tok, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("the file stayed")
	}
	if st := s.exportState(t, tok); st.Export != nil {
		t.Errorf("the record stayed: %+v", st.Export)
	}
	if rec := s.req("DELETE", "/auth/export/"+id, tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("again: %d, want 404", rec.Code)
	}
}

// A request that a newer one replaced must not come back to the screen when the newer one is removed: the person removed their
// file, and sees none, not "the previous one expired".
func TestDataExport_RemovingTheNewFileDoesNotShowTheOneItReplaced(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	_, first := s.exportRequest(t, tok)
	s.exports.Prepare(context.Background(), first)
	_, second := s.exportRequest(t, tok)
	if st := s.exportState(t, tok); st.Export == nil || st.Export.ID != second || st.Export.State != "pending" {
		t.Fatalf("the screen must show the new request: %+v", st.Export)
	}
	s.exports.Prepare(context.Background(), second)
	if rec := s.req("DELETE", "/auth/export/"+second, tok, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if st := s.exportState(t, tok); st.Export != nil {
		t.Errorf("after removing the file the screen shows %+v, want none", st.Export)
	}
	// The replaced one still counts for the hour.
	if s.scalar(t, `SELECT count(*) FROM data_exports WHERE superseded`) != "1" {
		t.Error("the replaced request must stay on record for the limit")
	}
}

func TestDataExport_AJobThatCannotWriteFailsOnceAndTheScreenSaysOnlyThat(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	_, id := s.exportRequest(t, tok)

	blocker := filepath.Join(t.TempDir(), "arquivo")
	os.WriteFile(blocker, []byte("x"), 0o600)
	s.exports.Dir = filepath.Join(blocker, "dentro") // a folder that cannot be made: its parent is a file
	err := s.exports.Prepare(context.Background(), id)
	if err == nil {
		t.Fatal("a job that could not write succeeded")
	}
	st := s.exportState(t, tok)
	if st.Export == nil || st.Export.State != "failed" {
		t.Fatalf("state = %+v", st.Export)
	}
	rec := s.req("GET", "/auth/export", tok, "")
	if strings.Contains(rec.Body.String(), "not a directory") || strings.Contains(rec.Body.String(), blocker) || strings.Contains(rec.Body.String(), "error") {
		t.Errorf("the screen is told what the server said: %s", rec.Body.String())
	}
	if s.scalar(t, `SELECT COALESCE(error, '') FROM data_exports WHERE id = $1`, id) == "" {
		t.Error("the reason was not kept for the logs")
	}
	if err := s.exports.Prepare(context.Background(), id); err == nil {
		t.Error("a request that already failed was run again")
	}
	if err := s.exports.Prepare(context.Background(), "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("a request that does not exist was run")
	}
}

func TestDataExport_PurgeRemovesWhatIsOldAndKeepsWhatIsLive(t *testing.T) {
	s := newAuthStack(t)
	ana := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	bia := s.addUserWithPassword(t, "bia", "reader", "s3cret")
	cai := s.addUserWithPassword(t, "cai", "reader", "s3cret")
	dir := s.exports.Dir
	os.MkdirAll(dir, 0o700)
	row := func(user, state, extra string) string {
		var id string
		if err := s.db.QueryRow(`INSERT INTO data_exports (user_id, state`+extra+`) VALUES ($1, $2`+strings.Repeat("", 0)+`) RETURNING id`, user, state).Scan(&id); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, id+".zip"), []byte("zip"), 0o600)
		return id
	}
	expired := row(ana, "ready", "")
	s.db.Exec(`UPDATE data_exports SET expires_at = now() - interval '1 minute' WHERE id = $1`, expired)
	live := row(bia, "ready", "")
	s.db.Exec(`UPDATE data_exports SET expires_at = now() + interval '5 hours' WHERE id = $1`, live)
	// A file that is hours old but still has a request that owns it must stay: age alone does not make a file a stray.
	os.Chtimes(filepath.Join(dir, live+".zip"), time.Now().Add(-3*time.Hour), time.Now().Add(-3*time.Hour))
	oldFailed := row(cai, "failed", "")
	s.db.Exec(`UPDATE data_exports SET requested_at = now() - interval '8 days' WHERE id = $1`, oldFailed)
	newFailed := row(ana, "failed", "")
	stalePending := row(bia, "pending", "")
	s.db.Exec(`UPDATE data_exports SET requested_at = now() - interval '3 hours' WHERE id = $1`, stalePending)

	// Files nobody owns: an old one goes, a young one (being written) stays, and so does a stranger's file name.
	stray := filepath.Join(dir, "00000000-0000-0000-0000-00000000aaaa.zip")
	os.WriteFile(stray, []byte("x"), 0o600)
	os.Chtimes(stray, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))
	young := filepath.Join(dir, "00000000-0000-0000-0000-00000000bbbb.zip")
	os.WriteFile(young, []byte("x"), 0o600)
	partial := filepath.Join(dir, live+".zip.partial")
	os.WriteFile(partial, []byte("x"), 0o600)
	os.Chtimes(partial, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))

	longGone := row(cai, "ready", "")
	s.db.Exec(`UPDATE data_exports SET expires_at = now() - interval '2 hours' WHERE id = $1`, longGone)

	n, err := s.exports.Purge(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("purged %d, %v; want 2 (the one over for two hours and the old failure)", n, err)
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	for name, want := range map[string]bool{
		"expired file": false, "long gone file": false, "live file": true, "old failed file": false, "new failed file": true, "stale pending file": true,
		"old stray": false, "young stray": true, "old partial": false,
	} {
		path := map[string]string{
			"expired file": filepath.Join(dir, expired+".zip"), "long gone file": filepath.Join(dir, longGone+".zip"), "live file": filepath.Join(dir, live+".zip"),
			"old failed file": filepath.Join(dir, oldFailed+".zip"), "new failed file": filepath.Join(dir, newFailed+".zip"),
			"stale pending file": filepath.Join(dir, stalePending+".zip"), "old stray": stray, "young stray": young, "old partial": partial,
		}[name]
		if exists(path) != want {
			t.Errorf("%s: exists = %v, want %v", name, exists(path), want)
		}
	}
	if s.scalar(t, `SELECT state FROM data_exports WHERE id = $1`, stalePending) != "failed" {
		t.Error("a request that never answered was not marked failed")
	}
	// The one that ended a minute ago lost its file but keeps its row for the hour (the limit on requests counts rows).
	if s.scalar(t, `SELECT count(*) FROM data_exports`) != "4" {
		t.Errorf("rows left = %s, want 4", s.scalar(t, `SELECT count(*) FROM data_exports`))
	}
	if s.scalar(t, `SELECT count(*) FROM data_exports WHERE id = $1`, expired) != "1" {
		t.Error("the row of a request that ended a minute ago was deleted: it must stay for the hour")
	}
}

func TestDataExport_DeletingTheAccountDeletesItsRequests(t *testing.T) {
	s := newAuthStack(t)
	ana := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	s.exportRequest(t, tok)
	s.db.Exec(`DELETE FROM users WHERE id = $1`, ana)
	if s.scalar(t, `SELECT count(*) FROM data_exports`) != "0" {
		t.Error("a request outlived its account")
	}
}

func TestDataExport_NotesPastTheLimitAreSaidToBeCut(t *testing.T) {
	s := newAuthStack(t)
	id := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	work, _, file := testdb.AddWork(t, s.db, testdb.Work{Title: "Livro", Author: "Autor", Path: "a/l.epub", Format: "epub"})
	if _, err := s.db.Exec(`INSERT INTO notes (user_id, work_id, source_title, source_author, file_id, kind, body, tags, locator, locator_version)
		SELECT $1, $2, 'Livro', 'Autor', $3, 'note', 'nota ' || g, ARRAY[]::text[], '{"type":"epub","href":"c1.xhtml"}', 1 FROM generate_series(1, $4) g`,
		id, work, file, exportCap+3); err != nil {
		t.Fatal(err)
	}
	files := buildFor(t, s, id)
	if !strings.Contains(files["LEIA-ME.txt"], fmt.Sprintf("limite de %d", exportCap)) {
		t.Error("a file that leaves notes out does not say so")
	}
	var notes struct{ Count int }
	json.Unmarshal([]byte(files["anotacoes.json"]), &notes)
	if notes.Count != exportCap {
		t.Errorf("notes = %d, want the cap (%d)", notes.Count, exportCap)
	}
	// And one under the limit says nothing of the sort.
	s.db.Exec(`DELETE FROM notes WHERE id IN (SELECT id FROM notes LIMIT 5)`)
	s.db.Exec(`DELETE FROM notes WHERE id IN (SELECT id FROM notes ORDER BY id LIMIT $1)`, exportCap-10)
	if files := buildFor(t, s, id); strings.Contains(files["LEIA-ME.txt"], "limite de") {
		t.Error("a file that has everything talks of a limit")
	}
}

func TestDataExport_TheFileIsGoodForAdayAndNoMore(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	tok := s.login(t, "ana", "s3cret")
	_, id := s.exportRequest(t, tok)
	s.exports.Prepare(context.Background(), id)
	var hours float64
	if err := s.db.QueryRow(`SELECT extract(epoch FROM expires_at - ready_at) / 3600 FROM data_exports WHERE id = $1`, id).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if hours < 23.99 || hours > 24.01 {
		t.Errorf("the file is kept %.2f hours, want 24", hours)
	}
	if ExportTTL != 24*time.Hour {
		t.Errorf("ExportTTL = %v", ExportTTL)
	}
}
