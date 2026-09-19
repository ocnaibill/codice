package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/identity"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
	"github.com/ocnaibill/codice/backend/internal/ldapauth/ldaptest"
)

// withDirectory starts a directory with these people and makes the stack use it.
func withDirectory(t *testing.T, s *authStack, users ...ldaptest.User) *ldaptest.Server {
	t.Helper()
	dir := ldaptest.Start(t, users...)
	c, err := ldapauth.New(ldapauth.Config{
		URL: dir.URL(), BindDN: ldaptest.ServiceDN, BindPassword: ldaptest.ServicePassword, BaseDN: ldaptest.BaseDN,
		UserFilter: "(uid={username})", UsernameAttr: "uid", IDAttr: "entryUUID", EmailAttr: "mail",
		AllowPlaintext: true, Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.authH.Directory = c
	s.ldapAdmin.Directory = c
	return dir
}

func (s *authStack) allowCreate(t *testing.T, on bool) {
	t.Helper()
	if err := identity.SetPolicy(context.Background(), s.db, identity.Policy{AllowCreate: on, RevalidateHours: 24}, ""); err != nil {
		t.Fatal(err)
	}
}

// addLinked makes an account that signs in through the directory: no local password.
func (s *authStack) addLinked(t *testing.T, name, role, uuid string) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow(`INSERT INTO users (username, email, role) VALUES ($1, $2, $3) RETURNING id`, name, name+"@x", role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO external_identities (user_id, provider, subject, last_verified_at) VALUES ($1, 'ldap', $2, now())`, id, uuid); err != nil {
		t.Fatal(err)
	}
	return id
}

func (s *authStack) tryLogin(user, pass string) *responseView {
	rec := s.req("POST", "/auth/login", "", fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass))
	return &responseView{rec.Code, rec.Body.String()}
}

type responseView struct {
	code int
	body string
}

func (v *responseView) token() string {
	var out AuthResponse
	json.Unmarshal([]byte(v.body), &out)
	return out.Token
}

func (v *responseView) ticket() string {
	var out struct{ Ticket string }
	json.Unmarshal([]byte(v.body), &out)
	return out.Ticket
}

var (
	dirAna = ldaptest.User{Name: "ana", Password: "senha-do-diretorio", Email: "ana@example.com", UUID: "uuid-ana"}
	dirBob = ldaptest.User{Name: "bob", Password: "senha-do-bob", Email: "bob@example.com", UUID: "uuid-bob"}
)

func TestLDAPLogin_ALinkedAccountSignsInThroughTheDirectory(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna)
	id := s.addLinked(t, "ana", "reader", "uuid-ana")

	ok := s.tryLogin("ana", "senha-do-diretorio")
	if ok.code != 200 || ok.token() == "" {
		t.Fatalf("directory login: %d %s", ok.code, ok.body)
	}
	if rec := s.req("GET", "/probe", ok.token(), ""); rec.Code != 200 {
		t.Errorf("the session does not work: %d", rec.Code)
	}
	bad := s.tryLogin("ana", "errada")
	if bad.code != http.StatusUnauthorized || bad.body != invalidLoginMessage+"\n" {
		t.Errorf("wrong directory password: %d %q", bad.code, bad.body)
	}
	if v := s.tryLogin("ana", ""); v.code != http.StatusUnauthorized {
		t.Errorf("an empty password: %d, want 401", v.code)
	}
	for _, b := range dir.Binds() {
		if strings.HasSuffix(b, "|") && !strings.HasPrefix(b, "cn=svc") {
			t.Errorf("an empty password reached the directory as a bind: %q", b)
		}
	}
	if s.scalar(t, `SELECT (last_verified_at > now() - interval '1 minute')::text FROM external_identities WHERE user_id = $1`, id) != "true" {
		t.Error("the login did not record the directory's confirmation")
	}
}

func TestLDAPLogin_TheOwnerIsAlwaysLocal(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, ldaptest.User{Name: "boss", Password: "senha-do-diretorio", UUID: "uuid-boss"})
	s.allowCreate(t, true)
	s.addUserWithPassword(t, "boss", "owner", "senha-local")

	if v := s.tryLogin("boss", "senha-local"); v.code != 200 {
		t.Fatalf("the owner's local login: %d", v.code)
	}
	if v := s.tryLogin("boss", "senha-do-diretorio"); v.code != http.StatusUnauthorized {
		t.Errorf("the owner with the directory password: %d, want 401", v.code)
	}
	if len(dir.Searches()) != 0 || len(dir.Binds()) != 0 {
		t.Errorf("the directory was contacted for the owner: %v %v", dir.Searches(), dir.Binds())
	}
	// And an identity cannot be tied to the owner, even by hand.
	if _, err := s.db.Exec(`INSERT INTO external_identities (user_id, provider, subject) SELECT id, 'ldap', 'x' FROM users WHERE username = 'boss'`); err == nil {
		t.Error("the database accepted an external identity for the owner")
	}
}

func TestLDAPLogin_BlockedAccountsAreRefusedWithoutAskingTheDirectory(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna)
	id := s.addLinked(t, "ana", "reader", "uuid-ana")
	s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, id)

	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != http.StatusUnauthorized || v.body != invalidLoginMessage+"\n" {
		t.Errorf("blocked: %d %q", v.code, v.body)
	}
	if len(dir.Binds()) != 0 {
		t.Errorf("the directory was asked about a blocked account: %v", dir.Binds())
	}
}

func TestLDAPLogin_ADirectoryOutageIsDistinctAndOnlyHitsWhoNeedsIt(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna)
	s.addLinked(t, "ana", "reader", "uuid-ana")
	s.addUserWithPassword(t, "loc", "reader", "senha-local")
	dir.SetDown(true)

	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != http.StatusServiceUnavailable {
		t.Errorf("linked account, directory down: %d, want 503 (operational, not a wrong password)", v.code)
	}
	if v := s.tryLogin("loc", "senha-local"); v.code != 200 {
		t.Errorf("a local account during the outage: %d, want 200", v.code)
	}
	if v := s.tryLogin("loc", "errada"); v.code != http.StatusUnauthorized {
		t.Errorf("a wrong local password during the outage: %d, want 401", v.code)
	}
	if v := s.tryLogin("desconhecido", "x"); v.code != http.StatusUnauthorized {
		t.Errorf("an unknown name: %d, want 401", v.code)
	}
	dir.SetDown(false)
	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != 200 {
		t.Errorf("after the outage: %d", v.code)
	}
}

func TestLDAPLogin_ARenameOrRemovalInTheDirectoryIsFollowedByTheStableID(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna)
	s.addLinked(t, "ana", "reader", "uuid-ana")
	dir.Rename("ana", "ana.silva")

	// The account is still "ana" here; the entry is found by its id, not its name.
	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != 200 {
		t.Errorf("after a rename: %d, want 200", v.code)
	}
	dir.Remove("ana.silva")
	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != http.StatusUnauthorized {
		t.Errorf("removed from the directory: %d, want 401", v.code)
	}
}

func TestLDAPLogin_FirstLoginCreatesAReaderOnlyWhenTheOwnerAllowedIt(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, dirAna)

	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != http.StatusUnauthorized {
		t.Fatalf("with creation off: %d, want 401", v.code)
	}
	if s.scalar(t, `SELECT count(*) FROM users`) != "0" {
		t.Fatal("an account was created with the policy off")
	}

	s.allowCreate(t, true)
	if v := s.tryLogin("ana", "errada"); v.code != http.StatusUnauthorized {
		t.Errorf("a wrong directory password: %d, want 401", v.code)
	}
	if v := s.tryLogin("ninguem", "x"); v.code != http.StatusUnauthorized {
		t.Errorf("a name the directory does not have: %d, want 401", v.code)
	}
	if s.scalar(t, `SELECT count(*) FROM users`) != "0" {
		t.Fatal("a failed login created an account")
	}

	first := s.tryLogin("ana", "senha-do-diretorio")
	if first.code != 200 {
		t.Fatalf("first login: %d %s", first.code, first.body)
	}
	if got := s.scalar(t, `SELECT role || '/' || username || '/' || email || '/' || (password_hash IS NULL)::text FROM users`); got != "reader/ana/ana@example.com/true" {
		t.Errorf("the created account = %q; it must be a reader with no local password", got)
	}
	if s.scalar(t, `SELECT count(*) FROM external_identities WHERE subject = 'uuid-ana'`) != "1" {
		t.Error("the identity was not recorded")
	}
	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != 200 || s.scalar(t, `SELECT count(*) FROM users`) != "1" {
		t.Errorf("the second login must reuse the account: %d, accounts = %s", v.code, s.scalar(t, `SELECT count(*) FROM users`))
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'identity.create'`) != "1" {
		t.Error("the creation was not audited")
	}
}

func TestLDAPLogin_ConcurrentFirstLoginsCreateOneAccount(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, dirAna)
	s.allowCreate(t, true)

	const tries = 10
	codes := make([]int, tries)
	var wg sync.WaitGroup
	for i := 0; i < tries; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); codes[i] = s.tryLogin("ana", "senha-do-diretorio").code }(i)
	}
	wg.Wait()
	if s.scalar(t, `SELECT count(*) FROM users`) != "1" || s.scalar(t, `SELECT count(*) FROM external_identities`) != "1" {
		t.Errorf("codes = %v; accounts = %s, identities = %s; want one of each", codes,
			s.scalar(t, `SELECT count(*) FROM users`), s.scalar(t, `SELECT count(*) FROM external_identities`))
	}
	for _, c := range codes {
		if c != 200 {
			t.Errorf("codes = %v; every login should succeed", codes)
			break
		}
	}
}

