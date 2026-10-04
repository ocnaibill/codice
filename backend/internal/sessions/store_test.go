package sessions

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

var ctx = context.Background()

func newStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var ana, bia string
	db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('ana', 'a@x', 'reader') RETURNING id`).Scan(&ana)
	db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('bia', 'b@x', 'reader') RETURNING id`).Scan(&bia)
	return &Store{DB: db}, ana, bia
}

func column(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s.String
}

func TestCreateSession_KeepsTheDeviceTheAddressAndTheFirstUse(t *testing.T) {
	s, ana, _ := newStore(t)
	id, _, err := s.CreateSession(ctx, ana, "Mozilla/5.0 (X11; Linux) Firefox/130.0", "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	if got := column(t, s.DB, `SELECT ip FROM sessions WHERE id = $1`, id); got != "203.0.113.7" {
		t.Errorf("ip = %q", got)
	}
	if got := column(t, s.DB, `SELECT user_agent FROM sessions WHERE id = $1`, id); !strings.Contains(got, "Firefox") {
		t.Errorf("user agent = %q", got)
	}
	if got := column(t, s.DB, `SELECT (last_seen_at IS NOT NULL)::text FROM sessions WHERE id = $1`, id); got != "true" {
		t.Error("a login is the first use of its session")
	}
	// IPv6, written out in full, is the longest address.
	v6 := "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
	id6, _, _ := s.CreateSession(ctx, ana, "x", v6)
	if got := column(t, s.DB, `SELECT ip FROM sessions WHERE id = $1`, id6); got != v6 {
		t.Errorf("ipv6 = %q", got)
	}
}

func TestCreateSession_AnAddressItCannotKeepIsLeftEmptyNotCut(t *testing.T) {
	s, ana, _ := newStore(t)
	for name, ip := range map[string]string{"empty": "", "too long": strings.Repeat("1", MaxIP+1)} {
		id, _, err := s.CreateSession(ctx, ana, "x", ip)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := column(t, s.DB, `SELECT COALESCE(ip, 'NULL') FROM sessions WHERE id = $1`, id); got != "NULL" {
			t.Errorf("%s: ip = %q, want none", name, got)
		}
	}
	long := strings.Repeat("a", 400)
	id, _, _ := s.CreateSession(ctx, ana, long, "")
	if got := column(t, s.DB, `SELECT length(user_agent)::text FROM sessions WHERE id = $1`, id); got != "255" {
		t.Errorf("user agent length = %s, want 255", got)
	}
}

func TestCheckSession_WritesTheLastUseAtMostOnceAMinute(t *testing.T) {
	s, ana, _ := newStore(t)
	id, _, _ := s.CreateSession(ctx, ana, "x", "")
	old := func() { s.DB.Exec(`UPDATE sessions SET last_seen_at = '2000-01-01' WHERE id = $1`, id) }
	year := func() string {
		return column(t, s.DB, `SELECT to_char(last_seen_at, 'YYYY') FROM sessions WHERE id = $1`, id)
	}

	old()
	if _, _, err := s.CheckSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	if year() == "2000" {
		t.Fatal("the first request after a while must note the use")
	}
	old()
	if _, _, err := s.CheckSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	if year() != "2000" {
		t.Error("a second request within a minute must not write")
	}
	// A minute later it writes again.
	s.touched.mu.Lock()
	s.touched.last[id] = time.Now().Add(-SeenEvery - time.Second)
	s.touched.mu.Unlock()
	if _, _, err := s.CheckSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	if year() == "2000" {
		t.Error("after a minute the use must be noted again")
	}
}

func TestCheckSession_AnEndedSessionIsRefusedAndNotTouched(t *testing.T) {
	s, ana, _ := newStore(t)
	id, _, _ := s.CreateSession(ctx, ana, "x", "")
	s.DB.Exec(`UPDATE sessions SET revoked_at = now(), last_seen_at = '2000-01-01' WHERE id = $1`, id)
	if _, _, err := s.CheckSession(ctx, id); err != middleware.ErrInvalidSession {
		t.Fatalf("err = %v", err)
	}
	if got := column(t, s.DB, `SELECT to_char(last_seen_at, 'YYYY') FROM sessions WHERE id = $1`, id); got != "2000" {
		t.Error("an ended session was touched")
	}
}

func TestTouchedSessions_ForgetsWhenItGrowsTooBig(t *testing.T) {
	var tt touchedSessions
	now := time.Now()
	for i := 0; i < 10000; i++ {
		if !tt.due(strings.Repeat("a", 1)+string(rune('A'+i%26))+time.Duration(i).String(), now) {
			t.Fatal("a session never seen is always due")
		}
	}
	if len(tt.last) != 10000 {
		t.Fatalf("size = %d", len(tt.last))
	}
	if !tt.due("another", now) || len(tt.last) != 1 {
		t.Errorf("at the cap the table starts over: size %d", len(tt.last))
	}
}

