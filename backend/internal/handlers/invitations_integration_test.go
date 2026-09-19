package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type invite struct {
	ID, Token, Role string
}

func (s *authStack) invite(t *testing.T, bearer, body string, want int) invite {
	t.Helper()
	rec := s.req("POST", "/invitations", bearer, body)
	if rec.Code != want {
		t.Fatalf("invite %s: %d %s, want %d", body, rec.Code, rec.Body.String(), want)
	}
	var out invite
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (s *authStack) redeem(token, username, password, email string) (int, string) {
	body := fmt.Sprintf(`{"token":%q,"username":%q,"password":%q,"email":%q}`, token, username, password, email)
	rec := s.req("POST", "/auth/redeem", "", body)
	return rec.Code, rec.Body.String()
}

func TestInvitation_OnlyWhoMayInviteWhomAndNeverAnOwner(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "adm", "admin", "s3cret")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss, adm, ana := s.login(t, "boss", "s3cret"), s.login(t, "adm", "s3cret"), s.login(t, "ana", "s3cret")

	s.invite(t, adm, `{}`, 201) // the default is a reader
	s.invite(t, adm, `{"role":"reader"}`, 201)
	s.invite(t, adm, `{"role":"admin"}`, 403) // only the owner makes admins
	s.invite(t, boss, `{"role":"admin"}`, 201)
	s.invite(t, boss, `{"role":"owner"}`, 400) // no invitation grants owner
	s.invite(t, ana, `{}`, 403)                // a reader invites nobody
	s.invite(t, boss, `{"email":"sem-arroba"}`, 400)
	s.invite(t, boss, `not json`, 400)

	if n := s.scalar(t, `SELECT count(*) FROM invitations`); n != "3" {
		t.Errorf("stored %s invitations, want 3 (the refused ones must leave nothing)", n)
	}
	// Even a hand-made row cannot grant owner.
	if _, err := s.db.Exec(`INSERT INTO invitations (token_hash, role, expires_at) VALUES ('x', 'owner', now())`); err == nil {
		t.Error("the database accepted an owner invitation")
	}
}

func TestInvitation_SecretIsShownOnceAndNeverStoredOrAudited(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	inv := s.invite(t, boss, `{"email":"Ana@Example.com"}`, 201)
	if len(inv.Token) < 40 {
		t.Fatalf("token too short to be a secret: %q", inv.Token)
	}

	dump := s.scalar(t, `SELECT (SELECT coalesce(string_agg(i::text, ' '), '') FROM invitations i) || ' ' ||
	                            (SELECT coalesce(string_agg(details::text, ' '), '') FROM audit_log)`)
	if strings.Contains(dump, inv.Token) {
		t.Error("the secret appears in the database or the audit log")
	}
	if got := s.scalar(t, `SELECT details->>'emailRestricted' FROM audit_log WHERE action = 'invitation.create'`); got != "true" {
		t.Errorf("audit did not note the email restriction: %q", got)
	}
	listing := s.req("GET", "/invitations", boss, "").Body.String()
	if strings.Contains(listing, inv.Token) {
		t.Error("the listing exposes the secret")
	}
	if s.scalar(t, `SELECT email FROM invitations`) != "ana@example.com" {
		t.Error("the address should be stored normalised")
	}
}

func TestInvitation_RedeemCreatesTheAccountOnceAndLogsIn(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	boss := s.login(t, "boss", "s3cret")
	inv := s.invite(t, boss, `{"role":"admin"}`, 201)

	if rec := s.req("GET", "/auth/invitation?token="+inv.Token, "", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("check: %d %s", rec.Code, rec.Body.String())
	}
	code, body := s.redeem(inv.Token, "novo", "senha-longa", "")
	if code != http.StatusCreated {
		t.Fatalf("redeem: %d %s", code, body)
	}
	var out AuthResponse
	json.Unmarshal([]byte(body), &out)
	if rec := s.req("GET", "/probe", out.Token, ""); rec.Code != 200 {
		t.Errorf("the session returned by redeem does not work: %d", rec.Code)
	}
	if s.scalar(t, `SELECT role FROM users WHERE username = 'novo'`) != "admin" {
		t.Error("the account did not get the invited role")
	}
	if got := s.scalar(t, `SELECT (SELECT username FROM users WHERE id = used_by) || '/' || (used_at IS NOT NULL)::text FROM invitations`); got != "novo/true" {
		t.Errorf("invitation not marked as used: %q", got)
	}
	if got := s.scalar(t, `SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'invitation.%'`); got != "invitation.create,invitation.redeem" {
		t.Errorf("audit = %q", got)
	}

	// Single use: the same link does nothing the second time.
	if code, _ := s.redeem(inv.Token, "outro", "senha-longa", ""); code != http.StatusNotFound {
		t.Errorf("second redeem: %d, want 404", code)
	}
	if code, _ := s.redeem(inv.Token, "novo", "senha-longa", ""); code != http.StatusNotFound {
		t.Errorf("second redeem by the same name: %d, want 404", code)
	}
	if n := s.scalar(t, `SELECT count(*) FROM users`); n != "2" {
		t.Errorf("accounts = %s, want 2", n)
	}
	// The new account really works with its password.
	if fresh := s.login(t, "novo", "senha-longa"); fresh == "" {
		t.Error("cannot sign in with the chosen password")
	}
}

