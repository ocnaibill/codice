package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/testdb"
)

func (s *authStack) del(bearer, id, confirm string) int {
	return s.req("DELETE", "/users/"+id, bearer, fmt.Sprintf(`{"confirmUsername":%q}`, confirm)).Code
}

func TestDeleteAccount_FollowsThePolicyAndNeedsTheNameTyped(t *testing.T) {
	s := newAuthStack(t)
	ownerID := s.addUserWithPassword(t, "boss", "owner", "s3cret")
	adminID := s.addUserWithPassword(t, "adm", "admin", "s3cret")
	admin2ID := s.addUserWithPassword(t, "adm2", "admin", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	reader2ID := s.addUserWithPassword(t, "bob", "reader", "s3cret")
	boss, adm, ana := s.login(t, "boss", "s3cret"), s.login(t, "adm", "s3cret"), s.login(t, "ana", "s3cret")

	for name, tc := range map[string]struct{ token, id, name string }{
		"admin deletes the owner": {adm, ownerID, "boss"},
		"admin deletes an admin":  {adm, admin2ID, "adm2"},
		"admin deletes themself":  {adm, adminID, "adm"},
		"owner deletes themself":  {boss, ownerID, "boss"},
		"reader deletes a reader": {ana, reader2ID, "bob"},
		"reader deletes themself": {ana, readerID, "ana"},
	} {
		if code := s.del(tc.token, tc.id, tc.name); code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", name, code)
		}
	}
	if n := s.scalar(t, `SELECT count(*) FROM users`); n != "5" {
		t.Fatalf("a forbidden deletion removed accounts: %s left", n)
	}

	// Permitted, but only with the username repeated exactly.
	for _, wrong := range []string{"", "bob ", "BOB", "ana"} {
		if code := s.del(adm, reader2ID, wrong); code != http.StatusBadRequest {
			t.Errorf("confirm %q: %d, want 400", wrong, code)
		}
	}
	if s.scalar(t, `SELECT count(*) FROM users WHERE username = 'bob'`) != "1" {
		t.Fatal("a wrong confirmation deleted the account")
	}
	if code := s.del(adm, reader2ID, "bob"); code != http.StatusNoContent {
		t.Errorf("admin deletes a reader: %d, want 204", code)
	}
	if code := s.del(boss, admin2ID, "adm2"); code != http.StatusNoContent {
		t.Errorf("owner deletes an admin: %d, want 204", code)
	}
	if code := s.del(boss, reader2ID, "bob"); code != http.StatusNotFound {
		t.Errorf("already gone: %d, want 404", code)
	}
	if code := s.del(boss, "not-a-uuid", "x"); code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", code)
	}
	// The role is the database's, not the token's.
	s.db.Exec(`UPDATE users SET role = 'reader' WHERE id = $1`, adminID)
	if code := s.del(adm, readerID, "ana"); code != http.StatusForbidden {
		t.Errorf("stale admin token: %d, want 403", code)
	}
}

