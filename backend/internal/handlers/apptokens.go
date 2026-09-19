package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
)

// AppTokensHandler lets a user manage the tokens their OPDS and sync clients use
// instead of the account password (DEC-071). Users only see and revoke their own.
type AppTokensHandler struct {
	Sessions *sessions.Store
}

type createAppTokenRequest struct {
	Name string `json:"name"`
}

type createAppTokenResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"` // shown once; only its hash is stored
}

func (h *AppTokensHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAppTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		http.Error(w, "name is required (up to 100 characters)", http.StatusBadRequest)
		return
	}

	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	id, token, err := h.Sessions.CreateAppToken(r.Context(), userID, name)
	if err != nil {
		http.Error(w, "Error creating token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createAppTokenResponse{ID: id, Name: name, Token: token})
}

func (h *AppTokensHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	tokens, err := h.Sessions.ListAppTokens(r.Context(), userID)
	if err != nil {
		http.Error(w, "Error listing tokens", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

func (h *AppTokensHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	ok, err := h.Sessions.RevokeAppToken(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Error revoking token", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
