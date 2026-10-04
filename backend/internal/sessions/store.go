// Package sessions keeps login sessions and app tokens in PostgreSQL so access
// can be revoked immediately (DEC-060, DEC-070, DEC-071). A JWT alone proves
// nothing here: every request is checked against these rows.
package sessions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ocnaibill/codice/backend/internal/middleware"
)

const (
	defaultSessionTTL = 7 * 24 * time.Hour
	maxSessionHours   = 24 * 365 // longer than a year is treated as a mistake
)

// TTL is how long a login lasts unless revoked earlier: JWT_EXPIRATION_HOURS
// when it is a sensible whole number of hours, otherwise 7 days.
func TTL() time.Duration {
	h, err := strconv.Atoi(os.Getenv("JWT_EXPIRATION_HOURS"))
	if err != nil || h <= 0 || h > maxSessionHours {
		return defaultSessionTTL
	}
	return time.Duration(h) * time.Hour
}

// AppTokenPrefix marks app tokens so they are recognisable in configs and
// secret scanners.
const AppTokenPrefix = "cdc_"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Store struct {
	DB *sql.DB

	touched touchedSessions
}

// SeenEvery is how often the "last used" of a session is written at most. A session is checked on every
// request, and a write for each of them would cost the database more than the information is worth.
const SeenEvery = time.Minute

// touchedSessions remembers when each session's last use was written, so most requests write nothing.
type touchedSessions struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// due says whether the last use of the session should be written now, and notes that it is.
func (t *touchedSessions) due(id string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if at, ok := t.last[id]; ok && now.Sub(at) < SeenEvery {
		return false
	}
	// Only live sessions are in here; the table is thrown away when it grows, which only costs one more write each.
	if t.last == nil || len(t.last) >= 10000 {
		t.last = make(map[string]time.Time)
	}
	t.last[id] = now
	return true
}

// MaxIP is the longest address kept (an IPv6 address written out is 45 characters).
const MaxIP = 45

// CreateSession records a new login for the user, with the device it came from and the address it came
// from (what the server believes it to be: see middleware.RequestClientIP).
func (s *Store) CreateSession(ctx context.Context, userID, userAgent, ip string) (id string, expires time.Time, err error) {
	if len(userAgent) > 255 {
		userAgent = userAgent[:255]
	}
	if len(ip) > MaxIP {
		ip = ""
	}
	expires = time.Now().Add(TTL())
	err = s.DB.QueryRowContext(ctx,
		`INSERT INTO sessions (user_id, expires_at, user_agent, ip, last_seen_at) VALUES ($1, $2, $3, NULLIF($4, ''), now()) RETURNING id`,
		userID, expires, userAgent, ip).Scan(&id)
	return id, expires, err
}

// CheckSession returns the account behind a session if the session is live:
// not revoked, not expired, and its account not blocked. The role is read from
// the users table, so a promotion or demotion takes effect on the next request.
// It matches middleware.SessionChecker.
func (s *Store) CheckSession(ctx context.Context, sessionID string) (userID, role string, err error) {
	if !uuidPattern.MatchString(sessionID) {
		return "", "", middleware.ErrInvalidSession
	}
	err = s.DB.QueryRowContext(ctx, `
		SELECT u.id, u.role
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND u.blocked_at IS NULL`,
		sessionID).Scan(&userID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", middleware.ErrInvalidSession
	}
	if err == nil && s.touched.due(sessionID, time.Now()) {
		// Best effort: a request is never refused because the note of its last use could not be written.
		s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, sessionID)
	}
	return userID, role, err
}