func TestListLive_OnlyTheAccountsOwnLiveSessionsMostRecentFirst(t *testing.T) {
	s, ana, bia := newStore(t)
	older, _, _ := s.CreateSession(ctx, ana, "Antigo", "10.0.0.1")
	newer, _, _ := s.CreateSession(ctx, ana, "Novo", "10.0.0.2")
	ended, _, _ := s.CreateSession(ctx, ana, "Encerrado", "10.0.0.3")
	expired, _, _ := s.CreateSession(ctx, ana, "Vencido", "10.0.0.4")
	s.CreateSession(ctx, bia, "Da Bia", "10.0.0.5")
	s.DB.Exec(`UPDATE sessions SET last_seen_at = now() - interval '2 days' WHERE id = $1`, older)
	s.DB.Exec(`UPDATE sessions SET revoked_at = now() WHERE id = $1`, ended)
	s.DB.Exec(`UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE id = $1`, expired)

	got, err := s.ListLive(ctx, ana)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != newer || got[1].ID != older {
		t.Fatalf("sessions = %+v", got)
	}
	if got[0].IP != "10.0.0.2" || got[0].UserAgent != "Novo" || got[0].LastSeenAt == nil || got[0].CreatedAt.IsZero() || got[0].ExpiresAt.IsZero() {
		t.Errorf("fields = %+v", got[0])
	}
	if none, _ := s.ListLive(ctx, "00000000-0000-0000-0000-000000000000"); len(none) != 0 || none == nil {
		t.Errorf("an account with none: %v (must be an empty list, not null)", none)
	}
	// One made before the columns existed has neither address nor use.
	s.DB.Exec(`UPDATE sessions SET ip = NULL, last_seen_at = NULL WHERE id = $1`, older)
	got, _ = s.ListLive(ctx, ana)
	for _, x := range got {
		if x.ID == older && (x.IP != "" || x.LastSeenAt != nil) {
			t.Errorf("an old session = %+v", x)
		}
	}
}

func TestRevokeOwned_EndsOnlyTheAccountsOwnLiveSession(t *testing.T) {
	s, ana, bia := newStore(t)
	mine, _, _ := s.CreateSession(ctx, ana, "x", "")
	hers, _, _ := s.CreateSession(ctx, bia, "x", "")

	if ok, err := s.RevokeOwned(ctx, ana, hers); err != nil || ok {
		t.Errorf("someone else's session: %v %v, want false", ok, err)
	}
	if _, _, err := s.CheckSession(ctx, hers); err != nil {
		t.Error("someone else's session was ended")
	}
	if ok, err := s.RevokeOwned(ctx, ana, "nao-e-um-id"); ok || err != nil {
		t.Errorf("a malformed id: %v %v, want a plain \"no\", not an error of the database", ok, err)
	}
	if ok, err := s.RevokeOwned(ctx, ana, mine); err != nil || !ok {
		t.Fatalf("own session: %v %v", ok, err)
	}
	if _, _, err := s.CheckSession(ctx, mine); err != middleware.ErrInvalidSession {
		t.Errorf("an ended session must be refused at once: %v", err)
	}
	if ok, _ := s.RevokeOwned(ctx, ana, mine); ok {
		t.Error("an ended session cannot be ended again")
	}
}

func TestRevokeOthers_KeepsTheOneInUseAndCountsTheRest(t *testing.T) {
	s, ana, bia := newStore(t)
	keep, _, _ := s.CreateSession(ctx, ana, "x", "")
	s.CreateSession(ctx, ana, "y", "")
	s.CreateSession(ctx, ana, "z", "")
	other, _, _ := s.CreateSession(ctx, bia, "w", "")

	n, err := s.RevokeOthers(ctx, ana, keep)
	if err != nil || n != 2 {
		t.Fatalf("revoked %d, %v; want 2", n, err)
	}
	if _, _, err := s.CheckSession(ctx, keep); err != nil {
		t.Error("the session in use was ended")
	}
	if _, _, err := s.CheckSession(ctx, other); err != nil {
		t.Error("another account's session was ended")
	}
	if n, _ := s.RevokeOthers(ctx, ana, keep); n != 0 {
		t.Errorf("again: %d, want 0", n)
	}
}

func TestRevokeAllSessions_EndsEverySessionButNotTheAppTokens(t *testing.T) {
	s, ana, _ := newStore(t)
	a, _, _ := s.CreateSession(ctx, ana, "x", "")
	s.CreateSession(ctx, ana, "y", "")
	_, token, err := s.CreateAppToken(ctx, ana, "KOReader")
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.RevokeAllSessions(ctx, ana)
	if err != nil || n != 2 {
		t.Fatalf("revoked %d, %v; want 2", n, err)
	}
	if _, _, err := s.CheckSession(ctx, a); err != middleware.ErrInvalidSession {
		t.Errorf("session still live: %v", err)
	}
	if _, _, err := s.VerifyAppToken(ctx, "ana", token); err != nil {
		t.Errorf("the app token must stay: %v", err)
	}
}
