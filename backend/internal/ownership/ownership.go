// Package ownership holds the rules for changing hands of the instance: the
// two-step transfer the owner starts from the interface (RF-038) and the two
// recovery operations that only the local command can run (DEC-057). The API
// and the command share this code, so there is one implementation of "who is
// the owner" and the invariant that there is exactly one.
package ownership

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/secrets"
	"github.com/ocnaibill/codice/backend/internal/sessions"
)

// TransferTTL is how long an unanswered transfer waits.
const TransferTTL = 7 * 24 * time.Hour

// RecoveryLinkTTL is the life of the reset link the local command prints (DEC-057).
const RecoveryLinkTTL = time.Hour

// lockKey serialises every change of ownership, so two of them can never
// interleave (pg_advisory_xact_lock). The value is arbitrary but must stay constant.
const lockKey = 7301133

var (
	ErrNotOwner    = errors.New("only the owner can do this")
	ErrNotEligible = errors.New("that account cannot receive ownership: it must be active, have a local password and no linked external identity")
	ErrPending     = errors.New("a transfer is already waiting for an answer")
	ErrNoTransfer  = errors.New("there is no transfer waiting for an answer")
	ErrBadRole     = errors.New("the former owner becomes 'admin' or 'reader'")
	ErrNoOwner     = errors.New("the instance has no owner")
	ErrNotFound    = errors.New("account not found")
)

type Transfer struct {
	ID         string    `json:"id"`
	FromUserID string    `json:"-"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ToUserID   string    `json:"toId"`
	FormerRole string    `json:"formerRole"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func begin(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// eligible reports whether an account may become the owner (DEC-073: the owner
// signs in locally only).
func eligible(ctx context.Context, tx *sql.Tx, id string) (username string, err error) {
	var role string
	var blocked, hasPassword, external bool
	err = tx.QueryRowContext(ctx, `
		SELECT username, COALESCE(role, 'reader'), blocked_at IS NOT NULL, COALESCE(password_hash, '') <> '',
		       EXISTS (SELECT 1 FROM external_identities e WHERE e.user_id = users.id)
		FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&username, &role, &blocked, &hasPassword, &external)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if role == authz.RoleOwner || blocked || !hasPassword || external {
		return "", ErrNotEligible
	}
	return username, nil
}

func expireLapsed(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE ownership_transfers SET state = 'expired', decided_at = now()
		WHERE state = 'pending' AND expires_at <= now()`)
	return err
}

// Start begins a transfer. The caller has already checked the owner's password.
// Nothing changes for anyone until the target accepts.
func Start(ctx context.Context, db *sql.DB, ownerID, targetID, formerRole string) (Transfer, error) {
	var t Transfer
	if !authz.IsAssignableRole(formerRole) {
		return t, ErrBadRole
	}
	tx, err := begin(ctx, db)
	if err != nil {
		return t, err
	}
	defer tx.Rollback()

	var ownerName, role string
	if err := tx.QueryRowContext(ctx, `SELECT username, role FROM users WHERE id = $1 FOR UPDATE`, ownerID).Scan(&ownerName, &role); err != nil || role != authz.RoleOwner {
		return t, ErrNotOwner
	}
	if targetID == ownerID {
		return t, ErrNotEligible
	}
	targetName, err := eligible(ctx, tx, targetID)
	if err != nil {
		return t, err
	}
	if err := expireLapsed(ctx, tx); err != nil {
		return t, err
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ownership_transfers (from_user, to_user, former_role, expires_at)
		VALUES ($1, $2, $3, now() + $4::interval) RETURNING id, expires_at`,
		ownerID, targetID, formerRole, TransferTTL.String()).Scan(&t.ID, &t.ExpiresAt)
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		return t, ErrPending
	}
	if err != nil {
		return t, err
	}
	if err := audit.Record(ctx, tx, ownerID, "ownership.transfer_start", "user", targetID,
		map[string]any{"to": targetName, "formerRole": formerRole, "expiresAt": t.ExpiresAt}); err != nil {
		return t, err
	}
	t.From, t.To, t.ToUserID, t.FormerRole = ownerName, targetName, targetID, formerRole
	return t, tx.Commit()
}

// Pending returns the transfer waiting for an answer that involves this account,
// as the owner who started it or as the account it is offered to.
func Pending(ctx context.Context, db *sql.DB, userID string) (t Transfer, outgoing bool, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT t.id, t.from_user, f.username, t.to_user, r.username, t.former_role, t.expires_at
		FROM ownership_transfers t JOIN users f ON f.id = t.from_user JOIN users r ON r.id = t.to_user
		WHERE t.state = 'pending' AND t.expires_at > now() AND (t.from_user = $1 OR t.to_user = $1)`, userID).
		Scan(&t.ID, &t.FromUserID, &t.From, &t.ToUserID, &t.To, &t.FormerRole, &t.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, false, ErrNoTransfer
	}
	return t, t.FromUserID == userID, err
}

func decide(ctx context.Context, db *sql.DB, who string, asOwner bool, state, action string) error {
	tx, err := begin(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	col := "to_user"
	if asOwner {
		col = "from_user"
	}
	var id, to string
	err = tx.QueryRowContext(ctx, `UPDATE ownership_transfers SET state = $2, decided_at = now()
		WHERE state = 'pending' AND expires_at > now() AND `+col+` = $1 RETURNING id, to_user`, who, state).Scan(&id, &to)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoTransfer
	}
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, who, action, "user", to, nil); err != nil {
		return err
	}
	return tx.Commit()
}

// Cancel withdraws the pending transfer; only the owner who started it can.
func Cancel(ctx context.Context, db *sql.DB, ownerID string) error {
	return decide(ctx, db, ownerID, true, "cancelled", "ownership.transfer_cancel")
}

// Decline is the target refusing the offer.
func Decline(ctx context.Context, db *sql.DB, targetID string) error {
	return decide(ctx, db, targetID, false, "declined", "ownership.transfer_decline")
}

// swap makes newID the owner and oldID the former role, inside the caller's
// transaction. The database has a unique index that allows a single owner and a
// trigger that refuses an owner with an external identity, so a mistake here
// fails instead of leaving two owners or none.
func swap(ctx context.Context, tx *sql.Tx, oldID, newID, formerRole string) error {
	res, err := tx.ExecContext(ctx, `UPDATE users SET role = $2 WHERE id = $1 AND role = 'owner'`, oldID, formerRole)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotOwner
	}
	res, err = tx.ExecContext(ctx, `UPDATE users SET role = 'owner' WHERE id = $1 AND role <> 'owner' AND blocked_at IS NULL`, newID)
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23514" {
		return ErrNotEligible
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotEligible
	}
	return nil
}