func TestLDAPLogin_ASameNamedLocalAccountIsJoinedOnlyWithBothPasswords(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna)
	s.addUserWithPassword(t, "ana", "reader", "senha-local")

	// The local password is a plain local login and never touches the directory.
	if v := s.tryLogin("ana", "senha-local"); v.code != 200 {
		t.Fatalf("local login: %d", v.code)
	}
	if len(dir.Binds()) != 0 {
		t.Fatalf("the directory was contacted for a local login: %v", dir.Binds())
	}
	// A password that is neither: refused, and the directory saw it only as a bind.
	if v := s.tryLogin("ana", "nenhuma-das-duas"); v.code != http.StatusUnauthorized {
		t.Errorf("neither password: %d, want 401", v.code)
	}

	// The directory password alone opens NO session: it starts the link.
	step := s.tryLogin("ana", "senha-do-diretorio")
	if step.code != http.StatusAccepted || step.token() != "" || step.ticket() == "" {
		t.Fatalf("directory password on a local account: %d %s", step.code, step.body)
	}
	if s.scalar(t, `SELECT count(*) FROM external_identities`) != "0" {
		t.Fatal("the identity was linked before the local password was proven")
	}
	if s.scalar(t, `SELECT count(*) FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = 'ana') AND revoked_at IS NULL`) != "1" {
		t.Error("a session was opened by the first step")
	}

	link := func(ticket, pw string) *responseView {
		rec := s.req("POST", "/auth/link", "", fmt.Sprintf(`{"ticket":%q,"password":%q}`, ticket, pw))
		return &responseView{rec.Code, rec.Body.String()}
	}
	if v := link(step.ticket(), "errada"); v.code != http.StatusUnauthorized || v.body != invalidLoginMessage+"\n" {
		t.Errorf("a wrong local password: %d %q", v.code, v.body)
	}
	if v := link("ticket-inventado", "senha-local"); v.code != http.StatusUnauthorized || v.body != invalidLoginMessage+"\n" {
		t.Errorf("an unknown ticket: %d %q (must look the same)", v.code, v.body)
	}
	done := link(step.ticket(), "senha-local")
	if done.code != 200 || done.token() == "" {
		t.Fatalf("link: %d %s", done.code, done.body)
	}
	if s.scalar(t, `SELECT count(*) FROM external_identities WHERE subject = 'uuid-ana'`) != "1" {
		t.Error("the identity was not linked")
	}
	if v := link(step.ticket(), "senha-local"); v.code != http.StatusUnauthorized {
		t.Errorf("a ticket used twice: %d, want 401", v.code)
	}
	// From now on the account signs in through the directory.
	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != 200 {
		t.Errorf("after linking, the directory password: %d", v.code)
	}
	if v := s.tryLogin("ana", "senha-local"); v.code != http.StatusUnauthorized {
		t.Errorf("after linking, the local password: %d, want 401 (DEC-072: a linked account is checked in the directory)", v.code)
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'identity.link'`) != "1" {
		t.Error("the link was not audited")
	}
}

func TestLDAPLink_ATicketIsBurntByWrongPasswordsAndByTime(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, dirAna)
	s.addUserWithPassword(t, "ana", "reader", "senha-local")
	link := func(ticket, pw string) int {
		return s.req("POST", "/auth/link", "", fmt.Sprintf(`{"ticket":%q,"password":%q}`, ticket, pw)).Code
	}

	step := s.tryLogin("ana", "senha-do-diretorio")
	for i := 0; i < identity.MaxTicketAttempts; i++ {
		if code := link(step.ticket(), "errada"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code := link(step.ticket(), "senha-local"); code != http.StatusUnauthorized {
		t.Errorf("the right password after too many wrong ones: %d, want 401 (the ticket is burnt)", code)
	}

	expired := s.tryLogin("ana", "senha-do-diretorio")
	s.db.Exec(`UPDATE link_tickets SET expires_at = now() - interval '1 second' WHERE used_at IS NULL`)
	if code := link(expired.ticket(), "senha-local"); code != http.StatusUnauthorized {
		t.Errorf("an expired ticket: %d, want 401", code)
	}
	if s.scalar(t, `SELECT count(*) FROM external_identities`) != "0" {
		t.Error("nothing should have been linked")
	}
}

func TestLDAPLogin_AnEntryAlreadyTiedToAnotherAccountIsNotOfferedAgain(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, dirAna)
	s.addLinked(t, "outra-conta", "reader", "uuid-ana") // the entry belongs to this one
	s.addUserWithPassword(t, "ana", "reader", "senha-local")

	if v := s.tryLogin("ana", "senha-do-diretorio"); v.code != http.StatusUnauthorized {
		t.Errorf("%d %s; the entry is taken, so no link may be offered", v.code, v.body)
	}
}

func TestLDAPLogin_ASharedEmailAloneJoinsNothing(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, ldaptest.User{Name: "carol", Password: "senha-carol", Email: "carol@example.com", UUID: "uuid-carol"})
	s.allowCreate(t, true)
	localID := s.addUserWithPassword(t, "carolina", "reader", "senha-local")
	s.db.Exec(`UPDATE users SET email = 'carol@example.com' WHERE id = $1`, localID)

	step := s.tryLogin("carol", "senha-carol")
	if step.code != http.StatusAccepted || step.token() != "" {
		t.Fatalf("the same e-mail must ask for the local password: %d %s", step.code, step.body)
	}
	if s.scalar(t, `SELECT count(*) FROM users`) != "1" || s.scalar(t, `SELECT count(*) FROM external_identities`) != "0" {
		t.Fatal("an e-mail match created or joined something on its own")
	}
	rec := s.req("POST", "/auth/link", "", fmt.Sprintf(`{"ticket":%q,"password":"errada"}`, step.ticket()))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("without the local password: %d", rec.Code)
	}
	rec = s.req("POST", "/auth/link", "", fmt.Sprintf(`{"ticket":%q,"password":"senha-local"}`, step.ticket()))
	if rec.Code != 200 || s.scalar(t, `SELECT username FROM users WHERE id = (SELECT user_id FROM external_identities)`) != "carolina" {
		t.Errorf("with both proofs: %d; the identity should join the existing account", rec.Code)
	}
	if s.scalar(t, `SELECT count(*) FROM users`) != "1" {
		t.Error("a second account was created")
	}
}

func TestLDAPLogin_TheOwnersEmailIsNeverACandidateAndDoesNotBlockTheNewAccount(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, ldaptest.User{Name: "dana", Password: "senha-dana", Email: "boss@x", UUID: "uuid-dana"})
	s.allowCreate(t, true)
	s.addUserWithPassword(t, "boss", "owner", "senha-local") // email boss@x

	if v := s.tryLogin("dana", "senha-dana"); v.code != 200 {
		t.Fatalf("a directory user whose address the owner already uses: %d %s", v.code, v.body)
	}
	if got := s.scalar(t, `SELECT role || '/' || email FROM users WHERE username = 'dana'`); got != "reader/dana@codice.local" {
		t.Errorf("account = %q; the owner's address must not be taken over", got)
	}
	if s.scalar(t, `SELECT count(*) FROM external_identities WHERE user_id = (SELECT id FROM users WHERE role = 'owner')`) != "0" {
		t.Error("the owner got an external identity")
	}
}

func TestLDAPLogin_EveryCredentialFailureLooksTheSame(t *testing.T) {
	s := newAuthStack(t)
	withDirectory(t, s, dirAna)
	s.allowCreate(t, true)
	s.addLinked(t, "ldapuser", "reader", "uuid-ldapuser")
	s.addUserWithPassword(t, "loc", "reader", "senha-local")
	blockedID := s.addUserWithPassword(t, "blk", "reader", "senha-local")
	s.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, blockedID)

	answers := map[string]bool{}
	for name, tc := range map[string][2]string{
		"unknown to both":                  {"ninguem", "x"},
		"wrong local password":             {"loc", "errada"},
		"blocked local":                    {"blk", "senha-local"},
		"linked, not in the directory":     {"ldapuser", "x"},
		"in the directory, wrong password": {"ana", "errada"},
	} {
		v := s.tryLogin(tc[0], tc[1])
		if v.code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, v.code)
		}
		answers[v.body] = true
	}
	if len(answers) != 1 {
		t.Errorf("the answers differ, so they reveal something: %v", answers)
	}
}

func TestIdentitySweep_RevalidatesWithACeilingAndNeverForLocalAccounts(t *testing.T) {
	s := newAuthStack(t)
	dir := withDirectory(t, s, dirAna, dirBob)
	s.addLinked(t, "ana", "reader", "uuid-ana")
	s.addLinked(t, "bob", "reader", "uuid-bob")
	s.addUserWithPassword(t, "loc", "reader", "senha-local")
	ana, bob, loc := s.tryLogin("ana", "senha-do-diretorio").token(), s.tryLogin("bob", "senha-do-bob").token(), s.login(t, "loc", "senha-local")
	sweep := func(ceiling time.Duration) identity.SweepResult {
		res, err := identity.Sweep(context.Background(), s.db, s.authH.Directory, ceiling)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	live := func(token string) bool { return s.req("GET", "/probe", token, "").Code == 200 }

	// Confirmed by the directory: everyone stays, and the confirmation is recorded.
	s.db.Exec(`UPDATE external_identities SET last_verified_at = now() - interval '20 hours'`)
	if res := sweep(24 * time.Hour); res.Checked != 2 || res.Confirmed != 2 || res.Revoked != 0 {
		t.Errorf("sweep = %+v", res)
	}
	if s.scalar(t, `SELECT count(*) FROM external_identities WHERE last_verified_at > now() - interval '1 minute'`) != "2" {
		t.Error("the confirmations were not recorded")
	}

	// Disabled in the directory: access ends at the next revalidation, only for that person.
	dir.Remove("bob")
	if res := sweep(24 * time.Hour); res.Revoked != 1 {
		t.Errorf("sweep = %+v", res)
	}
	if live(bob) || !live(ana) || !live(loc) {
		t.Errorf("after a removal: ana=%v bob=%v loc=%v; only bob should lose access", live(ana), live(bob), live(loc))
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'identity.deactivated'`) != "1" {
		t.Error("the revocation was not audited")
	}

	// Directory unreachable: kept until the ceiling, then ended, and nothing is deleted.
	dir.SetDown(true)
	if res := sweep(24 * time.Hour); res.Revoked != 0 || !live(ana) {
		t.Errorf("an outage inside the ceiling ended a session: %+v", res)
	}
	s.db.Exec(`UPDATE external_identities SET last_verified_at = now() - interval '25 hours'`)
	if res := sweep(24 * time.Hour); res.Revoked != 1 || live(ana) {
		t.Errorf("an outage beyond the ceiling: %+v, ana live = %v", res, live(ana))
	}
	if s.scalar(t, `SELECT count(*) FROM users WHERE username = 'ana'`) != "1" {
		t.Error("the account was deleted")
	}
	if !live(loc) {
		t.Error("a local account was affected")
	}
}

