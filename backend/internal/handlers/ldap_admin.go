package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/identity"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
)

// LDAPAdminHandler is the owner's view of the directory sign-in. The connection
// (address, service account and its secret) is deployment configuration read from
// the environment: it is shown here without any secret, and never editable over
// HTTP. The policy, which is the owner's to decide, is editable.
type LDAPAdminHandler struct {
	DB *sql.DB
	// Directory is nil when LDAP is not configured. Host and BaseDN describe it.
	Directory ldapauth.Directory
	Host      string
	BaseDN    string
}

// Get describes the state.
func (h *LDAPAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	policy, err := identity.GetPolicy(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the policy", http.StatusInternalServerError)
		return
	}
	var linked int
	h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM external_identities WHERE provider = $1`, ldapauth.Provider).Scan(&linked)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"configured": h.Directory != nil, "host": h.Host, "baseDN": h.BaseDN,
		"policy": policy, "linkedAccounts": linked,
	})
}

// SetPolicy changes what the owner decides: whether a first login creates an
// account, and the revalidation ceiling.
func (h *LDAPAdminHandler) SetPolicy(w http.ResponseWriter, r *http.Request) {
	var p identity.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if p.RevalidateHours < 1 || p.RevalidateHours > 24*14 {
		http.Error(w, "revalidateHours must be between 1 and 336", http.StatusBadRequest)
		return
	}
	if p.AllowCreate && h.Directory == nil {
		http.Error(w, "LDAP is not configured on the server, so accounts cannot be created from it", http.StatusConflict)
		return
	}
	if err := identity.SetPolicy(r.Context(), h.DB, p, currentUserID(r)); err != nil {
		log.Println("Error saving the LDAP policy:", err)
		http.Error(w, "Error saving the policy", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// Check tests the connection and the service account, so the owner can tell a
// wrong address or password from everything else. It says which kind of failure,
// never the directory's own message, which could carry details.
func (h *LDAPAdminHandler) Check(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.Directory == nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "reason": "not_configured"})
		return
	}
	err := h.Directory.Check(r.Context())
	reason := ""
	if err != nil {
		// The reason (a certificate that does not match, a refused service account...)
		// goes to the server log for the operator; the screen only gets the category.
		log.Printf("LDAP connection test failed: %v", err)
		reason = "unavailable"
		if !errors.Is(err, ldapauth.ErrUnavailable) {
			reason = "error"
		}
	}
	audit.Record(r.Context(), h.DB, currentUserID(r), "ldap.check", "settings", "ldap", map[string]any{"ok": err == nil})
	json.NewEncoder(w).Encode(map[string]any{"ok": err == nil, "reason": reason})
}
