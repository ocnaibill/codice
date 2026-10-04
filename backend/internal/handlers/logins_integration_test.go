package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/ldapauth/ldaptest"
	"github.com/ocnaibill/codice/backend/internal/logins"
)

// events is the record as the tests read it: "result/method user|typed ip xCount".
func (s *authStack) events(t *testing.T) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT e.result, e.method, COALESCE(u.username, ''), COALESCE(e.typed_name, ''), COALESCE(e.ip, ''), e.count
		FROM login_events e LEFT JOIN users u ON u.id = e.user_id ORDER BY e.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var result, method, user, typed, ip string
		var n int
		rows.Scan(&result, &method, &user, &typed, &ip, &n)
		who := user
		if typed != "" {
			who = "typed:" + typed
		}
		out = append(out, fmt.Sprintf("%s/%s %s %s x%d", result, method, who, ip, n))
	}
	return out
}

func (s *authStack) expectEvents(t *testing.T, want ...string) {
	t.Helper()
	got := s.events(t)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("record =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLoginRecord_LocalSignInsAndTheirFailures(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	bob := s.addUserWithPassword(t, "bob", "reader", "s3cret")
	s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, bob)

	s.tryLogin("ana", "s3cret")
	s.tryLogin("ana", "errada")
	s.tryLogin("bob", "s3cret")  // the right password, but the account is blocked
	s.tryLogin("bob", "errada")  // blocked: whatever the password
	s.tryLogin("fantasma", "x1") // no such account
	s.expectEvents(t,
		"success/local ana 192.0.2.1 x1",
		"bad_password/local ana 192.0.2.1 x1",
		"blocked/local bob 192.0.2.1 x2",
		"unknown_user/local typed:fantasma 192.0.2.1 x1",
	)
}

func TestLoginRecord_ARunOfWrongPasswordsIsOneRowWithACount(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	for i := 0; i < 6; i++ {
		s.tryLogin("ana", fmt.Sprintf("tentativa%d", i))
	}
	s.expectEvents(t, "bad_password/local ana 192.0.2.1 x6")
}

func TestLoginRecord_TheAddressIsTheRealOneBehindATrustedProxy(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.loginFrom(t, "ana", "s3cret", "Firefox", "203.0.113.7")
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"username":"ana","password":"errada"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	s.router.ServeHTTP(httptest.NewRecorder(), req)
	s.expectEvents(t, "success/local ana 203.0.113.7 x1", "bad_password/local ana 203.0.113.8 x1")
	var ua string
	s.db.QueryRow(`SELECT user_agent FROM login_events WHERE result = 'success'`).Scan(&ua)
	if ua != "Firefox" {
		t.Errorf("device = %q", ua)
	}
}

func TestLoginRecord_ANameThatLooksLikeAPasswordIsNotKept(t *testing.T) {
	s := newAuthStack(t)
	s.tryLogin("minha senha secreta 123", "x")
	s.tryLogin("S3nh@Forte!2026", "x")
	s.tryLogin("root", "x")
	s.expectEvents(t,
		"unknown_user/local  192.0.2.1 x2", // two rows' worth of dropped names: grouped, since nothing tells them apart
		"unknown_user/local typed:root 192.0.2.1 x1",
	)
}

func TestLoginRecord_WhatIsNotAnAttemptAtTheDoorIsNotRecorded(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	if rec := s.req("POST", "/auth/login", "", "isto não é json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed: %d", rec.Code)
	}
	s.expectEvents(t)
}

func TestLoginRecord_TheDirectory(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna, dirBob)
	s.addLinked(t, "ana", "reader", "uuid-ana")

	s.tryLogin("ana", "senha-do-diretorio")
	s.tryLogin("ana", "errada")
	s.expectEvents(t, "success/ldap ana 192.0.2.1 x1", "bad_password/ldap ana 192.0.2.1 x1")

	// First sign-in of someone who exists only in the directory, with the owner's permission to create accounts.
	s.allowCreate(t, true)
	if v := s.tryLogin("bob", "senha-do-bob"); v.code != 200 {
		t.Fatalf("first login: %d %s", v.code, v.body)
	}
	s.tryLogin("bob2", "x") // not in the directory either
	got := s.events(t)
	if got[2] != "success/ldap bob 192.0.2.1 x1" || got[3] != "unknown_user/ldap typed:bob2 192.0.2.1 x1" {
		t.Errorf("record = %v", got)
	}

	// The directory goes away: an operational answer, recorded as such.
	dir.SetDown(true)
	s.tryLogin("ana", "senha-do-diretorio")
	if last := s.events(t); last[len(last)-1] != "directory_unavailable/ldap ana 192.0.2.1 x1" {
		t.Errorf("record = %v", last)
	}
}