func TestLDAPAdmin_ShowsTheStateWithoutSecretsAndTheOwnerSetsThePolicy(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	boss := s.login(t, "boss", "s3cret")

	var state struct {
		Configured bool
		Host       string
		Policy     identity.Policy
	}
	json.Unmarshal(s.req("GET", "/admin/ldap", boss, "").Body.Bytes(), &state)
	if state.Configured || state.Policy.AllowCreate || state.Policy.RevalidateHours != 24 {
		t.Errorf("default state = %+v", state)
	}
	// Creating accounts from a directory that is not configured makes no sense.
	if code := s.req("PUT", "/admin/ldap/policy", boss, `{"allowCreate":true,"revalidateHours":24}`).Code; code != http.StatusConflict {
		t.Errorf("allowCreate without a directory: %d, want 409", code)
	}
	if rec := s.req("POST", "/admin/ldap/check", boss, ""); !strings.Contains(rec.Body.String(), `"not_configured"`) {
		t.Errorf("check without a directory: %s", rec.Body.String())
	}

	dir := withDirectory(t, s, dirAna)
	s.ldapAdmin.Host = "ldap://ldap.example"
	body := s.req("GET", "/admin/ldap", boss, "").Body.String()
	for _, secret := range []string{ldaptest.ServicePassword, ldaptest.ServiceDN} {
		if strings.Contains(body, secret) {
			t.Errorf("the state exposes %q", secret)
		}
	}
	if code := s.req("PUT", "/admin/ldap/policy", boss, `{"allowCreate":true,"revalidateHours":48}`).Code; code != 200 {
		t.Fatalf("set policy: %d", code)
	}
	if got, _ := identity.GetPolicy(context.Background(), s.db); !got.AllowCreate || got.RevalidateHours != 48 {
		t.Errorf("policy = %+v", got)
	}
	for _, bad := range []string{`{"allowCreate":true,"revalidateHours":0}`, `{"allowCreate":true,"revalidateHours":9999}`, `nope`} {
		if code := s.req("PUT", "/admin/ldap/policy", boss, bad).Code; code != http.StatusBadRequest {
			t.Errorf("policy %s: %d, want 400", bad, code)
		}
	}
	if s.scalar(t, `SELECT count(*) FROM audit_log WHERE action = 'ldap.policy'`) != "1" {
		t.Error("the policy change was not audited once")
	}

	if rec := s.req("POST", "/admin/ldap/check", boss, ""); !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("check: %s", rec.Body.String())
	}
	dir.SetDown(true)
	if rec := s.req("POST", "/admin/ldap/check", boss, ""); !strings.Contains(rec.Body.String(), `"unavailable"`) {
		t.Errorf("check with the directory down: %s", rec.Body.String())
	}
}

func TestListAccounts_MarksTheOnesThatSignInThroughTheDirectory(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	s.addLinked(t, "ana", "reader", "uuid-ana")
	boss := s.login(t, "boss", "s3cret")
	var list struct {
		Data []struct{ Username, External string }
	}
	json.Unmarshal(s.req("GET", "/users", boss, "").Body.Bytes(), &list)
	got := map[string]string{}
	for _, a := range list.Data {
		got[a.Username] = a.External
	}
	if got["ana"] != "ldap" || got["boss"] != "" {
		t.Errorf("external = %v", got)
	}
}
