package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ocnaibill/codice/backend/internal/middleware"
)

func TestBlock_EndsEveryFormOfAccessAtOnceAndUnblockingDoesNotRevive(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	ana := s.login(t, "ana", "s3cret")
	resource := mintToken(t, s, ana, "assets")
	_, appToken := createAppToken(t, s, ana, "kobo")

	if rec := s.req("POST", "/users/"+readerID+"/block", boss, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.req("GET", "/probe", ana, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("session after block: %d, want 401", rec.Code)
	}
	if rec := s.req("GET", "/files/probe?rt="+resource, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("resource token after block: %d, want 401", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("app token after block: %d, want 401", rec.Code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"s3cret"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("login after block: %d, want 401", rec.Code)
	}
	// Sessions and tokens are really revoked, not just hidden by the flag.
	if n := s.scalar(t, `SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, readerID); n != "0" {
		t.Errorf("live sessions left: %s", n)
	}
	if n := s.scalar(t, `SELECT count(*) FROM app_tokens WHERE user_id = $1 AND revoked_at IS NULL`, readerID); n != "0" {
		t.Errorf("live app tokens left: %s", n)
	}

	// Unblocking lets the person sign in again but does not bring back what ended.
	if rec := s.req("POST", "/users/"+readerID+"/unblock", boss, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("unblock: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", ana, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("old session after unblock: %d, want 401", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("old app token after unblock: %d, want 401", rec.Code)
	}
	fresh := s.login(t, "ana", "s3cret")
	if rec := s.req("GET", "/probe", fresh, ""); rec.Code != 200 {
		t.Errorf("new session after unblock: %d", rec.Code)
	}

	if got := s.scalar(t, `SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'user.%block'`); got != "user.block,user.unblock" {
		t.Errorf("audit = %q", got)
	}
}

func TestBlock_FollowsThePolicyAndTheDatabaseRole(t *testing.T) {
	s := newAuthStack(t)
	ownerID := s.addUserWithPassword(t, "boss", "owner", "s3cret")
	adminID := s.addUserWithPassword(t, "adm", "admin", "s3cret")
	admin2ID := s.addUserWithPassword(t, "adm2", "admin", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	adm := s.login(t, "adm", "s3cret")
	ana := s.login(t, "ana", "s3cret")

	block := func(token, id string) int { return s.req("POST", "/users/"+id+"/block", token, "").Code }

	for name, tc := range map[string]struct{ token, target string }{
		"admin blocks the owner":  {adm, ownerID},
		"admin blocks an admin":   {adm, admin2ID},
		"admin blocks themself":   {adm, adminID},
		"owner blocks themself":   {boss, ownerID},
		"reader blocks a reader":  {ana, readerID},
		"reader blocks the admin": {ana, adminID},
	} {
		if code := block(tc.token, tc.target); code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", name, code)
		}
	}
	if n := s.scalar(t, `SELECT count(*) FROM users WHERE blocked_at IS NOT NULL`); n != "0" {
		t.Fatalf("a forbidden block changed %s account(s)", n)
	}
	if n := s.scalar(t, `SELECT count(*) FROM sessions WHERE revoked_at IS NOT NULL`); n != "0" {
		t.Errorf("a forbidden block revoked %s session(s)", n)
	}

	if code := block(adm, readerID); code != http.StatusNoContent {
		t.Errorf("admin blocks a reader: %d, want 204", code)
	}
	if code := block(boss, admin2ID); code != http.StatusNoContent {
		t.Errorf("owner blocks an admin: %d, want 204", code)
	}

	// The role is the database's, not the token's: a demoted admin cannot block.
	s.db.Exec(`UPDATE users SET role = 'reader' WHERE id = $1`, adminID)
	if code := block(adm, ownerID); code != http.StatusForbidden {
		t.Errorf("stale admin token: %d, want 403", code)
	}

	if code := block(boss, "not-a-uuid"); code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", code)
	}
	if code := block(boss, "b190281f-fe3e-4308-ad71-000000000000"); code != http.StatusNotFound {
		t.Errorf("unknown id: %d, want 404", code)
	}
}

func TestBlock_IsIdempotentAndAuditedOnce(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss := s.login(t, "boss", "s3cret")

	for i := 0; i < 2; i++ {
		if rec := s.req("POST", "/users/"+readerID+"/block", boss, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("block #%d: %d", i, rec.Code)
		}
	}
	first := s.scalar(t, `SELECT blocked_at::text FROM users WHERE id = $1`, readerID)
	s.req("POST", "/users/"+readerID+"/block", boss, "")
	if again := s.scalar(t, `SELECT blocked_at::text FROM users WHERE id = $1`, readerID); again != first {
		t.Errorf("blocking again moved the timestamp: %s -> %s", first, again)
	}
	if n := s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'user.block'`); n != "1" {
		t.Errorf("block audited %s times, want 1", n)
	}
	// Unblocking someone who is not blocked is also a quiet no-op.
	s.req("POST", "/users/"+readerID+"/unblock", boss, "")
	s.req("POST", "/users/"+readerID+"/unblock", boss, "")
	if n := s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'user.unblock'`); n != "1" {
		t.Errorf("unblock audited %s times, want 1", n)
	}
}

func TestListAccounts_ShowsWhoTheCallerMayBlockAndNothingSecret(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "adm", "admin", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, readerID)
	adm := s.login(t, "adm", "s3cret")

	rec := s.req("GET", "/users", adm, "")
	if rec.Code != 200 {
		t.Fatalf("list: %d", rec.Code)
	}
	for _, secret := range []string{"password", "hash", "sso", "$2a$"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), secret) {
			t.Errorf("the listing leaks %q", secret)
		}
	}
	var body struct {
		Data []struct {
			Username  string
			Role      string
			BlockedAt *string
			CanBlock  bool
			IsSelf    bool
		}
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	got := map[string]struct {
		role               string
		blocked, can, self bool
	}{}
	for _, a := range body.Data {
		got[a.Username] = struct {
			role               string
			blocked, can, self bool
		}{a.Role, a.BlockedAt != nil, a.CanBlock, a.IsSelf}
	}
	if len(body.Data) != 3 || body.Data[0].Username != "boss" {
		t.Fatalf("listing = %+v (owner first, then admins)", body.Data)
	}
	// An admin may block readers only: not the owner, another admin, or themself.
	if got["ana"].can != true || !got["ana"].blocked {
		t.Errorf("reader = %+v", got["ana"])
	}
	if got["boss"].can || got["adm"].can || !got["adm"].self {
		t.Errorf("owner/admin rows = %+v / %+v", got["boss"], got["adm"])
	}
}

func TestBlock_ClosesTheAccountsOpenSockets(t *testing.T) {
	t.Setenv("JWT_SECRET", "test_secret_key_for_testing_12345678")
	ws := &WsHandler{Auth: middleware.Authenticator{
		Sessions: func(_ context.Context, sid string) (string, string, error) { return sid, "reader", nil },
	}}
	srv := httptest.NewServer(http.HandlerFunc(ws.HandleWS))
	defer srv.Close()

	dial := func(user string) *websocket.Conn {
		ticket, _, _ := middleware.IssueResourceToken(user, user, middleware.ScopeWS, time.Minute)
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"?ticket="+ticket, nil)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	ana, bob := dial("ana"), dial("bob")
	defer bob.Close()
	time.Sleep(100 * time.Millisecond) // let the server register both

	ws.DisconnectUser("ana")
	ana.SetReadDeadline(time.Now().Add(2 * time.Second))
	// Closed means the read fails for that reason, not because we ran out of time.
	if _, _, err := ana.ReadMessage(); err == nil || isTimeout(err) {
		t.Errorf("the blocked account's socket is still open (read: %v)", err)
	}
	bob.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := bob.ReadMessage(); err == nil || !isTimeout(err) {
		t.Errorf("another account's socket was closed: %v", err)
	}
}

func (s *authStack) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := s.db.QueryRow(query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