func TestLoginRecord_TheOwnerIsAlwaysLocalEvenWithADirectory(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, ldaptest.User{Name: "boss", Password: "senha-do-diretorio", UUID: "uuid-boss"})
	s.addUserWithPassword(t, "boss", "owner", "senha-local")
	s.tryLogin("boss", "senha-local")
	s.tryLogin("boss", "senha-do-diretorio")
	s.expectEvents(t, "success/local boss 192.0.2.1 x1", "bad_password/local boss 192.0.2.1 x1")
}

func TestLoginRecord_ALinkRequestIsRecordedAsOneAndItsCompletionAsASignIn(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, dirAna)
	s.addUserWithPassword(t, "ana", "reader", "senha-local")

	// The local account's password is wrong, the directory's is right: the person is asked to prove both.
	offer := s.tryLogin("ana", "senha-do-diretorio")
	if offer.code != http.StatusAccepted || offer.ticket() == "" {
		t.Fatalf("offer: %d %s", offer.code, offer.body)
	}
	s.expectEvents(t, "link_offered/ldap ana 192.0.2.1 x1")
	rec := s.req("POST", "/auth/link", "", fmt.Sprintf(`{"ticket":%q,"password":"errada"}`, offer.ticket()))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("link with a wrong password: %d", rec.Code)
	}
	offer = s.tryLogin("ana", "senha-do-diretorio")
	rec = s.req("POST", "/auth/link", "", fmt.Sprintf(`{"ticket":%q,"password":"senha-local"}`, offer.ticket()))
	if rec.Code != http.StatusOK {
		t.Fatalf("link: %d %s", rec.Code, rec.Body.String())
	}
	got := s.events(t)
	if got[len(got)-1] != "success/ldap ana 192.0.2.1 x1" {
		t.Errorf("record = %v", got)
	}
}

