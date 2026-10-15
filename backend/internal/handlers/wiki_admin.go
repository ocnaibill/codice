package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"database/sql"
	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/authz"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/wiki"
	"github.com/redis/go-redis/v9"
)

// WikiAdminHandler is the tab of the wiki in the administration (DEC-185): the provider, which only the owner configures (DEC-177),
// the queue of generations, and the works and series with their exceptions, for owner and admins.
type WikiAdminHandler struct {
	DB          *sql.DB
	RedisClient *redis.Client
	// SecretKey is the master key of the deployment (CODICE_SECRET_KEY): without it the key of the provider cannot be kept.
	SecretKey string
	// HTTP calls the provider for "Testar"; nil uses a client of its own.
	HTTP *http.Client
}

func isOwnerRequest(r *http.Request) bool {
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	return role == authz.RoleOwner
}

type providerView struct {
	Configured bool   `json:"configured"`
	Kind       string `json:"kind,omitempty"`
	Model      string `json:"model,omitempty"`
	Host       string `json:"host,omitempty"`
	Remote     bool   `json:"remote"`
	// For the owner only.
	BaseURL       string `json:"baseUrl,omitempty"`
	KeySet        bool   `json:"keySet,omitempty"`
	KeyHint       string `json:"keyHint,omitempty"`
	KeyUnreadable bool   `json:"keyUnreadable,omitempty"`
	Policy        string `json:"policy,omitempty"`
	MasterKey     *bool  `json:"masterKey,omitempty"`
}

func (h *WikiAdminHandler) view(r *http.Request, p *wiki.Provider) providerView {
	v := providerView{}
	owner := isOwnerRequest(r)
	if owner {
		has := len(h.SecretKey) >= wiki.MinMasterKey
		v.MasterKey = &has
	}
	if p == nil {
		return v
	}
	v.Configured, v.Kind, v.Model, v.Host, v.Remote = true, p.Kind, p.Model, p.Host(), p.Remote()
	if owner {
		v.BaseURL, v.KeySet, v.KeyHint, v.Policy = p.BaseURL, p.KeySealed != "", p.KeyHint, p.Policy()
		if p.KeySealed != "" {
			if _, err := wiki.Open(h.SecretKey, p.KeySealed); err != nil {
				v.KeyUnreadable = true
			}
		}
	}
	return v
}

// Provider says which provider writes the wiki: everything but the key to the owner, which service and model to admins.
func (h *WikiAdminHandler) Provider(w http.ResponseWriter, r *http.Request) {
	p, err := wiki.LoadProvider(r.Context(), h.DB)
	if err != nil {
		log.Println("Error reading the wiki provider:", err)
		http.Error(w, "Error reading the wiki provider", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, h.view(r, p))
}

// SetProvider configures the provider (owner). The key is sealed before it is stored and never sent back.
func (h *WikiAdminHandler) SetProvider(w http.ResponseWriter, r *http.Request) {
	var in wiki.ProviderInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		http.Error(w, "invalid provider", http.StatusBadRequest)
		return
	}
	before, err := wiki.LoadProvider(r.Context(), h.DB)
	if err != nil {
		log.Println("Error reading the wiki provider:", err)
		http.Error(w, "Error saving the wiki provider", http.StatusInternalServerError)
		return
	}
	p, err := wiki.Apply(in, before, h.SecretKey)
	var bad wiki.InputError
	switch {
	case errors.As(err, &bad):
		http.Error(w, bad.Msg, http.StatusBadRequest)
		return
	case errors.Is(err, wiki.ErrNoMasterKey):
		http.Error(w, "Para guardar a chave, defina CODICE_SECRET_KEY (32 caracteres ou mais) no ambiente do servidor e do worker e reinicie-os.", http.StatusConflict)
		return
	case err != nil:
		log.Println("Error sealing the wiki provider key:", err)
		http.Error(w, "Error saving the wiki provider", http.StatusInternalServerError)
		return
	}
	if err := wiki.SaveProvider(r.Context(), h.DB, p, currentUserID(r)); err != nil {
		log.Println("Error saving the wiki provider:", err)
		http.Error(w, "Error saving the wiki provider", http.StatusInternalServerError)
		return
	}
	keyChanged := in.Key != "" || in.ClearKey
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "wiki.provider", "settings", wiki.SettingProvider,
		map[string]any{"kind": p.Kind, "host": p.Host(), "model": p.Model, "remote": p.Remote(), "keyChanged": keyChanged})
	writeJSON(w, http.StatusOK, h.view(r, p))
}

