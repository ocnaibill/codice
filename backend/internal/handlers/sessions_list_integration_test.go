package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sessionRow struct {
	ID         string  `json:"id"`
	UserAgent  string  `json:"userAgent"`
	IP         *string `json:"ip"`
	LastSeenAt *string `json:"lastSeenAt"`
	Current    bool    `json:"current"`
}

// loginAs signs in from a device at an address and returns the token and the session id.
func (s *authStack) loginFrom(t *testing.T, user, pass, agent, forwarded string) (token, sid string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"username":"`+user+`","password":"`+pass+`"}`))
	req.Header.Set("User-Agent", agent)
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body.String())
	}
	var body AuthResponse
	json.Unmarshal(rec.Body.Bytes(), &body)
	s.db.QueryRow(`SELECT id FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = $1) ORDER BY created_at DESC LIMIT 1`, user).Scan(&sid)
	return body.Token, sid
}

func (s *authStack) sessionList(t *testing.T, token, path string) []sessionRow {
	t.Helper()
	rec := s.req("GET", path, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	var out []sessionRow
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSessionsList_LoginRecordsTheDeviceAndTheRealAddressBehindAProxy(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")

	phone, phoneSID := s.loginFrom(t, "ana", "s3cret", "Mozilla/5.0 (iPhone) Safari", "203.0.113.7")
	_, _ = s.loginFrom(t, "ana", "s3cret", "Mozilla/5.0 (X11; Linux) Firefox/130", "")

	list := s.sessionList(t, phone, "/auth/sessions")
	if len(list) != 2 {
		t.Fatalf("sessions = %+v", list)
	}
	byID := map[string]sessionRow{}
	for _, x := range list {
		byID[x.ID] = x
	}
	p := byID[phoneSID]
	if p.IP == nil || *p.IP != "203.0.113.7" || !strings.Contains(p.UserAgent, "iPhone") || !p.Current {
		t.Errorf("the phone = %+v, want the client's address from the proxy header and marked as the one in use", p)
	}
	for _, x := range list {
		if x.ID != phoneSID && (x.Current || x.IP == nil || *x.IP != "192.0.2.1") {
			t.Errorf("a login with no proxy header = %+v, want the address of the connection and not current", x)
		}
	}
}

func TestSessionsList_AStrangersHeaderIsNotBelieved(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"username":"ana","password":"s3cret"}`))
	req.RemoteAddr = "198.51.100.9:5555" // not a proxy of the stack
	req.Header.Set("X-Forwarded-For", "10.9.9.9")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	var ip string
	s.db.QueryRow(`SELECT ip FROM sessions`).Scan(&ip)
	if ip != "198.51.100.9" {
		t.Errorf("ip = %q: a header written by whoever connects must not become the address on record", ip)
	}
}

func TestSessionsList_AccountsAreSeparateAndAnEndedSessionIsGone(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.addUserWithPassword(t, "bia", "reader", "s3cret")
	ana, _ := s.loginFrom(t, "ana", "s3cret", "A", "")
	bia, biaSID := s.loginFrom(t, "bia", "s3cret", "B", "")

	if list := s.sessionList(t, ana, "/auth/sessions"); len(list) != 1 || list[0].UserAgent != "A" {
		t.Errorf("ana sees %+v", list)
	}
	if rec := s.req("GET", "/auth/sessions", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", rec.Code)
	}

	// Ana cannot end Bia's session, and cannot tell it exists.
	if rec := s.req("DELETE", "/auth/sessions/"+biaSID, ana, ""); rec.Code != http.StatusNotFound {
		t.Errorf("another account's session: %d, want 404", rec.Code)
	}
	if rec := s.req("GET", "/probe", bia, ""); rec.Code != 200 {
		t.Error("Bia's session was ended by Ana")
	}
}