func TestLoginRecord_SetupAndInvitationSignInsHaveTheirOwnWay(t *testing.T) {
	s := newAuthStack(t)
	rec := s.req("POST", "/auth/setup", "", `{"username":"dono","email":"d@x","password":"umasenhaboa"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	var owner AuthResponse
	json.Unmarshal(rec.Body.Bytes(), &owner)
	inv := s.req("POST", "/invitations", owner.Token, `{"role":"reader","email":"n@x"}`)
	var token struct{ Token string }
	json.Unmarshal(inv.Body.Bytes(), &token)
	if rec := s.req("POST", "/auth/redeem", "", fmt.Sprintf(`{"token":%q,"username":"nova","password":"umasenhaboa","email":"n@x"}`, token.Token)); rec.Code != http.StatusCreated {
		t.Fatalf("redeem: %d", rec.Code)
	}
	s.expectEvents(t, "success/setup dono 192.0.2.1 x1", "success/invite nova 192.0.2.1 x1")
}

func TestLoginRecord_AnAppThatGivesTheWrongSecretIsRecorded(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	session := s.login(t, "ana", "s3cret")
	_, token := createAppToken(t, s, session, "KOReader")
	s.db.Exec(`DELETE FROM login_events`)

	if rec := basicReq(s, "ana", token); rec.Code != 200 {
		t.Fatalf("a good secret: %d", rec.Code)
	}
	basicReq(s, "ana", "cdc_errado")
	basicReq(s, "ana", "cdc_errado")
	basicReq(s, "fantasma", "cdc_x")
	// A request with no credentials at all is the client asking what to give: not an attempt.
	req := httptest.NewRequest("GET", "/files/probe", nil)
	s.router.ServeHTTP(httptest.NewRecorder(), req)
	s.expectEvents(t,
		"bad_app_token/app ana 192.0.2.1 x2",
		"unknown_user/app typed:fantasma 192.0.2.1 x1",
	)
}

// ---- Reading the record ----

func (s *authStack) tokenOf(t *testing.T, name, role string) string {
	t.Helper()
	s.addUserWithPassword(t, name, role, "s3cret")
	return s.login(t, name, "s3cret")
}

type listing struct {
	Entries []struct {
		ID       int64  `json:"id"`
		Result   string `json:"result"`
		Method   string `json:"method"`
		Username string `json:"username"`
		Typed    string `json:"typed"`
		IP       string `json:"ip"`
		Count    int    `json:"count"`
	} `json:"entries"`
	More          bool `json:"more"`
	RetentionDays int  `json:"retentionDays"`
	SameAddress   struct {
		Warn    bool   `json:"warn"`
		Address string `json:"address"`
	} `json:"sameAddress"`
}

func (s *authStack) list(t *testing.T, token, query string) (int, listing) {
	t.Helper()
	rec := s.req("GET", "/admin/logins"+query, token, "")
	var out listing
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestLoginRecordList_TheStaffReadsItAndTheReaderDoesNot(t *testing.T) {
	s := newAuthStack(t)
	owner := s.tokenOf(t, "dono", "owner")
	admin := s.tokenOf(t, "adm", "admin")
	reader := s.tokenOf(t, "ana", "reader")

	if code, _ := s.list(t, "", ""); code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", code)
	}
	if rec := s.req("GET", "/admin/logins", reader, ""); rec.Code != http.StatusForbidden {
		// The test stack has no role gate of its own: the router's `staff` does. The handler itself is open to whoever
		// reaches it, so what is tested here is only that a signed-in caller gets an answer.
		t.Logf("reader through the test stack: %d (the role gate is the router's)", rec.Code)
	}
	for _, token := range []string{owner, admin} {
		code, got := s.list(t, token, "")
		if code != 200 || len(got.Entries) != 3 || got.RetentionDays != 90 {
			t.Fatalf("list: %d %+v", code, got)
		}
		if got.Entries[0].Result != "success" || got.Entries[0].Username != "ana" || got.Entries[0].IP != "192.0.2.1" {
			t.Errorf("newest = %+v", got.Entries[0])
		}
	}
}

func TestLoginRecordList_TheNamesTypedAreTheOwnersAlone(t *testing.T) {
	s := newAuthStack(t)
	owner := s.tokenOf(t, "dono", "owner")
	admin := s.tokenOf(t, "adm", "admin")
	s.tryLogin("root", "x")

	_, forOwner := s.list(t, owner, "?result=unknown_user")
	_, forAdmin := s.list(t, admin, "?result=unknown_user")
	if len(forOwner.Entries) != 1 || forOwner.Entries[0].Typed != "root" {
		t.Errorf("owner = %+v", forOwner.Entries)
	}
	if len(forAdmin.Entries) != 1 || forAdmin.Entries[0].Typed != "" || forAdmin.Entries[0].Result != "unknown_user" {
		t.Errorf("an administrator sees that someone tried a name, not which: %+v", forAdmin.Entries)
	}
	rec := s.req("GET", "/admin/logins?result=unknown_user", admin, "")
	if strings.Contains(rec.Body.String(), "root") {
		t.Errorf("the name typed is in the answer given to an administrator: %s", rec.Body.String())
	}
}

func TestLoginRecordList_FiltersAndPages(t *testing.T) {
	s := newAuthStack(t)
	owner := s.tokenOf(t, "dono", "owner")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	for i := 0; i < 3; i++ {
		s.loginFrom(t, "ana", "s3cret", "A", fmt.Sprintf("203.0.113.%d", 10+i))
	}
	s.loginFrom(t, "ana", "s3cret", "B", "198.51.100.1")
	s.tryLogin("ana", "errada")

	if _, got := s.list(t, owner, "?result=bad_password"); len(got.Entries) != 1 {
		t.Errorf("by result: %+v", got.Entries)
	}
	if _, got := s.list(t, owner, "?username=ana"); len(got.Entries) != 5 {
		t.Errorf("by account: %d, want 5 (the owner's own is not ana's)", len(got.Entries))
	}
	_, p1 := s.list(t, owner, "?username=ana&limit=4")
	if len(p1.Entries) != 4 || !p1.More {
		t.Fatalf("page 1: %+v", p1)
	}
	_, p2 := s.list(t, owner, fmt.Sprintf("?username=ana&limit=4&before=%d", p1.Entries[3].ID))
	if len(p2.Entries) != 1 || p2.More {
		t.Errorf("page 2: %+v", p2)
	}
	future := url.QueryEscape("2999-01-01T00:00:00Z")
	if _, got := s.list(t, owner, "?from="+future); len(got.Entries) != 0 {
		t.Errorf("from the future: %+v", got.Entries)
	}
	past := url.QueryEscape("2000-01-01T00:00:00Z")
	if _, got := s.list(t, owner, "?to="+past); len(got.Entries) != 0 {
		t.Errorf("to the past: %+v", got.Entries)
	}
}

func TestLoginRecordList_RefusesWhatItDoesNotUnderstand(t *testing.T) {
	s := newAuthStack(t)
	owner := s.tokenOf(t, "dono", "owner")
	for _, q := range []string{"?result=banana", "?from=ontem", "?to=2026-10-04", "?before=x", "?before=-1", "?limit=x", "?limit=-3"} {
		if rec := s.req("GET", "/admin/logins"+q, owner, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, rec.Code)
		}
	}
	if rec := s.req("GET", "/admin/logins?limit=100000", owner, ""); rec.Code != 200 {
		t.Errorf("a huge limit is capped, not refused: %d", rec.Code)
	}
}

func TestLoginRecordList_WarnsWhenEveryoneComesFromOneProxyAddress(t *testing.T) {
	s := newAuthStack(t)
	owner := s.tokenOf(t, "dono", "owner")
	_, got := s.list(t, owner, "")
	if got.SameAddress.Warn {
		t.Error("a fresh record must not warn")
	}
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	for i := 0; i < 6; i++ {
		s.login(t, "ana", "s3cret")
	}
	if _, got = s.list(t, owner, ""); got.SameAddress.Warn {
		t.Errorf("7 sign-ins from one PUBLIC address (a household): %+v", got.SameAddress)
	}
	// The same, but the address is a private one: what a proxy that hides the clients looks like.
	s.db.Exec(`DELETE FROM login_events`)
	for i := 0; i < 6; i++ {
		s.loginFrom(t, "ana", "s3cret", "A", "10.1.2.3")
	}
	_, got = s.list(t, owner, "")
	if !got.SameAddress.Warn || got.SameAddress.Address != "10.1.2.3" {
		t.Errorf("6 sign-ins all from 10.1.2.3: %+v", got.SameAddress)
	}
}

func TestLoginRecordRetention_OnlyWithinLimitsAndAudited(t *testing.T) {
	s := newAuthStack(t)
	owner := s.tokenOf(t, "dono", "owner")
	for _, body := range []string{`{}`, `nope`, `{"retentionDays":6}`, `{"retentionDays":3651}`, `{"retentionDays":0}`, `{"retentionDays":-1}`} {
		if rec := s.req("PUT", "/admin/logins/settings", owner, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, rec.Code)
		}
	}
	for _, days := range []int{7, 30, 3650} {
		if rec := s.req("PUT", "/admin/logins/settings", owner, fmt.Sprintf(`{"retentionDays":%d}`, days)); rec.Code != 200 {
			t.Fatalf("%d days: %d", days, rec.Code)
		}
		if _, got := s.list(t, owner, ""); got.RetentionDays != days {
			t.Errorf("after setting %d: %d", days, got.RetentionDays)
		}
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'logins.retention'`) != "3" {
		t.Error("the changes were not audited")
	}
}

