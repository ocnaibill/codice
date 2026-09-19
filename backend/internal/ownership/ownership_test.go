package ownership_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/ownership"
	"github.com/ocnaibill/codice/backend/internal/testdb"
	"golang.org/x/crypto/bcrypt"
)

type env struct {
	t  *testing.T
	db *sql.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return &env{t, db}
}

func (e *env) user(name, role string) string {
	e.t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("s3cret"), 4)
	var id string
	if err := e.db.QueryRow(`INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
		name, name+"@x", string(hash), role).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) scalar(query string, args ...any) string {
	e.t.Helper()
	var s sql.NullString
	if err := e.db.QueryRow(query, args...).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s.String
}

func (e *env) roles() string {
	return e.scalar(`SELECT string_agg(username || ':' || role, ',' ORDER BY username) FROM users`)
}

func (e *env) session(userID string) {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO sessions (user_id, expires_at) VALUES ($1, now() + interval '1 day')`, userID); err != nil {
		e.t.Fatal(err)
	}
}

var ctx = context.Background()

func TestStart_OnlyTheOwnerAndOnlyToAnEligibleAccount(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	adm := e.user("adm", "admin")
	ana := e.user("ana", "reader")
	blocked := e.user("blk", "reader")
	e.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, blocked)
	noPass := e.user("nop", "reader")
	e.db.Exec(`UPDATE users SET password_hash = NULL WHERE id = $1`, noPass)
	linked := e.user("ldap", "reader")
	e.db.Exec(`INSERT INTO external_identities (user_id, provider, subject) VALUES ($1, 'ldap', 'uid=x')`, linked)
	before := e.roles()

	cases := map[string]struct {
		owner, target, role string
		want                error
	}{
		"an admin starts":                  {adm, ana, "reader", ownership.ErrNotOwner},
		"a reader starts":                  {ana, adm, "reader", ownership.ErrNotOwner},
		"to a blocked account":             {boss, blocked, "reader", ownership.ErrNotEligible},
		"to one with no password":          {boss, noPass, "reader", ownership.ErrNotEligible},
		"to one with an external identity": {boss, linked, "reader", ownership.ErrNotEligible},
		"to themself":                      {boss, boss, "reader", ownership.ErrNotEligible},
		"no former role":                   {boss, ana, "", ownership.ErrBadRole},
		"former role owner":                {boss, ana, "owner", ownership.ErrBadRole},
		"unknown target":                   {boss, "b190281f-fe3e-4308-ad71-000000000000", "reader", ownership.ErrNotFound},
	}
	for name, c := range cases {
		if _, err := ownership.Start(ctx, e.db, c.owner, c.target, c.role); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if e.scalar(`SELECT count(*) FROM ownership_transfers`) != "0" || e.roles() != before {
		t.Fatal("a refused start left something behind")
	}

	// Starting changes nothing for anyone: it only makes an offer.
	if _, err := ownership.Start(ctx, e.db, boss, ana, "admin"); err != nil {
		t.Fatal(err)
	}
	if e.roles() != before {
		t.Errorf("roles changed before acceptance: %s", e.roles())
	}
	if _, err := ownership.Start(ctx, e.db, boss, adm, "reader"); !errors.Is(err, ownership.ErrPending) {
		t.Errorf("second transfer while one waits: %v, want ErrPending", err)
	}
}

func TestAccept_SwapsTheRolesAtomicallyAndLeavesOneOwner(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	ana := e.user("ana", "reader")
	other := e.user("bob", "reader")
	if _, err := ownership.Start(ctx, e.db, boss, ana, "admin"); err != nil {
		t.Fatal(err)
	}

	if _, err := ownership.Accept(ctx, e.db, other); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("someone else accepting: %v, want ErrNoTransfer", err)
	}
	if _, err := ownership.Accept(ctx, e.db, boss); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("the owner accepting their own offer: %v, want ErrNoTransfer", err)
	}
	if e.roles() != "ana:reader,bob:reader,boss:owner" {
		t.Fatalf("a refused acceptance changed roles: %s", e.roles())
	}

	if _, err := ownership.Accept(ctx, e.db, ana); err != nil {
		t.Fatal(err)
	}
	if e.roles() != "ana:owner,bob:reader,boss:admin" {
		t.Errorf("roles = %s; the former owner should become the admin they chose", e.roles())
	}
	if e.scalar(`SELECT count(*) FROM users WHERE role = 'owner'`) != "1" {
		t.Error("there must be exactly one owner")
	}
	if e.scalar(`SELECT state FROM ownership_transfers`) != "accepted" {
		t.Error("the transfer was not closed")
	}
	if _, err := ownership.Accept(ctx, e.db, ana); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("accepting twice: %v, want ErrNoTransfer", err)
	}
	if got := e.scalar(`SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'ownership.%'`); got != "ownership.transfer_start,ownership.transfer_accept" {
		t.Errorf("audit = %q", got)
	}
}

