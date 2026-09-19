// Package identity ties accounts to identities in an external directory
// (DEC-051, DEC-053, DEC-061, DEC-072 to DEC-075). An identity is the pair
// provider and stable identifier, never a user name: renaming someone in the
// directory must not create another account or lose the old one.
package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
	"github.com/ocnaibill/codice/backend/internal/secrets"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"golang.org/x/crypto/bcrypt"
)

const (
	// TicketTTL is how long a person has to give the local password after proving
	// the directory one.
	TicketTTL = 5 * time.Minute
	// MaxTicketAttempts is how many wrong local passwords burn a ticket.
	MaxTicketAttempts = 5
)

var (
	ErrExists        = errors.New("that identity is already linked")
	ErrNameTaken     = errors.New("that username is already taken")
	ErrNotEligible   = errors.New("that account cannot be linked to a directory identity")
	ErrTicketInvalid = errors.New("the ticket is not valid")
	ErrBadPassword   = errors.New("the local password is not correct")
)

// Policy is what the owner decides about the directory (RF-047, DEC-061). The
// connection itself, with its secret, is deployment configuration instead.
type Policy struct {
	// AllowCreate lets a valid directory login with no account create a reader
	// account at the first sign-in. Off until the owner turns it on.
	AllowCreate bool `json:"allowCreate"`
	// RevalidateHours is the ceiling on how long a directory account stays signed in
	// without the directory confirming it (24 by default).
	RevalidateHours int `json:"revalidateHours"`
}

const policyKey = "ldap_policy"

var DefaultPolicy = Policy{AllowCreate: false, RevalidateHours: 24}

func GetPolicy(ctx context.Context, db *sql.DB) (Policy, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, policyKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPolicy, nil
	}
	if err != nil {
		return DefaultPolicy, err
	}
	p := DefaultPolicy
	if json.Unmarshal(raw, &p) != nil || p.RevalidateHours < 1 {
		return DefaultPolicy, nil
	}
	return p, nil
}

func SetPolicy(ctx context.Context, db *sql.DB, p Policy, actor string) error {
	if p.RevalidateHours < 1 || p.RevalidateHours > 24*14 {
		return errors.New("revalidateHours must be between 1 and 336")
	}
	body, _ := json.Marshal(p)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_by) VALUES ($1, $2, NULLIF($3, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		policyKey, body, actor); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, actor, "ldap.policy", "settings", policyKey,
		map[string]any{"allowCreate": p.AllowCreate, "revalidateHours": p.RevalidateHours}); err != nil {
		return err
	}
	return tx.Commit()
}

// Linked is an account tied to a directory identity.
type Linked struct {
	UserID   string
	Username string
	Role     string
	Blocked  bool
}