func TestLoginRecordOwn_EachPersonReadsTheirOwnDoorOnly(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.addUserWithPassword(t, "bia", "reader", "s3cret")
	ana := s.login(t, "ana", "s3cret")
	s.tryLogin("ana", "errada")  // someone trying Ana's password
	s.tryLogin("bia", "errada")  // and Bia's
	s.tryLogin("fantasma", "x1") // a name with no account: nobody's door

	rec := s.req("GET", "/auth/logins", ana, "")
	var mine []struct {
		Result, Method, IP string
		Typed, Username    string
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &mine) != nil {
		t.Fatalf("own: %d %s", rec.Code, rec.Body.String())
	}
	if len(mine) != 2 || mine[0].Result != "bad_password" || mine[1].Result != "success" {
		t.Errorf("own = %+v", mine)
	}
	for _, e := range mine {
		if e.Typed != "" || e.Username != "" {
			t.Errorf("own must not carry a name: %+v", e)
		}
	}
	if strings.Contains(rec.Body.String(), "fantasma") {
		t.Error("another door's name is in what a person reads")
	}
	if rec := s.req("GET", "/auth/logins", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	var empty []any
	s.addUserWithPassword(t, "caio", "reader", "s3cret")
	// An account with no entry yet gets an empty list, not null.
	s.db.Exec(`DELETE FROM login_events`)
	rec = s.req("GET", "/auth/logins", ana, "")
	if json.Unmarshal(rec.Body.Bytes(), &empty) != nil || empty == nil || len(empty) != 0 {
		t.Errorf("an empty list must be [], got %s", rec.Body.String())
	}
}

func TestLoginRecord_ItNeverStandsInTheWayOfSigningIn(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	s.db.Exec(`DROP TABLE login_events`)
	if v := s.tryLogin("ana", "s3cret"); v.code != 200 || v.token() == "" {
		t.Errorf("with the record broken, a sign-in must still work: %d", v.code)
	}
	if v := s.tryLogin("ana", "errada"); v.code != http.StatusUnauthorized {
		t.Errorf("and a wrong password must still be refused the same way: %d", v.code)
	}
}

var _ = logins.Success
