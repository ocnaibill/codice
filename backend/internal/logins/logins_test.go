package logins

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

var ctx = context.Background()

func newRecorder(t *testing.T) (*Recorder, string, string) {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var ana, bia string
	db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('ana', 'a@x', 'reader') RETURNING id`).Scan(&ana)
	db.QueryRow(`INSERT INTO users (username, email, role) VALUES ('bia', 'b@x', 'reader') RETURNING id`).Scan(&bia)
	return &Recorder{DB: db}, ana, bia
}

func count(t *testing.T, db *sql.DB, where string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM login_events WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTypedName_KeepsOnlyAShortPlainName(t *testing.T) {
	keep := []string{"ana", "ana.silva", "a_b-c+d@x", "x", "Maria99", strings.Repeat("a", MaxTyped), " ana "}
	for _, in := range keep {
		if got := TypedName(in); got != strings.TrimSpace(in) {
			t.Errorf("TypedName(%q) = %q, want it kept", in, got)
		}
	}
	// What is typed in the wrong field is dropped whole, never cut: the start of a password is still part of one.
	drop := []string{
		"", "   ", "minha senha tem espaços", "uma frase longa com palavras", strings.Repeat("a", MaxTyped+1),
		"S3nh@Forte!2026", "senha#com#hash", "p@ss word", "a b", ".starts-with-dot", "-dash", "tab\tinside", "nova\nlinha", "ação", "senha(1)",
	}
	for _, in := range drop {
		if got := TypedName(in); got != "" {
			t.Errorf("TypedName(%q) = %q, want it dropped", in, got)
		}
	}
}

func TestRecord_ASuccessIsAlwaysItsOwnRow(t *testing.T) {
	r, ana, _ := newRecorder(t)
	for i := 0; i < 3; i++ {
		r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: "10.0.0.1", UserAgent: "Firefox"})
	}
	if n := count(t, r.DB, `result = 'success'`); n != 3 {
		t.Errorf("success rows = %d, want 3", n)
	}
	var cnt int
	r.DB.QueryRow(`SELECT max(count) FROM login_events`).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("a success was counted on another row: %d", cnt)
	}
}

func TestRecord_ARunOfTheSameFailureIsOneRowWithACount(t *testing.T) {
	r, ana, _ := newRecorder(t)
	for i := 0; i < 5; i++ {
		r.Record(ctx, Event{Result: BadPassword, Method: Local, UserID: ana, IP: "203.0.113.7", UserAgent: "curl"})
	}
	if n := count(t, r.DB, `true`); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	var cnt int
	var first, last time.Time
	r.DB.QueryRow(`SELECT count, at, last_at FROM login_events`).Scan(&cnt, &first, &last)
	if cnt != 5 || last.Before(first) {
		t.Errorf("count %d, first %v last %v", cnt, first, last)
	}
}

func TestRecord_FailuresThatDifferAreNotMixed(t *testing.T) {
	r, ana, bia := newRecorder(t)
	base := Event{Result: BadPassword, Method: Local, UserID: ana, IP: "203.0.113.7"}
	r.Record(ctx, base)
	for name, e := range map[string]Event{
		"other address": {Result: BadPassword, Method: Local, UserID: ana, IP: "203.0.113.8"},
		"other account": {Result: BadPassword, Method: Local, UserID: bia, IP: "203.0.113.7"},
		"other result":  {Result: Blocked, Method: Local, UserID: ana, IP: "203.0.113.7"},
		"other way":     {Result: BadPassword, Method: LDAP, UserID: ana, IP: "203.0.113.7"},
		"no address":    {Result: BadPassword, Method: Local, UserID: ana},
	} {
		before := count(t, r.DB, `true`)
		r.Record(ctx, e)
		if after := count(t, r.DB, `true`); after != before+1 {
			t.Errorf("%s: rows %d -> %d, want a new one", name, before, after)
		}
	}
	r.Record(ctx, base)
	if n := count(t, r.DB, `count = 2`); n != 1 {
		t.Errorf("the repeated one was not counted: %d", n)
	}
}

func TestRecord_AfterTheWindowANewRowStarts(t *testing.T) {
	r, ana, _ := newRecorder(t)
	e := Event{Result: BadPassword, Method: Local, UserID: ana, IP: "203.0.113.7"}
	r.Record(ctx, e)
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '11 minutes'`)
	r.Record(ctx, e)
	if n := count(t, r.DB, `true`); n != 2 {
		t.Errorf("rows = %d, want 2: a siege is a few rows with their times, not one that hides when it began", n)
	}
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '9 minutes' WHERE id = (SELECT max(id) FROM login_events)`)
	r.Record(ctx, e)
	if n := count(t, r.DB, `true`); n != 2 {
		t.Errorf("rows = %d, want 2: still inside the window", n)
	}
}

func TestRecord_TheTypedNameIsKeptOnlyForAnUnknownNameAndOnlyIfPlain(t *testing.T) {
	r, ana, _ := newRecorder(t)
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "admin", IP: "1.1.1.1"})
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "minha senha secreta", IP: "1.1.1.2"})
	r.Record(ctx, Event{Result: BadPassword, Method: Local, UserID: ana, Typed: "ana", IP: "1.1.1.3"})
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, Typed: "ana", IP: "1.1.1.4"})
	if n := count(t, r.DB, `typed_name = 'admin' AND result = 'unknown_user'`); n != 1 {
		t.Error("a plain unknown name must be kept")
	}
	if n := count(t, r.DB, `typed_name IS NOT NULL`); n != 1 {
		t.Errorf("rows with a typed name = %d, want only the plain unknown one", n)
	}
	if n := count(t, r.DB, `result = 'unknown_user' AND typed_name IS NULL`); n != 1 {
		t.Error("a name that looks like a password must be dropped, the row still recorded")
	}
}

func TestRecord_UnknownNamesGroupByNameAndAddress(t *testing.T) {
	r, _, _ := newRecorder(t)
	for i := 0; i < 4; i++ {
		r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "root", IP: "203.0.113.7"})
	}
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "admin", IP: "203.0.113.7"})
	if n := count(t, r.DB, `true`); n != 2 {
		t.Errorf("rows = %d, want 2 (root x4, admin)", n)
	}
}

func TestRecord_KeepsTheAddressAndDeviceCutToTheirColumns(t *testing.T) {
	r, ana, _ := newRecorder(t)
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: strings.Repeat("1", 80), UserAgent: strings.Repeat("a", 400)})
	var ipLen, uaLen int
	if err := r.DB.QueryRow(`SELECT length(ip), length(user_agent) FROM login_events`).Scan(&ipLen, &uaLen); err != nil {
		t.Fatal(err)
	}
	if ipLen != 45 || uaLen != 255 {
		t.Errorf("ip %d, user agent %d", ipLen, uaLen)
	}
}

func TestRecord_NilAndEmptyRecordersRecordNothing(t *testing.T) {
	var nobody *Recorder
	nobody.Record(ctx, Event{Result: Success, Method: Local}) // must not panic
	(&Recorder{}).Record(ctx, Event{Result: Success, Method: Local})
}

func TestRecord_ADatabaseThatFailsIsLoggedNotRaised(t *testing.T) {
	r, _, _ := newRecorder(t)
	r.DB.Exec(`DROP TABLE login_events`)
	r.Record(ctx, Event{Result: BadPassword, Method: Local, IP: "1.1.1.1"}) // the table is gone: logged, not raised
}

func TestRecord_ARefusedRequestThatIsAlreadyOverDoesNotStopTheRecord(t *testing.T) {
	r, ana, _ := newRecorder(t)
	over, cancel := context.WithCancel(ctx)
	cancel()
	r.Record(over, Event{Result: BadPassword, Method: Local, UserID: ana, IP: "1.1.1.1"})
	if n := count(t, r.DB, `true`); n != 1 {
		t.Errorf("a record made after the request ended: %d rows, want 1", n)
	}
}

func TestRecord_ADeletedAccountTakesItsRecordWithIt(t *testing.T) {
	r, ana, bia := newRecorder(t)
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: "10.0.0.1"})
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: bia, IP: "10.0.0.2"})
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "ghost", IP: "10.0.0.3"})
	r.DB.Exec(`DELETE FROM users WHERE id = $1`, ana)
	if n := count(t, r.DB, `true`); n != 2 {
		t.Errorf("rows = %d, want 2: the deleted account's record is personal data and goes with it", n)
	}
}

func TestList_NewestFirstWithFiltersAndPages(t *testing.T) {
	r, ana, bia := newRecorder(t)
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: "10.0.0.1"})
	r.Record(ctx, Event{Result: BadPassword, Method: Local, UserID: bia, IP: "10.0.0.2"})
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "root", IP: "10.0.0.3"})
	r.Record(ctx, Event{Result: Success, Method: LDAP, UserID: bia, IP: "10.0.0.4"})

	all, more, err := r.List(ctx, Query{ShowTyped: true})
	if err != nil || more || len(all) != 4 {
		t.Fatalf("all = %d more %v err %v", len(all), more, err)
	}
	if all[0].IP != "10.0.0.4" || all[3].IP != "10.0.0.1" {
		t.Errorf("order = %s ... %s, want newest first", all[0].IP, all[3].IP)
	}
	if all[0].Username != "bia" || all[0].Method != LDAP || all[1].Typed != "root" || all[1].Username != "" {
		t.Errorf("fields = %+v", all)
	}

	failed, _, _ := r.List(ctx, Query{Result: BadPassword})
	if len(failed) != 1 || failed[0].Username != "bia" {
		t.Errorf("by result = %+v", failed)
	}
	byUser, _, _ := r.List(ctx, Query{Username: "bia"})
	if len(byUser) != 2 {
		t.Errorf("by account = %+v", byUser)
	}
	if none, _, _ := r.List(ctx, Query{Username: "ninguem"}); len(none) != 0 {
		t.Errorf("by an account that does not exist = %+v", none)
	}

	page, more, _ := r.List(ctx, Query{Limit: 3})
	if len(page) != 3 || !more {
		t.Fatalf("page 1 = %d more %v", len(page), more)
	}
	next, more, _ := r.List(ctx, Query{Limit: 3, Before: page[2].ID})
	if len(next) != 1 || more || next[0].IP != "10.0.0.1" {
		t.Errorf("page 2 = %+v more %v", next, more)
	}
	if huge, _, _ := r.List(ctx, Query{Limit: 100000}); len(huge) != 4 {
		t.Errorf("a huge limit is capped, not refused: %d", len(huge))
	}
}

func TestList_TheNamesTypedAreForTheOwnerOnly(t *testing.T) {
	r, _, _ := newRecorder(t)
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "root", IP: "10.0.0.3"})
	staff, _, _ := r.List(ctx, Query{})
	if len(staff) != 1 || staff[0].Typed != "" || staff[0].Result != UnknownUser {
		t.Errorf("staff = %+v", staff)
	}
	owner, _, _ := r.List(ctx, Query{ShowTyped: true})
	if owner[0].Typed != "root" {
		t.Errorf("owner = %+v", owner)
	}
}

func TestList_ByPeriod(t *testing.T) {
	r, ana, _ := newRecorder(t)
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: ip})
	}
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '10 days' WHERE ip = '10.0.0.1'`)
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '3 days' WHERE ip = '10.0.0.2'`)
	week, _, _ := r.List(ctx, Query{From: time.Now().Add(-7 * 24 * time.Hour)})
	if len(week) != 2 {
		t.Errorf("last week = %d, want 2", len(week))
	}
	old, _, _ := r.List(ctx, Query{To: time.Now().Add(-5 * 24 * time.Hour)})
	if len(old) != 1 || old[0].IP != "10.0.0.1" {
		t.Errorf("before 5 days ago = %+v", old)
	}
	mid, _, _ := r.List(ctx, Query{From: time.Now().Add(-5 * 24 * time.Hour), To: time.Now().Add(-1 * 24 * time.Hour)})
	if len(mid) != 1 || mid[0].IP != "10.0.0.2" {
		t.Errorf("between = %+v", mid)
	}
}

func TestOwn_OnlyTheAccountsAndNeverTheNameTyped(t *testing.T) {
	r, ana, bia := newRecorder(t)
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: "10.0.0.1", UserAgent: "Firefox"})
	r.Record(ctx, Event{Result: BadPassword, Method: Local, UserID: ana, IP: "198.51.100.5"})
	r.Record(ctx, Event{Result: Success, Method: Local, UserID: bia, IP: "10.0.0.9"})
	r.Record(ctx, Event{Result: UnknownUser, Method: Local, Typed: "ana", IP: "198.51.100.6"})

	mine, err := r.Own(ctx, ana, 20)
	if err != nil || len(mine) != 2 {
		t.Fatalf("own = %+v %v", mine, err)
	}
	if mine[0].Result != BadPassword || mine[0].IP != "198.51.100.5" || mine[1].UserAgent != "Firefox" {
		t.Errorf("own = %+v", mine)
	}
	for _, e := range mine {
		if e.Typed != "" || e.Username != "" {
			t.Errorf("own must carry neither the name typed nor a name: %+v", e)
		}
	}
	if one, _ := r.Own(ctx, ana, 1); len(one) != 1 {
		t.Errorf("limit: %+v", one)
	}
	if none, err := r.Own(ctx, "00000000-0000-0000-0000-000000000000", 20); err != nil || none == nil || len(none) != 0 {
		t.Errorf("an account with none must be an empty list: %v %v", none, err)
	}
}

func TestRetention_DefaultsToNinetyDaysAndTakesWhatTheOwnerSetsWithinLimits(t *testing.T) {
	r, _, _ := newRecorder(t)
	if d, err := Retention(ctx, r.DB); err != nil || d != 90 {
		t.Fatalf("default = %d %v", d, err)
	}
	if err := SetRetention(ctx, r.DB, 30); err != nil {
		t.Fatal(err)
	}
	if d, _ := Retention(ctx, r.DB); d != 30 {
		t.Errorf("after setting 30: %d", d)
	}
	// A value outside the limits (written by hand in the database) is not obeyed.
	for _, bad := range []int{0, 6, MaxRetentionDays + 1, -5} {
		SetRetention(ctx, r.DB, bad)
		if d, _ := Retention(ctx, r.DB); d != DefaultRetentionDays {
			t.Errorf("stored %d: retention = %d, want the default", bad, d)
		}
	}
	for _, edge := range []int{MinRetentionDays, MaxRetentionDays} {
		SetRetention(ctx, r.DB, edge)
		if d, _ := Retention(ctx, r.DB); d != edge {
			t.Errorf("the limit %d itself must be accepted: %d", edge, d)
		}
	}
}

func TestPurge_RemovesWhatIsPastTheRetention(t *testing.T) {
	r, ana, _ := newRecorder(t)
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: ip})
	}
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '91 days' WHERE ip = '10.0.0.1'`)
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '89 days' WHERE ip = '10.0.0.2'`)
	n, err := r.Purge(ctx)
	if err != nil || n != 1 {
		t.Fatalf("purged %d, %v; want 1", n, err)
	}
	if left := count(t, r.DB, `true`); left != 2 {
		t.Errorf("left = %d", left)
	}
	// With a shorter retention the same record loses more.
	SetRetention(ctx, r.DB, 7)
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '8 days' WHERE ip = '10.0.0.2'`)
	if n, _ := r.Purge(ctx); n != 1 {
		t.Errorf("purged %d with 7 days, want 1", n)
	}
	// A siege that is still going is kept even if it began long ago: age is by the LAST time.
	r.Record(ctx, Event{Result: BadPassword, Method: Local, IP: "203.0.113.7"})
	r.DB.Exec(`UPDATE login_events SET at = now() - interval '30 days' WHERE ip = '203.0.113.7'`)
	if n, _ := r.Purge(ctx); n != 0 {
		t.Errorf("a row whose last time is today was purged: %d", n)
	}
}

