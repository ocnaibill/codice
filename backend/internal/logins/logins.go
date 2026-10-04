// Package logins keeps the record of sign-ins (DEC-121, issue #137): who tried, from where, by which way and how it
// went, the failures included. It is a log for the owner and the administrators, and for each person about their own
// account. It never gets in the way of signing in: recording is best effort, and a failure to record is only logged.
package logins

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// What happened.
const (
	Success              = "success"
	BadPassword          = "bad_password"
	UnknownUser          = "unknown_user"
	Blocked              = "blocked"
	DirectoryUnavailable = "directory_unavailable"
	RateLimited          = "rate_limited"
	LinkOffered          = "link_offered"
	BadAppToken          = "bad_app_token"
)

// The way in.
const (
	Local  = "local"
	LDAP   = "ldap"
	Invite = "invite"
	Setup  = "setup"
	App    = "app"
)

const (
	// GroupWindow is how long a failure keeps being counted on the same row: past it, a new row starts, so a
	// long siege is a few rows with their times, not one that hides when it began and ended.
	GroupWindow = 10 * time.Minute
	// MaxRows is the most the table is allowed to hold: past it the oldest go, whatever their age.
	MaxRows = 50000
	// DefaultRetentionDays is how long a row is kept unless the owner says otherwise.
	DefaultRetentionDays = 90
	MinRetentionDays     = 7
	MaxRetentionDays     = 3650
	settingsKey          = "login_events_retention_days"
)

// Event is one thing that happened at the door.
type Event struct {
	Result    string
	Method    string
	UserID    string // the account, when there is one
	Typed     string // the name typed, for a name with no account (it goes through TypedName)
	IP        string
	UserAgent string
}

// Recorder writes the record. The zero value (nil) records nothing, so a part that has none (a test) needs no check.
type Recorder struct {
	DB *sql.DB
}

var plainName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]*$`)

// MaxTyped is the longest name kept (the column holds 24).
const MaxTyped = 24

// TypedName is what is kept of a name that was typed and has no account, or "" when nothing should be: a field for
// a name is where a password lands when someone types in the wrong one. Only a short, plain name is kept (letters,
// digits and . _ @ + -, no spaces); a name with spaces or other symbols, or longer than a name is, is dropped whole,
// never cut, because the beginning of a password is still a piece of a password.
func TypedName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > MaxTyped || !plainName.MatchString(name) {
		return ""
	}
	return name
}

func cut(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Record writes the event. A failure that repeats (same address, same account or name, same way, same result) within
// GroupWindow is counted on its row instead of making another; a success is always its own row.
func (r *Recorder) Record(ctx context.Context, e Event) {
	if r == nil || r.DB == nil {
		return
	}
	// The request may already be over by the time this runs (a refused login answers first): the record must not
	// die with it, but must not hang either.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := r.record(ctx, e); err != nil {
		log.Printf("logins: could not record %s/%s: %v", e.Method, e.Result, err)
	}
}

func (r *Recorder) record(ctx context.Context, e Event) error {
	typed := ""
	if e.Result == UnknownUser {
		typed = TypedName(e.Typed)
	}
	ip, ua := cut(e.IP, 45), cut(e.UserAgent, 255)
	if e.Result != Success {
		res, err := r.DB.ExecContext(ctx, `
			UPDATE login_events SET count = count + 1, last_at = now()
			WHERE id = (
				SELECT id FROM login_events
				WHERE result = $1 AND method = $2 AND ip IS NOT DISTINCT FROM NULLIF($3, '')
					AND user_id IS NOT DISTINCT FROM NULLIF($4, '')::uuid AND typed_name IS NOT DISTINCT FROM NULLIF($5, '')
					AND result <> 'success' AND last_at > now() - $6::interval
				ORDER BY last_at DESC LIMIT 1 FOR UPDATE SKIP LOCKED)`,
			e.Result, e.Method, ip, e.UserID, typed, fmt.Sprintf("%d seconds", int(GroupWindow.Seconds())))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
	}
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO login_events (result, method, user_id, typed_name, ip, user_agent)
		VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''))`,
		e.Result, e.Method, e.UserID, typed, ip, ua)
	return err
}

// Entry is a row as it is read.
type Entry struct {
	ID        int64     `json:"id"`
	At        time.Time `json:"at"`
	LastAt    time.Time `json:"lastAt"`
	Count     int       `json:"count"`
	Result    string    `json:"result"`
	Method    string    `json:"method"`
	Username  string    `json:"username,omitempty"` // the account's, when there is one
	Typed     string    `json:"typed,omitempty"`    // the name typed, when it has no account: the owner's to see
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
}

// Query narrows the list. The zero value is everything, newest first.
type Query struct {
	Result   string
	Username string // the account's name, exactly
	From, To time.Time
	Before   int64 // only rows with a smaller id: the next page
	Limit    int
	// ShowTyped says whether the names typed for accounts that do not exist are given: only to the owner.
	ShowTyped bool
}

