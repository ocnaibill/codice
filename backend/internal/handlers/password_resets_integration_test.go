package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type approved struct{ ID, Token string }

func (s *authStack) askReset(username string) (int, string) {
	rec := s.req("POST", "/auth/reset-request", "", fmt.Sprintf(`{"username":%q}`, username))
	return rec.Code, rec.Body.String()
}

func (s *authStack) resetID(t *testing.T, username string) string {
	t.Helper()
	return s.scalar(t, `SELECT p.id FROM password_resets p JOIN users u ON u.id = p.user_id WHERE u.username = $1 ORDER BY p.created_at DESC LIMIT 1`, username)
}

func (s *authStack) approve(t *testing.T, bearer, id string, want int) approved {
	t.Helper()
	rec := s.req("POST", "/password-resets/"+id+"/approve", bearer, "")
	if rec.Code != want {
		t.Fatalf("approve: %d %s, want %d", rec.Code, rec.Body.String(), want)
	}
	var out approved
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (s *authStack) useReset(token, password string) int {
	return s.req("POST", "/auth/reset", "", fmt.Sprintf(`{"token":%q,"password":%q}`, token, password)).Code
}

func TestResetRequest_AnswersTheSameForEveryoneAndRecordsOnlyWhatCanBeReset(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	blockedID := s.addUserWithPassword(t, "blk", "reader", "s3cret")
	noPassID := s.addUserWithPassword(t, "ldap", "reader", "s3cret")
	s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, blockedID)
	s.db.Exec(`UPDATE users SET password_hash = NULL WHERE id = $1`, noPassID)

	answers := map[string]bool{}
	for _, name := range []string{"ana", "nao-existe", "boss", "blk", "ldap", "", "  "} {
		code, body := s.askReset(name)
		if code != http.StatusAccepted {
			t.Errorf("request for %q: %d, want 202", name, code)
		}
		answers[body] = true
	}
	if len(answers) != 1 {
		t.Errorf("the answers differ, so they reveal which accounts exist: %v", answers)
	}
	if got := s.scalar(t, `SELECT string_agg(u.username, ',') FROM password_resets p JOIN users u ON u.id = p.user_id`); got != "ana" {
		t.Errorf("requests recorded for %q; only ana can be reset from here", got)
	}
}