// Accept completes the pending transfer offered to targetID. The caller has
// already had the target sign in again. Either both roles change or neither does.
func Accept(ctx context.Context, db *sql.DB, targetID string) (Transfer, error) {
	var t Transfer
	tx, err := begin(ctx, db)
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		SELECT t.id, t.from_user, f.username, t.to_user, r.username, t.former_role, t.expires_at
		FROM ownership_transfers t JOIN users f ON f.id = t.from_user JOIN users r ON r.id = t.to_user
		WHERE t.state = 'pending' AND t.expires_at > now() AND t.to_user = $1 FOR UPDATE OF t`, targetID).
		Scan(&t.ID, &t.FromUserID, &t.From, &t.ToUserID, &t.To, &t.FormerRole, &t.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNoTransfer
	}
	if err != nil {
		return t, err
	}
	if _, err := eligible(ctx, tx, targetID); err != nil {
		return t, err
	}
	if err := swap(ctx, tx, t.FromUserID, targetID, t.FormerRole); err != nil {
		return t, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ownership_transfers SET state = 'accepted', decided_at = now() WHERE id = $1`, t.ID); err != nil {
		return t, err
	}
	if err := audit.Record(ctx, tx, targetID, "ownership.transfer_accept", "user", t.FromUserID,
		map[string]any{"from": t.From, "to": t.To, "formerRole": t.FormerRole}); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

