package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/ownership"
	"golang.org/x/crypto/bcrypt"
)

// OwnershipHandler is the two-step transfer of ownership (RF-038): the owner
// starts it confirming with their password, and the chosen account accepts by
// signing in again. Recovery when the owner is unavailable is NOT here: it exists
// only as a command on the server (DEC-057).
type OwnershipHandler struct {
	DB *sql.DB
}

// passwordMatches checks the caller's own password, for the actions that must not
// run on a session alone.
func (h *OwnershipHandler) passwordMatches(r *http.Request, userID, password string) (bool, error) {
	var hash string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(password_hash, '') FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		return false, err
	}
	if hash == "" {
		burnPasswordCheck(password)
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}

func ownershipError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ownership.ErrNotOwner):
		http.Error(w, "Forbidden", http.StatusForbidden)
	case errors.Is(err, ownership.ErrNotFound), errors.Is(err, ownership.ErrNoTransfer):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ownership.ErrBadRole):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ownership.ErrNotEligible), errors.Is(err, ownership.ErrPending):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		log.Println("Ownership error:", err)
		http.Error(w, "Error changing ownership", http.StatusInternalServerError)
	}
}

// Get shows the transfer waiting for an answer that involves the caller, either
// the one they started (outgoing) or the one offered to them.
func (h *OwnershipHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, outgoing, err := ownership.Pending(r.Context(), h.DB, currentUserID(r))
	w.Header().Set("Content-Type", "application/json")
	if errors.Is(err, ownership.ErrNoTransfer) {
		json.NewEncoder(w).Encode(map[string]any{"transfer": nil})
		return
	}
	if err != nil {
		http.Error(w, "Error reading the transfer", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"transfer": t, "outgoing": outgoing})
}

// Start begins a transfer. It needs the owner's password and an explicit choice
// of what the owner becomes: there is no default (DEC-058).
func (h *OwnershipHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetID   string `json:"targetId"`
		FormerRole string `json:"formerRole"`
		Password   string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !uuidPattern.MatchString(req.TargetID) {
		http.Error(w, "targetId and formerRole are required", http.StatusBadRequest)
		return
	}
	ownerID := currentUserID(r)
	ok, err := h.passwordMatches(r, ownerID, req.Password)
	if err != nil {
		http.Error(w, "Error checking the password", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "The password is not correct", http.StatusForbidden)
		return
	}
	t, err := ownership.Start(r.Context(), h.DB, ownerID, req.TargetID, req.FormerRole)
	if err != nil {
		ownershipError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
}

// Cancel withdraws the transfer the owner started.
func (h *OwnershipHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if err := ownership.Cancel(r.Context(), h.DB, currentUserID(r)); err != nil {
		ownershipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Accept completes the transfer offered to the caller. Signing in again is part
// of it: a session someone else might be holding is not enough to take over the
// instance.
func (h *OwnershipHandler) Accept(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	id := currentUserID(r)
	ok, err := h.passwordMatches(r, id, req.Password)
	if err != nil {
		http.Error(w, "Error checking the password", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "The password is not correct", http.StatusForbidden)
		return
	}
	if _, err := ownership.Accept(r.Context(), h.DB, id); err != nil {
		ownershipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Decline refuses the transfer offered to the caller.
func (h *OwnershipHandler) Decline(w http.ResponseWriter, r *http.Request) {
	if err := ownership.Decline(r.Context(), h.DB, currentUserID(r)); err != nil {
		ownershipError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AckNotice marks one of the caller's security notices as seen.
func AckNotice(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			http.Error(w, "Notice not found", http.StatusNotFound)
			return
		}
		res, err := db.ExecContext(r.Context(), `UPDATE security_notices SET acknowledged_at = now()
			WHERE id = $1 AND user_id = $2 AND acknowledged_at IS NULL`, id, currentUserID(r))
		if err != nil {
			http.Error(w, "Error updating the notice", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, "Notice not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
