package admincli_test

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/admincli"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
	"golang.org/x/crypto/bcrypt"
)

func setup(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("s3cret"), 4)
	for _, u := range [][2]string{{"boss", "owner"}, {"ana", "reader"}} {
		if _, err := db.Exec(`INSERT INTO users (username, email, password_hash, role) VALUES ($1, $2, $3, $4)`, u[0], u[0]+"@x", string(hash), u[1]); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func run(db *sql.DB, input string, args ...string) (string, error) {
	var out bytes.Buffer
	err := admincli.Run(context.Background(), db, args, strings.NewReader(input), &out)
	return out.String(), err
}

func roles(t *testing.T, db *sql.DB) string {
	t.Helper()
	var s string
	db.QueryRow(`SELECT string_agg(username || ':' || role, ',' ORDER BY username) FROM users`).Scan(&s)
	return s
}

func TestRecoverOwner_PrintsTheLinkOnlyToTheOperator(t *testing.T) {
	db := setup(t)
	out, err := run(db, "", "recover-owner", "--base-url", "https://codice.example/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "https://codice.example/?reset=") || !strings.Contains(out, "expires at") {
		t.Errorf("output = %q", out)
	}
	token := out[strings.Index(out, "?reset=")+len("?reset="):]
	token = strings.Fields(token)[0]
	var stored int
	db.QueryRow(`SELECT count(*) FROM password_resets WHERE token_hash = $1`, token).Scan(&stored)
	var leaked int
	db.QueryRow(`SELECT count(*) FROM audit_log WHERE details::text LIKE '%' || $1 || '%'`, token).Scan(&leaked)
	if stored != 0 || leaked != 0 {
		t.Error("the link itself was stored or audited")
	}
}

func TestTransferOwner_NeedsTheNameTypedAndDefaultsToTheConservativeRole(t *testing.T) {
	db := setup(t)
	if _, err := run(db, "nope\n", "transfer-owner", "--to", "ana"); err == nil {
		t.Error("a wrong confirmation was accepted")
	}
	if _, err := run(db, "", "transfer-owner", "--to", "ana"); err == nil {
		t.Error("no confirmation was accepted")
	}
	if roles(t, db) != "ana:reader,boss:owner" {
		t.Fatalf("an unconfirmed run changed roles: %s", roles(t, db))
	}
	if _, err := run(db, "ana\n", "transfer-owner", "--to", "ana"); err != nil {
		t.Fatal(err)
	}
	if roles(t, db) != "ana:owner,boss:reader" {
		t.Errorf("roles = %s (the former owner should default to reader)", roles(t, db))
	}
}

func TestTransferOwner_ChosenRoleYesFlagAndRefusals(t *testing.T) {
	db := setup(t)
	if _, err := run(db, "", "transfer-owner", "--to", "nobody", "--yes"); err == nil {
		t.Error("an unknown account was accepted")
	}
	if _, err := run(db, "", "transfer-owner", "--to", "ana", "--former-role", "owner", "--yes"); err == nil {
		t.Error("former role owner was accepted")
	}
	if _, err := run(db, "", "transfer-owner", "--yes"); err == nil {
		t.Error("a missing --to was accepted")
	}
	if roles(t, db) != "ana:reader,boss:owner" {
		t.Fatalf("a refused run changed roles: %s", roles(t, db))
	}
	if _, err := run(db, "", "transfer-owner", "--to", "ana", "--former-role", "admin", "--yes"); err != nil {
		t.Fatal(err)
	}
	if roles(t, db) != "ana:owner,boss:admin" {
		t.Errorf("roles = %s", roles(t, db))
	}
}

func TestUnknownOrMissingCommandDoesNothing(t *testing.T) {
	db := setup(t)
	if _, err := run(db, ""); err == nil {
		t.Error("no command was accepted")
	}
	if _, err := run(db, "", "make-me-owner"); err == nil {
		t.Error("an unknown command was accepted")
	}
	if out, err := run(db, "", "help"); err != nil || !strings.Contains(out, "recover-owner") {
		t.Errorf("help: %v %q", err, out)
	}
	if roles(t, db) != "ana:reader,boss:owner" {
		t.Error("something changed")
	}
}