// FindLinked returns the account tied to a stable identifier, if any.
func FindLinked(ctx context.Context, db *sql.DB, subject string) (*Linked, error) {
	var l Linked
	err := db.QueryRowContext(ctx, `
		SELECT u.id, u.username, COALESCE(u.role, 'reader'), u.blocked_at IS NOT NULL
		FROM external_identities e JOIN users u ON u.id = e.user_id
		WHERE e.provider = $1 AND e.subject = $2`, ldapauth.Provider, subject).Scan(&l.UserID, &l.Username, &l.Role, &l.Blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &l, err
}

// SubjectOf returns the stable identifier an account is linked to, if it is linked.
func SubjectOf(ctx context.Context, db *sql.DB, userID string) (string, bool, error) {
	var subject string
	err := db.QueryRowContext(ctx, `SELECT subject FROM external_identities WHERE user_id = $1 AND provider = $2`,
		userID, ldapauth.Provider).Scan(&subject)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return subject, err == nil, err
}

// Touch records that the directory just confirmed this identity.
func Touch(ctx context.Context, db *sql.DB, subject string) {
	if _, err := db.ExecContext(ctx, `UPDATE external_identities SET last_verified_at = now() WHERE provider = $1 AND subject = $2`,
		ldapauth.Provider, subject); err != nil {
		log.Println("identity: could not record the verification:", err)
	}
}

// LocalCandidateByEmail finds a local account that could be the same person as a
// directory entry with this e-mail: active, with a local password, not the owner
// and not already linked. A matching e-mail is never proof, only a reason to ask
// the person for the local password (DEC-053).
func LocalCandidateByEmail(ctx context.Context, db *sql.DB, email string) (userID string, ok bool, err error) {
	if email == "" {
		return "", false, nil
	}
	err = db.QueryRowContext(ctx, `
		SELECT u.id FROM users u
		WHERE lower(u.email) = $1 AND COALESCE(u.role, 'reader') <> 'owner' AND u.blocked_at IS NULL
		  AND COALESCE(u.password_hash, '') <> ''
		  AND NOT EXISTS (SELECT 1 FROM external_identities e WHERE e.user_id = u.id)`, strings.ToLower(email)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return userID, err == nil, err
}

// CreateAccount makes a reader account for a directory entry the first time it
// signs in (DEC-051, RF-047), together with its link, in one transaction. Nobody
// becomes admin or owner this way (DEC-052).
func CreateAccount(ctx context.Context, db *sql.DB, username string, entry *ldapauth.Entry) (string, error) {
	email := entry.Email
	if email == "" {
		email = username + "@codice.local"
	}
	create := func(email string) (string, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return "", err
		}
		defer tx.Rollback()
		var id string
		if err := tx.QueryRowContext(ctx, `INSERT INTO users (username, email, role) VALUES ($1, $2, 'reader') RETURNING id`,
			username, email).Scan(&id); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO external_identities (user_id, provider, subject, last_verified_at)
			VALUES ($1, $2, $3, now())`, id, ldapauth.Provider, entry.Subject); err != nil {
			return "", err
		}
		if err := audit.Record(ctx, tx, id, "identity.create", "user", id,
			map[string]any{"provider": ldapauth.Provider, "username": username}); err != nil {
			return "", err
		}
		return id, tx.Commit()
	}
	id, err := create(email)
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		switch {
		case strings.Contains(pe.Constraint, "users_email"):
			// Someone already uses that address (an account that cannot be linked,
			// such as the owner's): the person still gets an account, with a stand-in.
			return create(username + "@codice.local")
		case strings.Contains(pe.Constraint, "username"):
			return "", ErrNameTaken
		default:
			return "", ErrExists
		}
	}
	return id, err
}

// NewTicket starts the assisted link for an existing local account: the person
// has proven the directory password and must now prove the local one.
func NewTicket(ctx context.Context, db *sql.DB, userID, subject string) (string, error) {
	token, err := secrets.NewToken()
	if err != nil {
		return "", err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO link_tickets (token_hash, user_id, provider, subject, expires_at)
		VALUES ($1, $2, $3, $4, now() + $5::interval)`, secrets.Hash(token), userID, ldapauth.Provider, subject, TicketTTL.String())
	return token, err
}

// LinkWithTicket completes the assisted link: it checks the local password and,
// if it is right, ties the directory identity to the account. A wrong password
// counts against the ticket; after MaxTicketAttempts it is burnt.
func LinkWithTicket(ctx context.Context, db *sql.DB, token, password string) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var id int64
	var userID, subject string
	var attempts int
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, subject, attempts FROM link_tickets
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`, secrets.Hash(token)).Scan(&id, &userID, &subject, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		burn(password)
		return "", ErrTicketInvalid
	}
	if err != nil {
		return "", err
	}
	var hash, role string
	var blocked bool
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(password_hash, ''), COALESCE(role, 'reader'), blocked_at IS NOT NULL
		FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&hash, &role, &blocked); err != nil {
		return "", err
	}
	if blocked || role == authz.RoleOwner || hash == "" {
		return "", ErrNotEligible
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		// The attempt must be counted even though the request fails.
		burnedNow := attempts+1 >= MaxTicketAttempts
		if _, err := tx.ExecContext(ctx, `UPDATE link_tickets SET attempts = attempts + 1,
			used_at = CASE WHEN $2 THEN now() ELSE used_at END WHERE id = $1`, id, burnedNow); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return "", ErrBadPassword
	}
	res, err := tx.ExecContext(ctx, `UPDATE link_tickets SET used_at = now() WHERE id = $1 AND used_at IS NULL`, id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", ErrTicketInvalid
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO external_identities (user_id, provider, subject, last_verified_at)
		VALUES ($1, $2, $3, now())`, userID, ldapauth.Provider, subject); err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) {
			switch pe.Code {
			case "23505":
				return "", ErrExists
			case "23514": // the database refuses an owner with an external identity
				return "", ErrNotEligible
			}
		}
		return "", err
	}
	if err := audit.Record(ctx, tx, userID, "identity.link", "user", userID, map[string]any{"provider": ldapauth.Provider}); err != nil {
		return "", err
	}
	return userID, tx.Commit()
}

var dummyHash []byte

// burn spends the time of a password check, so a ticket that does not exist
// answers no faster than one that does.
func burn(password string) {
	if dummyHash == nil {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("codice-dummy-password"), 12)
	}
	bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// SweepResult says what a revalidation pass did.
type SweepResult struct{ Checked, Confirmed, Revoked int }

// Sweep revalidates the directory identities that have a live session (DEC-061).
// A person the directory confirms keeps their session. One it says no longer
// exists loses access now. When the directory cannot answer, the session is kept
// until the ceiling since the last confirmation, then ended, without deleting
// anything. Manual blocking is separate and immediate.
func Sweep(ctx context.Context, db *sql.DB, dir ldapauth.Directory, ceiling time.Duration) (SweepResult, error) {
	var res SweepResult
	rows, err := db.QueryContext(ctx, `
		SELECT e.user_id, e.subject, COALESCE(e.last_verified_at, e.created_at)
		FROM external_identities e
		WHERE e.provider = $1 AND EXISTS (
			SELECT 1 FROM sessions s WHERE s.user_id = e.user_id AND s.revoked_at IS NULL AND s.expires_at > now())`, ldapauth.Provider)
	if err != nil {
		return res, err
	}
	type due struct {
		userID, subject string
		last            time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.userID, &d.subject, &d.last); err == nil {
			list = append(list, d)
		}
	}
	rows.Close()

	store := &sessions.Store{DB: db}
	for _, d := range list {
		res.Checked++
		_, err := dir.LookupBySubject(ctx, d.subject)
		switch {
		case err == nil:
			Touch(ctx, db, d.subject)
			res.Confirmed++
		case errors.Is(err, ldapauth.ErrNotFound):
			revoke(ctx, db, store, d.userID, "identity.deactivated", "the directory no longer has this entry")
			res.Revoked++
		default:
			if time.Since(d.last) > ceiling {
				revoke(ctx, db, store, d.userID, "identity.unverifiable", "the directory could not confirm the identity within the ceiling")
				res.Revoked++
			}
		}
	}
	return res, nil
}

func revoke(ctx context.Context, db *sql.DB, store *sessions.Store, userID, action, why string) {
	if err := store.RevokeAllForUser(ctx, userID); err != nil {
		log.Println("identity: could not end the sessions:", err)
		return
	}
	if err := audit.Record(ctx, db, "", action, "user", userID, map[string]any{"reason": why}); err != nil {
		log.Println("identity: could not audit:", err)
	}
}