func TestAccept_RefusesWhenTheTargetChangedAndKeepsTheOwner(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	ana := e.user("ana", "reader")
	ownership.Start(ctx, e.db, boss, ana, "reader")

	e.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, ana) // blocked while the offer waited
	if _, err := ownership.Accept(ctx, e.db, ana); !errors.Is(err, ownership.ErrNotEligible) {
		t.Errorf("blocked target: %v, want ErrNotEligible", err)
	}
	if e.roles() != "ana:reader,boss:owner" {
		t.Errorf("a failed acceptance changed roles: %s", e.roles())
	}
	if e.scalar(`SELECT state FROM ownership_transfers`) != "pending" {
		t.Error("a failed acceptance should not close the offer")
	}

	// The database itself refuses an owner with an external identity.
	e.db.Exec(`UPDATE users SET blocked_at = NULL WHERE id = $1`, ana)
	e.db.Exec(`INSERT INTO external_identities (user_id, provider, subject) VALUES ($1, 'ldap', 'uid=ana')`, ana)
	if _, err := ownership.Accept(ctx, e.db, ana); !errors.Is(err, ownership.ErrNotEligible) {
		t.Errorf("target linked meanwhile: %v, want ErrNotEligible", err)
	}
	if e.roles() != "ana:reader,boss:owner" {
		t.Errorf("roles = %s", e.roles())
	}
}

func TestTransfer_ExpiresCancelsAndDeclines(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	ana := e.user("ana", "reader")

	ownership.Start(ctx, e.db, boss, ana, "reader")
	e.db.Exec(`UPDATE ownership_transfers SET expires_at = now() - interval '1 second'`)
	if _, err := ownership.Accept(ctx, e.db, ana); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("accepting an expired offer: %v", err)
	}
	if _, _, err := ownership.Pending(ctx, e.db, ana); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("an expired offer is still shown: %v", err)
	}
	// An expired offer does not block a new one.
	if _, err := ownership.Start(ctx, e.db, boss, ana, "reader"); err != nil {
		t.Fatalf("new offer after expiry: %v", err)
	}

	if err := ownership.Decline(ctx, e.db, boss); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("the owner declining their own offer: %v", err)
	}
	if err := ownership.Cancel(ctx, e.db, ana); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("the target cancelling: %v", err)
	}
	if err := ownership.Cancel(ctx, e.db, boss); err != nil {
		t.Fatal(err)
	}
	if _, err := ownership.Accept(ctx, e.db, ana); !errors.Is(err, ownership.ErrNoTransfer) {
		t.Errorf("accepting a cancelled offer: %v", err)
	}

	ownership.Start(ctx, e.db, boss, ana, "reader")
	if err := ownership.Decline(ctx, e.db, ana); err != nil {
		t.Fatal(err)
	}
	if e.roles() != "ana:reader,boss:owner" {
		t.Errorf("roles = %s", e.roles())
	}
}

func TestTransfer_ConcurrentAcceptsAndStartsNeverLeaveTwoOwners(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	ana := e.user("ana", "reader")
	ownership.Start(ctx, e.db, boss, ana, "reader")

	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = ownership.Accept(ctx, e.db, ana)
		}(i)
	}
	wg.Wait()
	won := 0
	for _, err := range results {
		if err == nil {
			won++
		}
	}
	if won != 1 {
		t.Errorf("%d acceptances succeeded, want 1: %v", won, results)
	}
	if e.roles() != "ana:owner,boss:reader" || e.scalar(`SELECT count(*) FROM users WHERE role = 'owner'`) != "1" {
		t.Errorf("roles = %s", e.roles())
	}
}