func TestPurge_CutsTheTableToItsSizeKeepingTheNewest(t *testing.T) {
	r, _, _ := newRecorder(t)
	if _, err := r.DB.Exec(`INSERT INTO login_events (result, method, ip) SELECT 'bad_password', 'local', '10.1.' || (g / 250) || '.' || (g % 250) FROM generate_series(1, $1) g`, MaxRows+25); err != nil {
		t.Fatal(err)
	}
	n, err := r.Purge(ctx)
	if err != nil || n != 25 {
		t.Fatalf("purged %d, %v; want 25", n, err)
	}
	if left := count(t, r.DB, `true`); left != MaxRows {
		t.Errorf("left = %d, want %d", left, MaxRows)
	}
	var minID, maxID int64
	r.DB.QueryRow(`SELECT min(id), max(id) FROM login_events`).Scan(&minID, &maxID)
	if maxID-minID+1 != MaxRows {
		t.Errorf("the ones kept must be the newest: ids %d..%d", minID, maxID)
	}
}

func TestSameAddress_WarnsOfAProxyThatHidesTheClients(t *testing.T) {
	r, ana, _ := newRecorder(t)
	add := func(n int, ip string) {
		for i := 0; i < n; i++ {
			r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: ip})
		}
	}
	if _, warn, _ := SameAddress(ctx, r.DB); warn {
		t.Error("an empty record must not warn")
	}
	add(4, "172.30.77.1")
	if _, warn, _ := SameAddress(ctx, r.DB); warn {
		t.Error("4 entries are too few to say")
	}
	add(1, "172.30.77.1")
	ip, warn, err := SameAddress(ctx, r.DB)
	if err != nil || !warn || ip != "172.30.77.1" {
		t.Errorf("5 entries, all from the Docker gateway: %q %v %v", ip, warn, err)
	}
	add(1, "172.30.77.10")
	if _, warn, _ := SameAddress(ctx, r.DB); warn {
		t.Error("two addresses: the clients are told apart, no warning")
	}
}