func TestResetRequest_RepeatedRequestsBecomeOne(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	for i := 0; i < 5; i++ {
		s.askReset("ana")
	}
	if n := s.scalar(t, `SELECT count(*) FROM password_resets`); n != "1" {
		t.Errorf("%s requests, want 1", n)
	}
	if n := s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'password_reset.request'`); n != "1" {
		t.Errorf("audited %s times, want 1", n)
	}

	// One that waited a week has lapsed and is replaced by the new one.
	s.db.Exec(`UPDATE password_resets SET created_at = now() - interval '8 days'`)
	s.askReset("ana")
	if got := s.scalar(t, `SELECT string_agg(coalesce(decision, 'open'), ',' ORDER BY created_at) FROM password_resets`); got != "expired,open" {
		t.Errorf("requests = %s", got)
	}
}

func TestReset_FullFlowEndsAllAccessAndTheLinkWorksOnce(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "adm", "admin", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "senha-antiga")
	adm := s.login(t, "adm", "s3cret")
	ana := s.login(t, "ana", "senha-antiga")
	_, appToken := createAppToken(t, s, ana, "kobo")

	s.askReset("ana")
	id := s.resetID(t, "ana")
	link := s.approve(t, adm, id, 200)
	if len(link.Token) < 40 {
		t.Fatalf("token too short to be a secret: %q", link.Token)
	}
	// Nothing changes until the link is used.
	if rec := s.req("GET", "/probe", ana, ""); rec.Code != 200 {
		t.Fatalf("approving alone ended the session: %d", rec.Code)
	}
	if rec := s.req("GET", "/auth/reset?token="+link.Token, "", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"username":"ana"`) {
		t.Fatalf("check: %d %s", rec.Code, rec.Body.String())
	}

	if code := s.useReset(link.Token, "curta"); code != 400 {
		t.Errorf("short password: %d, want 400", code)
	}
	if code := s.useReset(link.Token, "senha-nova-1"); code != http.StatusNoContent {
		t.Fatalf("use: %d", code)
	}
	if rec := s.req("GET", "/probe", ana, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("old session after the reset: %d, want 401", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("old app token after the reset: %d, want 401", rec.Code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"senha-antiga"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("the old password still works: %d", rec.Code)
	}
	if fresh := s.login(t, "ana", "senha-nova-1"); fresh == "" {
		t.Error("the new password does not work")
	}

	// Single use.
	if code := s.useReset(link.Token, "outra-senha-9"); code != http.StatusNotFound {
		t.Errorf("second use: %d, want 404", code)
	}
	if rec := s.req("GET", "/auth/reset?token="+link.Token, "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("check after use: %d, want 404", rec.Code)
	}

	if got := s.scalar(t, `SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'password_reset.%'`); got != "password_reset.request,password_reset.approve,password_reset.use" {
		t.Errorf("audit = %q", got)
	}
	dump := s.scalar(t, `SELECT (SELECT coalesce(string_agg(p::text, ' '), '') FROM password_resets p) || (SELECT coalesce(string_agg(details::text, ' '), '') FROM audit_log)`)
	if strings.Contains(dump, link.Token) || strings.Contains(dump, "senha-nova") {
		t.Error("a secret reached the database or the audit log")
	}
	if strings.Contains(s.req("GET", "/password-resets", adm, "").Body.String(), link.Token) {
		t.Error("the listing shows the link")
	}
}

func TestReset_WhoMayApproveWhom(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "adm", "admin", "s3cret")
	s.addUserWithPassword(t, "adm2", "admin", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss, adm, ana := s.login(t, "boss", "s3cret"), s.login(t, "adm", "s3cret"), s.login(t, "ana", "s3cret")
	s.askReset("adm2")
	s.askReset("ana")
	adminReq, readerReq := s.resetID(t, "adm2"), s.resetID(t, "ana")

	names := func(token string) string {
		var out struct{ Data []struct{ Username string } }
		json.Unmarshal(s.req("GET", "/password-resets", token, "").Body.Bytes(), &out)
		var list []string
		for _, r := range out.Data {
			list = append(list, r.Username)
		}
		return strings.Join(list, ",")
	}
	if got := names(adm); got != "ana" {
		t.Errorf("an admin sees %q, want only the readers' requests", got)
	}
	if got := names(boss); !strings.Contains(got, "ana") || !strings.Contains(got, "adm2") {
		t.Errorf("the owner sees %q, want everyone's", got)
	}
	if rec := s.req("POST", "/password-resets/"+adminReq+"/approve", adm, ""); rec.Code != http.StatusForbidden {
		t.Errorf("admin approves an admin's request: %d, want 403", rec.Code)
	}
	if rec := s.req("POST", "/password-resets/"+adminReq+"/reject", adm, ""); rec.Code != http.StatusForbidden {
		t.Errorf("admin rejects an admin's request: %d, want 403", rec.Code)
	}
	if rec := s.req("POST", "/password-resets/"+readerReq+"/approve", ana, ""); rec.Code != http.StatusForbidden {
		t.Errorf("a reader approves: %d, want 403", rec.Code)
	}
	if s.scalar(t, `SELECT count(*) FROM password_resets WHERE decision IS NOT NULL`) != "0" {
		t.Fatal("a refused decision changed a request")
	}
	s.approve(t, boss, adminReq, 200) // the owner may
	s.approve(t, adm, readerReq, 200)

	// The role is the database's, not the token's.
	s.askReset("adm2")
	s.db.Exec(`UPDATE users SET role = 'reader' WHERE username = 'adm'`)
	s.db.Exec(`UPDATE users SET role = 'admin' WHERE username = 'ana'`)
	if rec := s.req("POST", "/password-resets/"+s.resetID(t, "adm2")+"/approve", adm, ""); rec.Code != http.StatusForbidden {
		t.Errorf("stale admin token: %d, want 403", rec.Code)
	}
}

func TestReset_RejectedExpiredUsedAndUnknownLinksAllAnswerTheSame(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	for _, n := range []string{"a", "b", "c"} {
		s.addUserWithPassword(t, n, "reader", "s3cret")
	}
	boss := s.login(t, "boss", "s3cret")
	link := func(user string) approved {
		s.askReset(user)
		return s.approve(t, boss, s.resetID(t, user), 200)
	}
	expired, used := link("a"), link("b")
	s.db.Exec(`UPDATE password_resets SET token_expires_at = now() - interval '1 second' WHERE token_hash IS NOT NULL AND id = $1`, expired.ID)
	if code := s.useReset(used.Token, "senha-nova-1"); code != 204 {
		t.Fatalf("use: %d", code)
	}
	s.askReset("c")
	rejectedID := s.resetID(t, "c")
	if rec := s.req("POST", "/password-resets/"+rejectedID+"/reject", boss, ""); rec.Code != 204 {
		t.Fatalf("reject: %d", rec.Code)
	}

	answers := map[string]bool{}
	for name, token := range map[string]string{"expired": expired.Token, "used": used.Token, "unknown": "nao-existe", "empty": ""} {
		check := s.req("GET", "/auth/reset?token="+token, "", "")
		use := s.req("POST", "/auth/reset", "", fmt.Sprintf(`{"token":%q,"password":"outra-senha-9"}`, token))
		if check.Code != 404 || use.Code != 404 {
			t.Errorf("%s: check %d, use %d, want 404 for both", name, check.Code, use.Code)
		}
		answers[check.Body.String()+"|"+use.Body.String()] = true
	}
	if len(answers) != 1 {
		t.Errorf("the answers differ: %v", answers)
	}
	if rec := s.req("POST", "/password-resets/"+rejectedID+"/approve", boss, ""); rec.Code != http.StatusConflict {
		t.Errorf("approving a rejected request: %d, want 409", rec.Code)
	}
	if rec := s.req("POST", "/password-resets/"+rejectedID+"/reject", boss, ""); rec.Code != http.StatusConflict {
		t.Errorf("rejecting twice: %d, want 409", rec.Code)
	}
	// None of the refused uses changed a password.
	for _, n := range []string{"a", "c"} {
		if rec := s.req("POST", "/auth/login", "", fmt.Sprintf(`{"username":%q,"password":"s3cret"}`, n)); rec.Code != 200 {
			t.Errorf("%s lost the password without a valid reset: %d", n, rec.Code)
		}
	}
}

func TestReset_ALinkIsShownOnceAndARequestLapses(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	s.askReset("ana")
	id := s.resetID(t, "ana")
	s.approve(t, boss, id, 200)
	if rec := s.req("POST", "/password-resets/"+id+"/approve", boss, ""); rec.Code != http.StatusConflict {
		t.Errorf("approving again: %d, want 409 (the link is not shown twice)", rec.Code)
	}

	s.askReset("ana") // a new request while the link is out
	s.db.Exec(`UPDATE password_resets SET created_at = now() - interval '8 days' WHERE decision IS NULL`)
	if rec := s.req("POST", "/password-resets/"+s.resetID(t, "ana")+"/approve", boss, ""); rec.Code != http.StatusConflict {
		t.Errorf("approving a request that waited too long: %d, want 409", rec.Code)
	}
}

func TestReset_BlockedAccountsCannotBeApprovedOrUseAnApprovedLink(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	aID := s.addUserWithPassword(t, "a", "reader", "s3cret")
	bID := s.addUserWithPassword(t, "b", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")

	s.askReset("a")
	s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, aID)
	s.approve(t, boss, s.resetID(t, "a"), 409)

	s.askReset("b")
	link := s.approve(t, boss, s.resetID(t, "b"), 200)
	if rec := s.req("POST", "/users/"+bID+"/block", boss, ""); rec.Code != 204 {
		t.Fatalf("block: %d", rec.Code)
	}
	if code := s.useReset(link.Token, "senha-nova-1"); code != http.StatusNotFound {
		t.Errorf("a blocked account used its link: %d, want 404", code)
	}
	if rec := s.req("GET", "/auth/reset?token="+link.Token, "", ""); rec.Code != 404 {
		t.Errorf("check for a blocked account: %d, want 404", rec.Code)
	}
}

func TestReset_ConcurrentUsesChangeThePasswordOnce(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	s.askReset("ana")
	link := s.approve(t, boss, s.resetID(t, "ana"), 200)

	const tries = 16
	codes := make([]int, tries)
	var wg sync.WaitGroup
	for i := 0; i < tries; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = s.useReset(link.Token, fmt.Sprintf("senha-numero-%d", i))
		}(i)
	}
	wg.Wait()
	won := 0
	for _, c := range codes {
		if c == http.StatusNoContent {
			won++
		}
	}
	if won != 1 || s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'password_reset.use'`) != "1" {
		t.Errorf("codes = %v; want exactly one success and one audit entry", codes)
	}
}

func TestReset_UsingOneLinkKillsTheOthersOfTheSameAccount(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	s.askReset("ana")
	first := s.approve(t, boss, s.resetID(t, "ana"), 200)
	s.askReset("ana") // allowed: the first request is decided
	second := s.approve(t, boss, s.resetID(t, "ana"), 200)

	if code := s.useReset(second.Token, "senha-nova-1"); code != 204 {
		t.Fatalf("use: %d", code)
	}
	if code := s.useReset(first.Token, "outra-senha-9"); code != http.StatusNotFound {
		t.Errorf("the earlier link still works: %d, want 404", code)
	}
}