func TestRecoverAccess_EndsTheOwnersAccessAndPrintsOneLink(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	ana := e.user("ana", "reader")
	e.session(boss)
	e.session(ana)

	first, _, name, err := ownership.RecoverAccess(ctx, e.db)
	if err != nil || name != "boss" || len(first) < 40 {
		t.Fatalf("recover: %q %q %v", first, name, err)
	}
	if e.scalar(`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, boss) != "0" {
		t.Error("the owner's sessions were not ended")
	}
	if e.scalar(`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, ana) != "1" {
		t.Error("someone else's session was ended")
	}
	if e.scalar(`SELECT count(*) FROM security_notices WHERE user_id = $1 AND kind = 'owner_recovery_reset' AND acknowledged_at IS NULL`, boss) != "1" {
		t.Error("the owner is not told at the next sign-in")
	}
	if e.scalar(`SELECT count(*) FROM audit_log WHERE action = 'ownership.recovery_reset'`) != "1" {
		t.Error("recovery was not audited")
	}
	if e.scalar(`SELECT count(*) FROM audit_log WHERE details::text LIKE '%' || $1 || '%'`, first) != "0" {
		t.Error("the link reached the audit log")
	}
	// Running it again invalidates the earlier link: only the last one printed works.
	second, _, _, _ := ownership.RecoverAccess(ctx, e.db)
	if e.scalar(`SELECT count(*) FROM password_resets WHERE decision = 'approved' AND used_at IS NULL AND token_expires_at > now()`) != "1" || first == second {
		t.Error("an older recovery link is still valid")
	}
}

func TestRecoverTransfer_MovesOwnershipAtOnceAndEndsTheOldOwnersAccess(t *testing.T) {
	e := newEnv(t)
	boss := e.user("boss", "owner")
	ana := e.user("ana", "reader")
	e.user("bob", "reader")
	e.session(boss)
	ownership.Start(ctx, e.db, boss, e.scalarID("bob"), "reader") // an offer the recovery makes moot

	// Refusals leave everything as it was.
	before := e.roles()
	if _, err := ownership.RecoverTransfer(ctx, e.db, "nobody", "reader"); !errors.Is(err, ownership.ErrNotFound) {
		t.Errorf("unknown account: %v", err)
	}
	if _, err := ownership.RecoverTransfer(ctx, e.db, "ana", "owner"); !errors.Is(err, ownership.ErrBadRole) {
		t.Errorf("bad role: %v", err)
	}
	if _, err := ownership.RecoverTransfer(ctx, e.db, "boss", "reader"); !errors.Is(err, ownership.ErrNotEligible) {
		t.Errorf("to the owner themself: %v", err)
	}
	e.db.Exec(`UPDATE users SET blocked_at = now() WHERE id = $1`, ana)
	if _, err := ownership.RecoverTransfer(ctx, e.db, "ana", "reader"); !errors.Is(err, ownership.ErrNotEligible) {
		t.Errorf("blocked account: %v", err)
	}
	e.db.Exec(`UPDATE users SET blocked_at = NULL WHERE id = $1`, ana)
	if e.roles() != before || e.scalar(`SELECT count(*) FROM sessions WHERE revoked_at IS NOT NULL`) != "0" {
		t.Fatal("a refused recovery changed something")
	}

	from, err := ownership.RecoverTransfer(ctx, e.db, "ana", "reader")
	if err != nil || from != "boss" {
		t.Fatalf("recover transfer: %q %v", from, err)
	}
	if e.roles() != "ana:owner,bob:reader,boss:reader" {
		t.Errorf("roles = %s", e.roles())
	}
	if e.scalar(`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, boss) != "0" {
		t.Error("the former owner keeps a session")
	}
	if e.scalar(`SELECT count(*) FROM ownership_transfers WHERE state = 'pending'`) != "0" {
		t.Error("an old offer is still pending")
	}
	if e.scalar(`SELECT count(*) FROM security_notices WHERE kind = 'owner_recovery_transfer'`) != "2" {
		t.Error("both accounts should be told")
	}
	if e.scalar(`SELECT count(*) FROM audit_log WHERE action = 'ownership.recovery_transfer'`) != "1" {
		t.Error("recovery was not audited")
	}
}

func (e *env) scalarID(username string) string {
	return e.scalar(`SELECT id FROM users WHERE username = $1`, username)
}