func TestSameAddress_OnlyWhenTheAddressIsPrivateAndRecent(t *testing.T) {
	r, ana, _ := newRecorder(t)
	for i := 0; i < 8; i++ {
		r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: "203.0.113.7"})
	}
	if _, warn, _ := SameAddress(ctx, r.DB); warn {
		t.Error("one public address is a household, not a proxy")
	}
	r.DB.Exec(`DELETE FROM login_events`)
	for _, ip := range []string{"192.168.1.30", "10.0.0.5", "127.0.0.1", "169.254.1.1"} {
		r.DB.Exec(`DELETE FROM login_events`)
		for i := 0; i < 6; i++ {
			r.Record(ctx, Event{Result: Success, Method: Local, UserID: ana, IP: ip})
		}
		if _, warn, _ := SameAddress(ctx, r.DB); !warn {
			t.Errorf("%s is a private address and must warn", ip)
		}
	}
	r.DB.Exec(`UPDATE login_events SET last_at = now() - interval '8 days'`)
	if _, warn, _ := SameAddress(ctx, r.DB); warn {
		t.Error("entries older than a week must not count")
	}
	// A run of failures counted on one row counts as many as it says.
	r.DB.Exec(`DELETE FROM login_events`)
	for i := 0; i < 5; i++ {
		r.Record(ctx, Event{Result: BadPassword, Method: Local, IP: "172.30.77.1"})
	}
	if _, warn, _ := SameAddress(ctx, r.DB); !warn {
		t.Error("5 failures grouped on one row are still 5 entries")
	}
	// The limit being hit says nothing about who is behind it.
	r.DB.Exec(`DELETE FROM login_events`)
	for i := 0; i < 6; i++ {
		r.Record(ctx, Event{Result: RateLimited, Method: Local, IP: "172.30.77.1"})
	}
	if _, warn, _ := SameAddress(ctx, r.DB); warn {
		t.Error("rate-limited rows alone must not warn")
	}
}