func TestSessionsRevoke_EndsASessionAtOnceAndSignsOutIfItIsTheOneInUse(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	laptop, laptopSID := s.loginFrom(t, "ana", "s3cret", "Laptop", "")
	phone, phoneSID := s.loginFrom(t, "ana", "s3cret", "Phone", "")

	if rec := s.req("DELETE", "/auth/sessions/"+laptopSID, phone, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("end the laptop: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", laptop, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("the laptop's token after it was ended: %d, want 401", rec.Code)
	}
	if rec := s.req("DELETE", "/auth/sessions/"+laptopSID, phone, ""); rec.Code != http.StatusNotFound {
		t.Errorf("ending it again: %d, want 404", rec.Code)
	}
	if rec := s.req("DELETE", "/auth/sessions/not-an-id", phone, ""); rec.Code != http.StatusNotFound {
		t.Errorf("a malformed id: %d, want 404", rec.Code)
	}
	var audited int
	s.db.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'session.revoke'`).Scan(&audited)
	if audited != 1 {
		t.Errorf("audit rows = %d, want 1 (the failed attempts are not events)", audited)
	}
	// Ending the one in use is signing out.
	if rec := s.req("DELETE", "/auth/sessions/"+phoneSID, phone, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("end own current: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", phone, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("after ending the current one: %d, want 401", rec.Code)
	}
}

func TestSessionsRevokeOthers_KeepsTheOneInUse(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.addUserWithPassword(t, "bia", "reader", "s3cret")
	a1, _ := s.loginFrom(t, "ana", "s3cret", "1", "")
	a2, _ := s.loginFrom(t, "ana", "s3cret", "2", "")
	a3, _ := s.loginFrom(t, "ana", "s3cret", "3", "")
	bia, _ := s.loginFrom(t, "bia", "s3cret", "B", "")

	rec := s.req("POST", "/auth/sessions/revoke-others", a2, "")
	var out map[string]int
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["revoked"] != 2 {
		t.Fatalf("revoke others: %d %s", rec.Code, rec.Body.String())
	}
	for name, tok := range map[string]string{"a1": a1, "a3": a3} {
		if rec := s.req("GET", "/probe", tok, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, rec.Code)
		}
	}
	for name, tok := range map[string]string{"the one in use": a2, "another account": bia} {
		if rec := s.req("GET", "/probe", tok, ""); rec.Code != 200 {
			t.Errorf("%s was ended", name)
		}
	}
	var audited int
	s.db.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'session.revoke_others'`).Scan(&audited)
	if audited != 1 {
		t.Errorf("audit rows = %d", audited)
	}
	// With nothing else to end, nothing is recorded.
	s.req("POST", "/auth/sessions/revoke-others", a2, "")
	s.db.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'session.revoke_others'`).Scan(&audited)
	if audited != 1 {
		t.Errorf("an empty revoke was audited: %d", audited)
	}
}

func TestSessionsStaff_MayEndTheSessionsOfWhomTheyMayBlockAndSeeNoAddress(t *testing.T) {
	s := newAuthStack(t)
	ownerID := s.addUserWithPassword(t, "dono", "owner", "s3cret")
	adminID := s.addUserWithPassword(t, "adm", "admin", "s3cret")
	adm2ID := s.addUserWithPassword(t, "adm2", "admin", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	owner, _ := s.loginFrom(t, "dono", "s3cret", "O", "")
	admin, _ := s.loginFrom(t, "adm", "s3cret", "A", "")
	_, adm2SID := s.loginFrom(t, "adm2", "s3cret", "A2", "")
	reader, readerSID := s.loginFrom(t, "ana", "s3cret", "R", "203.0.113.7")

	// The list of another account has no address in it, not even an empty one.
	rec := s.req("GET", "/users/"+readerID+"/sessions", admin, "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "203.0.113.7") || strings.Contains(rec.Body.String(), `"ip"`) {
		t.Fatalf("an admin reads a reader's sessions: %d %s", rec.Code, rec.Body.String())
	}
	var list []sessionRow
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].ID != readerSID {
		t.Errorf("list = %+v", list)
	}

	for _, c := range []struct {
		name, token, target string
		want                int
	}{
		{"an admin on an admin", admin, adm2ID, 403},
		{"an admin on the owner", admin, ownerID, 403},
		{"the owner on the owner (their own are at /auth/sessions)", owner, ownerID, 403},
		{"an admin on themselves", admin, adminID, 403},
		{"the owner on an admin", owner, adminID, 200},
		{"the owner on a reader", owner, readerID, 200},
		{"a reader on anyone", reader, readerID, 403},
		{"nobody", "", readerID, 401},
		{"an unknown account", owner, "00000000-0000-0000-0000-000000000000", 404},
		{"a malformed id", owner, "x", 404},
	} {
		if rec := s.req("GET", "/users/"+c.target+"/sessions", c.token, ""); rec.Code != c.want {
			t.Errorf("%s: %d, want %d", c.name, rec.Code, c.want)
		}
	}

	// Ending one session of a reader.
	if rec := s.req("DELETE", "/users/"+readerID+"/sessions/"+adm2SID, admin, ""); rec.Code != http.StatusNotFound {
		t.Errorf("a session of ANOTHER account under this account's url: %d, want 404", rec.Code)
	}
	if rec := s.req("DELETE", "/users/"+readerID+"/sessions/"+readerSID, admin, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("end the reader's session: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", reader, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("the reader's token after the admin ended it: %d, want 401", rec.Code)
	}
	var by string
	s.db.QueryRow(`SELECT details->>'by' FROM audit_log WHERE action = 'session.revoke' AND target_id = $1`, readerID).Scan(&by)
	if by != "staff" {
		t.Errorf("the audit does not say it was the staff: %q", by)
	}
	if rec := s.req("DELETE", "/users/"+adm2ID+"/sessions/"+adm2SID, admin, ""); rec.Code != http.StatusForbidden {
		t.Errorf("an admin ending an admin's session: %d, want 403", rec.Code)
	}
}

func TestSessionsStaff_EndAllOfAnAccountWithoutBlockingIt(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "dono", "owner", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	owner, _ := s.loginFrom(t, "dono", "s3cret", "O", "")
	r1, _ := s.loginFrom(t, "ana", "s3cret", "1", "")
	s.loginFrom(t, "ana", "s3cret", "2", "")
	_, appToken := createAppToken(t, s, r1, "KOReader")

	rec := s.req("DELETE", "/users/"+readerID+"/sessions", owner, "")
	var out map[string]int
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["revoked"] != 2 {
		t.Fatalf("end all: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.req("GET", "/probe", r1, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("a session after it: %d", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != 200 {
		t.Errorf("the app token must stay: %d", rec.Code)
	}
	if got := s.sessionList(t, owner, "/users/"+readerID+"/sessions"); len(got) != 0 {
		t.Errorf("sessions left: %+v", got)
	}
	var audited int
	s.db.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'session.revoke_all' AND details->>'by' = 'staff'`).Scan(&audited)
	if audited != 1 {
		t.Errorf("audit rows = %d", audited)
	}
	if rec := s.req("DELETE", "/users/"+readerID+"/sessions", owner, ""); rec.Code != 200 {
		t.Errorf("ending none: %d", rec.Code)
	}
	s.db.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'session.revoke_all'`).Scan(&audited)
	if audited != 1 {
		t.Errorf("an empty end-all was audited: %d", audited)
	}
	// And she can sign in again: she was not blocked.
	s.login(t, "ana", "s3cret")
}

func TestSessionsList_TheInvitationSignUpAlsoRecordsTheAddress(t *testing.T) {
	s := newAuthStack(t)
	ownerID := s.addUserWithPassword(t, "dono", "owner", "s3cret")
	owner, _ := s.loginFrom(t, "dono", "s3cret", "O", "")
	rec := s.req("POST", "/invitations", owner, `{"role":"reader","email":"n@x"}`)
	var inv struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &inv)
	req := httptest.NewRequest("POST", "/auth/redeem", strings.NewReader(`{"token":"`+inv.Token+`","username":"nova","password":"umasenhaboa","email":"n@x"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome")
	out := httptest.NewRecorder()
	s.router.ServeHTTP(out, req)
	if out.Code != http.StatusCreated {
		t.Fatalf("redeem: %d %s (owner %s)", out.Code, out.Body.String(), ownerID)
	}
	var ip, ua string
	s.db.QueryRow(`SELECT ip, user_agent FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = 'nova')`).Scan(&ip, &ua)
	if ip != "203.0.113.50" || !strings.Contains(ua, "Chrome") {
		t.Errorf("session of the new account: ip %q ua %q", ip, ua)
	}
}