func TestDeleteAccount_RemovesPersonalDataAndAccessButNotTheLibrary(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	readerID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	otherID := s.addUserWithPassword(t, "bob", "reader", "s3cret")
	boss, ana := s.login(t, "boss", "s3cret"), s.login(t, "ana", "s3cret")
	_, appToken := createAppToken(t, s, ana, "kobo")

	work, _, file := testdb.AddWork(t, s.db, testdb.Work{Title: "Duna", Path: "duna.epub", Format: "epub", Author: "Frank Herbert"})
	for _, u := range []string{readerID, otherID} {
		for _, q := range []string{
			`INSERT INTO notes (user_id, work_id, quote) VALUES ($1, $2, 'segredo pessoal')`,
			`INSERT INTO favorites (user_id, work_id) VALUES ($1, $2)`,
		} {
			if _, err := s.db.Exec(q, u, work); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.db.Exec(`INSERT INTO reading_progress (user_id, file_id, position) VALUES ($1, $2, 'p1')`, u, file); err != nil {
			t.Fatal(err)
		}
	}

	if code := s.del(boss, readerID, "ana"); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}

	for _, q := range []string{
		`SELECT count(*) FROM users WHERE id = '%s'`,
		`SELECT count(*) FROM notes WHERE user_id = '%s'`,
		`SELECT count(*) FROM favorites WHERE user_id = '%s'`,
		`SELECT count(*) FROM reading_progress WHERE user_id = '%s'`,
		`SELECT count(*) FROM sessions WHERE user_id = '%s'`,
		`SELECT count(*) FROM app_tokens WHERE user_id = '%s'`,
	} {
		if n := s.scalar(t, fmt.Sprintf(q, readerID)); n != "0" {
			t.Errorf("%s -> %s left behind", q, n)
		}
	}
	if rec := s.req("GET", "/probe", ana, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("session of a deleted account: %d, want 401", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("app token of a deleted account: %d, want 401", rec.Code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"s3cret"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("login of a deleted account: %d, want 401", rec.Code)
	}

	// Someone else's data and the library itself are untouched.
	if s.scalar(t, `SELECT count(*) FROM notes WHERE user_id = '`+otherID+`'`) != "1" ||
		s.scalar(t, `SELECT count(*) FROM favorites WHERE user_id = '`+otherID+`'`) != "1" ||
		s.scalar(t, `SELECT count(*) FROM works`) != "1" || s.scalar(t, `SELECT count(*) FROM files`) != "1" {
		t.Error("deleting one account touched other data")
	}

	// The audit entry has counts and names, never the content of a note.
	entry := s.scalar(t, `SELECT details::text FROM audit_log WHERE action = 'user.delete'`)
	if !strings.Contains(entry, `"username": "ana"`) || !strings.Contains(entry, `"notes": 1`) {
		t.Errorf("audit = %s", entry)
	}
	if strings.Contains(entry, "segredo") {
		t.Error("the audit log holds the content of a note")
	}
}

func TestDeleteAccount_TheInvitationsItIssuedDieWithIt(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	admID := s.addUserWithPassword(t, "adm", "admin", "s3cret")
	boss, adm := s.login(t, "boss", "s3cret"), s.login(t, "adm", "s3cret")
	pending := s.invite(t, adm, `{}`, 201)
	usedInv := s.invite(t, adm, `{}`, 201)
	if code, _ := s.redeem(usedInv.Token, "convidada", "senha-longa", ""); code != 201 {
		t.Fatalf("redeem: %d", code)
	}

	if code := s.del(boss, admID, "adm"); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := s.redeem(pending.Token, "tarde", "senha-longa", ""); code != http.StatusNotFound {
		t.Errorf("a deleted account's invitation still works: %d", code)
	}
	// The account created through an invitation is not the issuer's data.
	if s.scalar(t, `SELECT count(*) FROM users WHERE username = 'convidada'`) != "1" {
		t.Error("deleting the issuer removed the account it invited")
	}
}

func TestChangePassword_NeedsTheCurrentOneAndEndsTheOtherSessions(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "ana", "reader", "senha-antiga")
	here := s.login(t, "ana", "senha-antiga")
	elsewhere := s.login(t, "ana", "senha-antiga")
	_, appToken := createAppToken(t, s, here, "kobo")
	change := func(token, current, next string) int {
		return s.req("POST", "/auth/password", token, fmt.Sprintf(`{"current":%q,"new":%q}`, current, next)).Code
	}

	if code := change("", "senha-antiga", "senha-nova-1"); code != http.StatusUnauthorized {
		t.Errorf("without a session: %d, want 401", code)
	}
	if code := change(here, "errada", "senha-nova-1"); code != http.StatusForbidden {
		t.Errorf("wrong current password: %d, want 403", code)
	}
	if code := change(here, "senha-antiga", "curta"); code != http.StatusBadRequest {
		t.Errorf("short new password: %d, want 400", code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"senha-antiga"}`); rec.Code != 200 {
		t.Fatalf("a refused change altered the password: %d", rec.Code)
	}

	if code := change(here, "senha-antiga", "senha-nova-1"); code != http.StatusNoContent {
		t.Fatalf("change: %d", code)
	}
	if rec := s.req("GET", "/probe", here, ""); rec.Code != 200 {
		t.Errorf("the session used to change it was ended: %d", rec.Code)
	}
	if rec := s.req("GET", "/probe", elsewhere, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("another session survived the change: %d, want 401", rec.Code)
	}
	if rec := basicReq(s, "ana", appToken); rec.Code != 200 {
		t.Errorf("an app token should keep working: %d", rec.Code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"senha-antiga"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("the old password still works: %d", rec.Code)
	}
	if rec := s.req("POST", "/auth/login", "", `{"username":"ana","password":"senha-nova-1"}`); rec.Code != 200 {
		t.Errorf("the new password does not work: %d", rec.Code)
	}
	if got := s.scalar(t, `SELECT details->>'sessionsEnded' FROM audit_log WHERE action = 'user.password_change'`); got != "2" {
		// the extra login above created none before the change: elsewhere + the login used to prove the refusal
		t.Errorf("audit sessionsEnded = %s", got)
	}
	if strings.Contains(s.scalar(t, `SELECT coalesce(string_agg(details::text, ' '), '') FROM audit_log`), "senha-") {
		t.Error("a password reached the audit log")
	}
}

func TestChangePassword_RefusesAnAccountWithoutALocalPassword(t *testing.T) {
	s := newAuthStack(t)
	id := s.addUserWithPassword(t, "ana", "reader", "senha-antiga")
	tok := s.login(t, "ana", "senha-antiga")
	s.db.Exec(`UPDATE users SET password_hash = NULL WHERE id = $1`, id)
	if code := s.req("POST", "/auth/password", tok, `{"current":"x","new":"senha-nova-1"}`).Code; code != http.StatusBadRequest {
		t.Errorf("no local password: %d, want 400", code)
	}
}