const maxPage = 200

// List returns a page of the record, newest first, and whether there is more after it.
func (r *Recorder) List(ctx context.Context, q Query) ([]Entry, bool, error) {
	limit := q.Limit
	if limit <= 0 || limit > maxPage {
		limit = 50
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT e.id, e.at, e.last_at, e.count, e.result, e.method, COALESCE(u.username, ''), COALESCE(e.typed_name, ''),
		       COALESCE(e.ip, ''), COALESCE(e.user_agent, '')
		FROM login_events e LEFT JOIN users u ON u.id = e.user_id
		WHERE ($1 = '' OR e.result = $1)
		  AND ($2 = '' OR u.username = $2)
		  AND ($3::timestamptz IS NULL OR e.last_at >= $3)
		  AND ($4::timestamptz IS NULL OR e.last_at < $4)
		  AND ($5::bigint = 0 OR e.id < $5)
		ORDER BY e.id DESC LIMIT $6`,
		q.Result, q.Username, nullTime(q.From), nullTime(q.To), q.Before, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var x Entry
		if err := rows.Scan(&x.ID, &x.At, &x.LastAt, &x.Count, &x.Result, &x.Method, &x.Username, &x.Typed, &x.IP, &x.UserAgent); err != nil {
			return nil, false, err
		}
		if !q.ShowTyped {
			x.Typed = ""
		}
		out = append(out, x)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, rows.Err()
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// Own returns the latest rows about one account, for the person to read: what happened at the door of THEIR account,
// the failed attempts included. The address is in it (it is theirs to know); the name typed never is.
func (r *Recorder) Own(ctx context.Context, userID string, limit int) ([]Entry, error) {
	if limit <= 0 || limit > maxPage {
		limit = 20
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, at, last_at, count, result, method, COALESCE(ip, ''), COALESCE(user_agent, '')
		FROM login_events WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var x Entry
		if err := rows.Scan(&x.ID, &x.At, &x.LastAt, &x.Count, &x.Result, &x.Method, &x.IP, &x.UserAgent); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Retention is how many days a row is kept.
func Retention(ctx context.Context, db *sql.DB) (int, error) {
	var days int
	err := db.QueryRowContext(ctx, `SELECT value::text::int FROM settings WHERE key = $1`, settingsKey).Scan(&days)
	if err == sql.ErrNoRows || days < MinRetentionDays || days > MaxRetentionDays {
		return DefaultRetentionDays, nil
	}
	return days, err
}

// SetRetention sets how many days a row is kept, within the limits.
func SetRetention(ctx context.Context, db *sql.DB, days int) error {
	_, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, to_jsonb($2::int))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, settingsKey, days)
	return err
}

// Purge removes what is past the retention and, past MaxRows, the oldest. It says how many went.
func (r *Recorder) Purge(ctx context.Context) (int, error) {
	days, err := Retention(ctx, r.DB)
	if err != nil {
		return 0, err
	}
	res, err := r.DB.ExecContext(ctx, `DELETE FROM login_events WHERE last_at < now() - make_interval(days => $1)`, days)
	if err != nil {
		return 0, err
	}
	aged, _ := res.RowsAffected()
	res, err = r.DB.ExecContext(ctx, `
		DELETE FROM login_events WHERE id IN (SELECT id FROM login_events ORDER BY id DESC OFFSET $1)`, MaxRows)
	if err != nil {
		return int(aged), err
	}
	over, _ := res.RowsAffected()
	return int(aged + over), nil
}

// SameAddress says whether the recent record looks like a server that sees every client as the same address: at
// least MinForWarning entries in the last 7 days, all from one address, and that address a private one (a proxy's,
// the Docker gateway's). It is the sign of CODICE_TRUSTED_PROXIES missing behind a proxy, where the limit on
// sign-ins is shared by everyone. A household with one public address is not it, and gets no warning.
func SameAddress(ctx context.Context, db *sql.DB) (address string, warn bool, err error) {
	const MinForWarning = 5
	rows, err := db.QueryContext(ctx, `
		SELECT ip, SUM(count)::int FROM login_events
		WHERE last_at > now() - interval '7 days' AND ip IS NOT NULL AND result <> 'rate_limited'
		GROUP BY ip`)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	total, distinct := 0, 0
	for rows.Next() {
		var ip string
		var n int
		if err := rows.Scan(&ip, &n); err != nil {
			return "", false, err
		}
		address, total, distinct = ip, total+n, distinct+1
	}
	if err := rows.Err(); err != nil || distinct != 1 || total < MinForWarning {
		return "", false, err
	}
	a, perr := netip.ParseAddr(address)
	if perr != nil {
		return "", false, nil
	}
	if !(a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast()) {
		return "", false, nil
	}
	return address, true, nil
}