// Session is one live login, as its owner sees it.
type Session struct {
	ID         string     `json:"id"`
	UserAgent  string     `json:"userAgent"`
	IP         string     `json:"ip,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
}

// ListLive returns the sessions of an account that are still good (not ended, not expired), the most
// recently used first.
func (s *Store) ListLive(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, COALESCE(user_agent, ''), COALESCE(ip, ''), created_at, last_seen_at, expires_at
		FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY COALESCE(last_seen_at, created_at) DESC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		var seen sql.NullTime
		if err := rows.Scan(&x.ID, &x.UserAgent, &x.IP, &x.CreatedAt, &seen, &x.ExpiresAt); err != nil {
			return nil, err
		}
		if seen.Valid {
			x.LastSeenAt = &seen.Time
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// RevokeOwned ends one session of an account. It says whether there was such a live session: an id that
// is another account's, or already ended, is "no".
func (s *Store) RevokeOwned(ctx context.Context, userID, sessionID string) (bool, error) {
	if !uuidPattern.MatchString(sessionID) {
		return false, nil
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > now()`,
		sessionID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RevokeAllSessions ends every live session of an account (its app tokens are not touched), and says how many.
func (s *Store) RevokeAllSessions(ctx context.Context, userID string) (int, error) {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()`, userID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// RevokeOthers ends every live session of an account but the one it is called from, and says how many.
func (s *Store) RevokeOthers(ctx context.Context, userID, keepSessionID string) (int, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND id <> $2::uuid AND revoked_at IS NULL AND expires_at > now()`,
		userID, keepSessionID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// RevokeSession ends one session (logout).
func (s *Store) RevokeSession(ctx context.Context, sessionID string) error {
	if !uuidPattern.MatchString(sessionID) {
		return nil
	}
	_, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	return err
}

// RevokeAllForUser ends every session and app token of an account. Blocking an
// account and resetting its password must call this (DEC-060).
func (s *Store) RevokeAllForUser(ctx context.Context, userID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := RevokeAllForUserTx(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// RevokeAllForUserTx is RevokeAllForUser inside a transaction the caller owns, so
// blocking an account and ending its access commit together or not at all.
func RevokeAllForUserTx(ctx context.Context, tx *sql.Tx, userID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE app_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

// AppToken is the public view of an app token; the secret is never stored.
type AppToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateAppToken issues a token for an OPDS or sync client. The returned secret
// is shown once; only its SHA-256 is kept.
func (s *Store) CreateAppToken(ctx context.Context, userID, name string) (id, token string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	token = AppTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	err = s.DB.QueryRowContext(ctx,
		`INSERT INTO app_tokens (user_id, name, token_hash) VALUES ($1, $2, $3) RETURNING id`,
		userID, name, hashToken(token)).Scan(&id)
	return id, token, err
}

// VerifyAppToken accepts an app token presented as the Basic password together
// with its owner's username. The account password is deliberately not accepted
// (DEC-071). It matches middleware.BasicVerifier.
func (s *Store) VerifyAppToken(ctx context.Context, username, token string) (id, role string, err error) {
	if !strings.HasPrefix(token, AppTokenPrefix) {
		return "", "", middleware.ErrInvalidCredentials
	}
	err = s.DB.QueryRowContext(ctx, `
		UPDATE app_tokens t SET last_used_at = now()
		FROM users u
		WHERE t.token_hash = $1 AND u.id = t.user_id AND u.username = $2
		  AND t.revoked_at IS NULL AND u.blocked_at IS NULL
		RETURNING u.id, u.role`, hashToken(token), username).Scan(&id, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", middleware.ErrInvalidCredentials
	}
	return id, role, err
}

// ListAppTokens returns the user's active tokens.
func (s *Store) ListAppTokens(ctx context.Context, userID string) ([]AppToken, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, created_at, last_used_at FROM app_tokens
		WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := []AppToken{}
	for rows.Next() {
		var t AppToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// RevokeAppToken revokes one of the user's own tokens and reports whether it
// existed. Other users' tokens are never affected.
func (s *Store) RevokeAppToken(ctx context.Context, userID, tokenID string) (bool, error) {
	if !uuidPattern.MatchString(tokenID) {
		return false, nil
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE app_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		tokenID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