// DeleteProvider turns the generation off (owner): what is queued waits, and nothing starts until a provider is configured.
func (h *WikiAdminHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	if err := wiki.DeleteProvider(r.Context(), h.DB); err != nil {
		log.Println("Error removing the wiki provider:", err)
		http.Error(w, "Error removing the wiki provider", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "wiki.provider.delete", "settings", wiki.SettingProvider, nil)
	writeJSON(w, http.StatusOK, h.view(r, nil))
}

// TestProvider asks the configured model for one short answer (owner).
func (h *WikiAdminHandler) TestProvider(w http.ResponseWriter, r *http.Request) {
	p, err := wiki.LoadProvider(r.Context(), h.DB)
	if err != nil {
		log.Println("Error reading the wiki provider:", err)
		http.Error(w, "Error testing the wiki provider", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, "Configure o provedor antes de testar.", http.StatusConflict)
		return
	}
	key := ""
	if p.KeySealed != "" {
		if key, err = wiki.Open(h.SecretKey, p.KeySealed); err != nil {
			writeJSON(w, http.StatusOK, wiki.TestResult{Problem: "A chave guardada não abre com a chave mestra deste servidor: digite a chave de novo."})
			return
		}
	}
	client := h.HTTP
	if client == nil {
		client = &http.Client{Timeout: 70 * time.Second}
	}
	res := wiki.Test(r.Context(), client, p, key)
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "wiki.provider.test", "settings", wiki.SettingProvider, map[string]any{"ok": res.OK})
	writeJSON(w, http.StatusOK, res)
}

// Runs lists the generations: the live ones in the order they run, and the last that finished.
func (h *WikiAdminHandler) Runs(w http.ResponseWriter, r *http.Request) {
	runs, err := wiki.Runs(r.Context(), h.DB, 20)
	if err != nil {
		log.Println("Error listing wiki runs:", err)
		http.Error(w, "Error listing the wiki generations", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (h *WikiAdminHandler) wake(r *http.Request, runID int64) {
	// Wake the workers; a failure only means they find it on their next poll.
	_ = jobs.Notify(r.Context(), h.RedisClient, runID)
}

func enqueueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wiki.ErrNoProvider):
		http.Error(w, "Configure o provedor da wiki antes de gerar: só o dono do acervo pode fazer isso.", http.StatusConflict)
	case errors.Is(err, wiki.ErrNotEligible):
		http.Error(w, "Esta obra ou série não tem livro em EPUB com o texto lido pelo Códice.", http.StatusBadRequest)
	default:
		log.Println("Error queueing a wiki generation:", err)
		http.Error(w, "Error queueing the wiki generation", http.StatusInternalServerError)
	}
}

// Generate queues the generation of one work or series, even one in the exceptions (a direct request, DEC-185).
func (h *WikiAdminHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var s wiki.Scope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&s); err != nil {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}
	id, created, err := wiki.Enqueue(r.Context(), h.DB, s, "manual", currentUserID(r))
	if err != nil {
		enqueueError(w, err)
		return
	}
	h.wake(r, id)
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"runId": id, "alreadyQueued": !created})
}

// GenerateAll queues every work and series without a wiki, but the exceptions.
func (h *WikiAdminHandler) GenerateAll(w http.ResponseWriter, r *http.Request) {
	n, err := wiki.EnqueueAll(r.Context(), h.DB, currentUserID(r))
	if err != nil {
		enqueueError(w, err)
		return
	}
	if n > 0 {
		h.wake(r, 0)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": n})
}

// ControlRun pauses, resumes or cancels a generation.
func (h *WikiAdminHandler) ControlRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	action := chi.URLParam(r, "action")
	if err != nil || id <= 0 {
		http.Error(w, "Generation not found", http.StatusNotFound)
		return
	}
	switch err := wiki.Control(r.Context(), h.DB, id, action, currentUserID(r)); {
	case errors.Is(err, wiki.ErrNotFound):
		http.Error(w, "Generation not found", http.StatusNotFound)
	case errors.Is(err, wiki.ErrState):
		http.Error(w, "Esta geração não está num estado que permita isso.", http.StatusConflict)
	case err != nil:
		log.Println("Error controlling a wiki run:", err)
		http.Error(w, "Error changing the wiki generation", http.StatusInternalServerError)
	default:
		if action == "resume" {
			h.wake(r, id)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Library lists the works and series that can have a wiki, with their wiki, their live generation and whether they are excluded.
func (h *WikiAdminHandler) Library(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	items, total, err := wiki.Library(r.Context(), h.DB, q.Get("q"), q.Get("filter"), limit, offset)
	if err != nil {
		log.Println("Error listing the wiki library:", err)
		http.Error(w, "Error listing the works for the wiki", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// SetExcluded marks or unmarks a work or series as "não gerar wiki".
func (h *WikiAdminHandler) SetExcluded(w http.ResponseWriter, r *http.Request) {
	var req struct {
		wiki.Scope
		Excluded bool `json:"excluded"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}
	switch err := wiki.SetExcluded(r.Context(), h.DB, req.Scope, req.Excluded, currentUserID(r)); {
	case errors.Is(err, wiki.ErrNotEligible):
		http.Error(w, "invalid scope", http.StatusBadRequest)
	case err != nil:
		log.Println("Error changing a wiki exclusion:", err)
		http.Error(w, "Error changing the exception", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