func TestInvitation_ExpiredRevokedUsedAndUnknownAllAnswerTheSame(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	boss := s.login(t, "boss", "s3cret")

	expired := s.invite(t, boss, `{}`, 201)
	s.db.Exec(`UPDATE invitations SET expires_at = now() - interval '1 second' WHERE id = $1`, expired.ID)
	revoked := s.invite(t, boss, `{}`, 201)
	if rec := s.req("DELETE", "/invitations/"+revoked.ID, boss, ""); rec.Code != 204 {
		t.Fatalf("revoke: %d", rec.Code)
	}
	used := s.invite(t, boss, `{}`, 201)
	if code, _ := s.redeem(used.Token, "primeiro", "senha-longa", ""); code != 201 {
		t.Fatalf("redeem: %d", code)
	}

	answers := map[string]bool{}
	for name, token := range map[string]string{"expired": expired.Token, "revoked": revoked.Token, "used": used.Token, "unknown": "nao-existe"} {
		check := s.req("GET", "/auth/invitation?token="+token, "", "")
		code, body := s.redeem(token, "intruso-"+name, "senha-longa", "")
		if check.Code != 404 || code != 404 {
			t.Errorf("%s: check %d, redeem %d, want 404 for both", name, check.Code, code)
		}
		answers[check.Body.String()+"|"+body] = true
	}
	if len(answers) != 1 {
		t.Errorf("the answers differ, so they reveal which case it was: %v", answers)
	}
	if n := s.scalar(t, `SELECT count(*) FROM users WHERE username LIKE 'intruso-%'`); n != "0" {
		t.Errorf("%s account(s) created from an invalid link", n)
	}

	states := map[string]string{}
	var list struct{ Data []struct{ ID, State string } }
	json.Unmarshal(s.req("GET", "/invitations", boss, "").Body.Bytes(), &list)
	for _, i := range list.Data {
		states[i.ID] = i.State
	}
	if states[expired.ID] != "expired" || states[revoked.ID] != "revoked" || states[used.ID] != "used" {
		t.Errorf("states = %v", states)
	}
}

func TestInvitation_ConcurrentRedeemsCreateExactlyOneAccount(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	inv := s.invite(t, s.login(t, "boss", "s3cret"), `{}`, 201)

	const tries = 24
	codes := make([]int, tries)
	var wg sync.WaitGroup
	for i := 0; i < tries; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = s.redeem(inv.Token, fmt.Sprintf("pessoa%d", i), "senha-longa", "")
		}(i)
	}
	wg.Wait()

	created := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			created++
		}
	}
	if created != 1 || s.scalar(t, `SELECT count(*) FROM users WHERE username LIKE 'pessoa%'`) != "1" {
		t.Errorf("codes = %v, accounts = %s; want exactly one", codes, s.scalar(t, `SELECT count(*) FROM users WHERE username LIKE 'pessoa%'`))
	}
}