// Notice records something a person must see at their next sign-in.
func Notice(ctx context.Context, tx *sql.Tx, userID, kind string, details map[string]any) error {
	payload, err := jsonBytes(details)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO security_notices (user_id, kind, details) VALUES ($1, $2, $3::jsonb)`, userID, kind, payload)
	return err
}

// RecoverAccess is the local command's first case (DEC-057): the owner lost their
// password or second factor. It ends every session and app token of the owner and
// returns a single-use reset link, valid for one hour, that only the caller sees.
// The owner is told at their next sign-in that this happened.
func RecoverAccess(ctx context.Context, db *sql.DB) (token string, expires time.Time, owner string, err error) {
	tx, err := begin(ctx, db)
	if err != nil {
		return "", expires, "", err
	}
	defer tx.Rollback()
	var ownerID string
	err = tx.QueryRowContext(ctx, `SELECT id, username FROM users WHERE role = 'owner' FOR UPDATE`).Scan(&ownerID, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", expires, "", ErrNoOwner
	}
	if err != nil {
		return "", expires, "", err
	}
	if err := sessions.RevokeAllForUserTx(ctx, tx, ownerID); err != nil {
		return "", expires, "", err
	}
	// Older links of the owner die: only the one printed now works.
	if _, err := tx.ExecContext(ctx, `UPDATE password_resets SET token_expires_at = now()
		WHERE user_id = $1 AND decision = 'approved' AND used_at IS NULL`, ownerID); err != nil {
		return "", expires, "", err
	}
	if token, err = secrets.NewToken(); err != nil {
		return "", expires, "", err
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO password_resets (user_id, decision, decided_at, token_hash, token_expires_at)
		VALUES ($1, 'approved', now(), $2, now() + $3::interval) RETURNING token_expires_at`,
		ownerID, secrets.Hash(token), RecoveryLinkTTL.String()).Scan(&expires); err != nil {
		return "", expires, "", err
	}
	if err := Notice(ctx, tx, ownerID, "owner_recovery_reset", map[string]any{"expiresAt": expires}); err != nil {
		return "", expires, "", err
	}
	if err := audit.Record(ctx, tx, "", "ownership.recovery_reset", "user", ownerID, map[string]any{"by": "local command", "username": owner}); err != nil {
		return "", expires, "", err
	}
	return token, expires, owner, tx.Commit()
}

// RecoverTransfer is the local command's second case (DEC-057): the owner is
// unavailable, so ownership goes to an existing account in one transaction. There
// is no owner present to choose the former owner's role, so the caller passes it
// (the command defaults to reader, the conservative choice). The former owner's
// sessions and tokens end, since their access may be what was compromised.
func RecoverTransfer(ctx context.Context, db *sql.DB, toUsername, formerRole string) (from string, err error) {
	if !authz.IsAssignableRole(formerRole) {
		return "", ErrBadRole
	}
	tx, err := begin(ctx, db)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var toID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = $1`, toUsername).Scan(&toID); errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	if _, err := eligible(ctx, tx, toID); err != nil {
		return "", err
	}
	var fromID string
	err = tx.QueryRowContext(ctx, `SELECT id, username FROM users WHERE role = 'owner' FOR UPDATE`).Scan(&fromID, &from)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoOwner
	}
	if err != nil {
		return "", err
	}
	if err := swap(ctx, tx, fromID, toID, formerRole); err != nil {
		return "", err
	}
	if err := sessions.RevokeAllForUserTx(ctx, tx, fromID); err != nil {
		return "", err
	}
	// A transfer the former owner had started no longer makes sense.
	if _, err := tx.ExecContext(ctx, `UPDATE ownership_transfers SET state = 'cancelled', decided_at = now() WHERE state = 'pending'`); err != nil {
		return "", err
	}
	details := map[string]any{"from": from, "to": toUsername, "formerRole": formerRole}
	for _, id := range []string{fromID, toID} {
		if err := Notice(ctx, tx, id, "owner_recovery_transfer", details); err != nil {
			return "", err
		}
	}
	if err := audit.Record(ctx, tx, "", "ownership.recovery_transfer", "user", toID, details); err != nil {
		return "", err
	}
	return from, tx.Commit()
}