func TestInvitation_ARefusedRedeemDoesNotBurnTheLink(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "ocupado", "reader", "s3cret")
	inv := s.invite(t, s.login(t, "boss", "s3cret"), `{"email":"ana@example.com"}`, 201)

	if code, _ := s.redeem(inv.Token, "ana", "curta", ""); code != 400 {
		t.Errorf("short password: %d, want 400", code)
	}
	if code, _ := s.redeem(inv.Token, "ana", "senha-longa", "outra@example.com"); code != 403 {
		t.Errorf("another address: %d, want 403", code)
	}
	if code, _ := s.redeem(inv.Token, "ocupado", "senha-longa", ""); code != 409 {
		t.Errorf("name taken: %d, want 409", code)
	}
	if s.scalar(t, `SELECT count(*) FROM invitations WHERE used_at IS NULL`) != "1" {
		t.Fatal("a refused attempt used up the invitation")
	}
	// Without an address the invitation's own is used; same address in other case works too.
	if code, body := s.redeem(inv.Token, "ana", "senha-longa", "ANA@example.com"); code != 201 {
		t.Fatalf("right address: %d %s", code, body)
	}
	if got := s.scalar(t, `SELECT email FROM users WHERE username = 'ana'`); got != "ana@example.com" {
		t.Errorf("account email = %q", got)
	}
}

func TestInvitation_RevokingFollowsThePolicyAndOnlyPendingOnes(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addUserWithPassword(t, "adm", "admin", "s3cret")
	boss, adm := s.login(t, "boss", "s3cret"), s.login(t, "adm", "s3cret")
	adminInvite := s.invite(t, boss, `{"role":"admin"}`, 201)
	readerInvite := s.invite(t, adm, `{}`, 201)

	if rec := s.req("DELETE", "/invitations/"+adminInvite.ID, adm, ""); rec.Code != 403 {
		t.Errorf("admin revoking an admin invitation: %d, want 403", rec.Code)
	}
	if rec := s.req("DELETE", "/invitations/"+readerInvite.ID, adm, ""); rec.Code != 204 {
		t.Errorf("admin revoking a reader invitation: %d", rec.Code)
	}
	if rec := s.req("DELETE", "/invitations/"+readerInvite.ID, adm, ""); rec.Code != 409 {
		t.Errorf("revoking twice: %d, want 409", rec.Code)
	}
	if rec := s.req("DELETE", "/invitations/b190281f-fe3e-4308-ad71-000000000000", boss, ""); rec.Code != 404 {
		t.Errorf("unknown: %d, want 404", rec.Code)
	}
	if rec := s.req("DELETE", "/invitations/xyz", boss, ""); rec.Code != 404 {
		t.Errorf("malformed: %d, want 404", rec.Code)
	}
	s.redeem(adminInvite.Token, "novo-admin", "senha-longa", "")
	if rec := s.req("DELETE", "/invitations/"+adminInvite.ID, boss, ""); rec.Code != 409 {
		t.Errorf("revoking a used invitation: %d, want 409", rec.Code)
	}
	if code, _ := s.redeem(readerInvite.Token, "tarde", "senha-longa", ""); code != 404 {
		t.Errorf("redeeming a revoked link: %d, want 404", code)
	}

	// The list tells each caller what it may revoke.
	s.invite(t, boss, `{"role":"admin"}`, 201)
	var list struct {
		Data []struct {
			Role      string
			CanRevoke bool
		}
	}
	json.Unmarshal(s.req("GET", "/invitations", adm, "").Body.Bytes(), &list)
	for _, i := range list.Data {
		if i.Role == "admin" && i.CanRevoke {
			t.Error("an admin is told it may revoke an admin invitation")
		}
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'invitation.revoke'`) != "1" {
		t.Error("revocation should be audited once")
	}
}

func TestBlock_RevokesTheInvitationsThatAccountHandedOut(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	admID := s.addUserWithPassword(t, "adm", "admin", "s3cret")
	boss, adm := s.login(t, "boss", "s3cret"), s.login(t, "adm", "s3cret")
	theirs := s.invite(t, adm, `{}`, 201)
	others := s.invite(t, boss, `{}`, 201)

	if rec := s.req("POST", "/users/"+admID+"/block", boss, ""); rec.Code != 204 {
		t.Fatalf("block: %d", rec.Code)
	}
	if code, _ := s.redeem(theirs.Token, "convidado", "senha-longa", ""); code != 404 {
		t.Errorf("a blocked account's invitation still works: %d", code)
	}
	if code, _ := s.redeem(others.Token, "outro", "senha-longa", ""); code != 201 {
		t.Errorf("someone else's invitation was revoked too: %d", code)
	}
}
