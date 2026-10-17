package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/audit"
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/ocnaibill/codice/backend/internal/version"
)

// DeviceHandler connects a reading device (a Kindle with KOReader) to an account by a code (DEC-194, the device authorization flow of
// RFC 8628): the device asks for a code and shows it, a person who is signed in approves it, and the device is given an access and the
// configuration of this instance. Nothing in the first half is signed in, so every answer says as little as it can.
type DeviceHandler struct {
	DB       *sql.DB
	Sessions *sessions.Store
	// PublicURL is CODICE_PUBLIC_URL, already checked, or empty; TrustedProxies are the proxies whose X-Forwarded-Proto is believed.
	PublicURL      string
	TrustedProxies []netip.Prefix
	// SecretKey is the master key of the deployment: the configuration says whether e-mail is on, which needs it.
	SecretKey string
}

const (
	pairingTTL       = 10 * time.Minute
	pairingInterval  = 5 // seconds the device is told to wait between asks
	pairingMinGap    = 4 * time.Second
	pairingPerIP     = 5 // requests waiting at the same time from one address
	pairingCodeLen   = 8
	deviceConfigVer  = 1
	deviceMinPlugin  = "0.1.0"
	pairingSettings  = "device.pairing"
	pairingNameLimit = 60
)

// pairingAlphabet is the one of the short access: nothing that is mistaken for something else on a screen (0 and o, 1, i and l).
const pairingAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

func drawSymbols(n int, alphabet string) (string, error) {
	out := make([]byte, 0, n)
	buf := make([]byte, 32)
	limit := 256 - 256%len(alphabet) // an int: for sixteen symbols it is 256, which a byte cannot hold
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < n {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	return string(out), nil
}

func hashDeviceCode(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}

// displayUserCode writes the code in two groups of four, the way it is read and typed ("BCDF-2345").
func displayUserCode(code string) string {
	if len(code) != pairingCodeLen {
		return code
	}
	return code[:4] + "-" + code[4:]
}

// normalizeUserCode is what a person typed, read as the code: no spaces or dashes, capitals. It reports false when it cannot be one.
func normalizeUserCode(typed string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		if r == ' ' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	code := b.String()
	if len(code) != pairingCodeLen {
		return "", false
	}
	for _, r := range code {
		if !strings.ContainsRune(pairingAlphabet, r) {
			return "", false
		}
	}
	return code, true
}

var platformRx = regexp.MustCompile(`^[a-z0-9._-]{1,30}$`)

// cleanLabel is a text the device sent about itself, as it is shown to the person who approves: no control characters, one line, short.
func cleanLabel(s string, limit int) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(out); len(r) > limit {
		out = strings.TrimSpace(string(r[:limit]))
	}
	return out
}

func (h *DeviceHandler) baseURL(r *http.Request) string {
	if h.PublicURL != "" {
		return h.PublicURL
	}
	return fmt.Sprintf("%s://%s", appMiddleware.RequestScheme(r, h.TrustedProxies), r.Host)
}

// deviceError answers the way the device reads it: {"error": code, "message": text for a person}.
func deviceError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

func deviceJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// PairingEnabled says whether this instance lets readers be connected by code. On unless the owner turned it off.
func PairingEnabled(ctx context.Context, db *sql.DB) bool {
	var raw []byte
	if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, pairingSettings).Scan(&raw); err != nil {
		return true
	}
	var s struct {
		Enabled *bool `json:"enabled"`
	}
	if json.Unmarshal(raw, &s) != nil || s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// Info is what a device asks first, to know that the address it was given is a Códice and whether it can be connected this way.
func (h *DeviceHandler) Info(w http.ResponseWriter, r *http.Request) {
	deviceJSON(w, http.StatusOK, map[string]any{"name": "Códice", "version": version.Version, "pairing": PairingEnabled(r.Context(), h.DB)})
}

// RequestCode starts a pairing: the device is given the code to show and the one to ask with.
func (h *DeviceHandler) RequestCode(w http.ResponseWriter, r *http.Request) {
	if !PairingEnabled(r.Context(), h.DB) {
		deviceError(w, http.StatusForbidden, "pairing_disabled", "Esta instância não conecta leitores por código.")
		return
	}
	var req struct {
		Client   string `json:"client"`
		Platform string `json:"platform"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&req); err != nil {
		deviceError(w, http.StatusBadRequest, "invalid_request", "Pedido inválido.")
		return
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if !platformRx.MatchString(platform) {
		platform = "other"
	}
	name, client := cleanLabel(req.Name, pairingNameLimit), cleanLabel(req.Client, 60)
	ip := appMiddleware.RequestClientIP(r)

	// the expired ones go with the next request: no job is needed for a table this small
	h.DB.ExecContext(r.Context(), `DELETE FROM device_pairings WHERE expires_at < now() - interval '1 hour'`)
	var waiting int
	if err := h.DB.QueryRowContext(r.Context(), `
		SELECT count(*) FROM device_pairings WHERE request_ip = $1 AND expires_at > now()
		  AND approved_at IS NULL AND denied_at IS NULL AND consumed_at IS NULL`, ip).Scan(&waiting); err == nil && waiting >= pairingPerIP {
		deviceError(w, http.StatusTooManyRequests, "too_many_requests", "Há pedidos demais esperando deste endereço. Espere alguns minutos.")
		return
	}

	deviceCode, err := drawSymbols(48, "0123456789abcdef")
	if err != nil {
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao criar o pedido.")
		return
	}
	var userCode string
	var id int64
	for try := 0; try < 5; try++ {
		if userCode, err = drawSymbols(pairingCodeLen, pairingAlphabet); err != nil {
			break
		}
		err = h.DB.QueryRowContext(r.Context(), `
			INSERT INTO device_pairings (device_code_hash, user_code, name, platform, client, request_ip, expires_at)
			VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, now() + $7 * interval '1 second') RETURNING id`,
			hashDeviceCode(deviceCode), userCode, name, platform, client, ip, int(pairingTTL.Seconds())).Scan(&id)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Println("Error creating a pairing:", err)
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao criar o pedido.")
		return
	}
	// the entry says which device asked and from where, and never either code
	_ = audit.Record(r.Context(), h.DB, "", "device.pair.request", "device_pairing", fmt.Sprint(id), map[string]any{"platform": platform, "name": name, "ip": ip})
	base := h.baseURL(r)
	deviceJSON(w, http.StatusOK, map[string]any{
		"deviceCode": deviceCode, "userCode": displayUserCode(userCode),
		"verificationUri": base + "/pair", "verificationUriComplete": base + "/pair?code=" + userCode,
		"expiresIn": int(pairingTTL.Seconds()), "interval": pairingInterval,
	})
}

// PollToken is what the device asks until the person has answered. The access is made here, in the first ask after the approval, and
// handed over once: it is not kept anywhere before that.
func (h *DeviceHandler) PollToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceCode string `json:"deviceCode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.DeviceCode == "" {
		deviceError(w, http.StatusBadRequest, "invalid_request", "Pedido inválido.")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao consultar o pedido.")
		return
	}
	defer tx.Rollback()
	var (
		id                               int64
		name, platform                   sql.NullString
		expired                          bool
		gap                              sql.NullFloat64
		approvedBy                       sql.NullString
		approvedAt, deniedAt, consumedAt sql.NullTime
	)
	err = tx.QueryRowContext(r.Context(), `
		SELECT id, name, platform, expires_at < now(), EXTRACT(EPOCH FROM now() - last_poll_at), approved_by, approved_at, denied_at, consumed_at
		FROM device_pairings WHERE device_code_hash = $1 FOR UPDATE`, hashDeviceCode(req.DeviceCode)).
		Scan(&id, &name, &platform, &expired, &gap, &approvedBy, &approvedAt, &deniedAt, &consumedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (expired || consumedAt.Valid)) {
		deviceError(w, http.StatusBadRequest, "expired_token", "O pedido venceu. Peça um código novo.")
		return
	}
	if err != nil {
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao consultar o pedido.")
		return
	}
	if deniedAt.Valid {
		deviceError(w, http.StatusBadRequest, "access_denied", "O pedido foi negado.")
		return
	}
	if gap.Valid && time.Duration(gap.Float64*float64(time.Second)) < pairingMinGap {
		tx.ExecContext(r.Context(), `UPDATE device_pairings SET last_poll_at = now() WHERE id = $1`, id)
		tx.Commit()
		deviceJSON(w, http.StatusBadRequest, map[string]any{"error": "slow_down", "message": "Consulte mais devagar.", "interval": pairingInterval + 5})
		return
	}
	if !approvedAt.Valid {
		tx.ExecContext(r.Context(), `UPDATE device_pairings SET last_poll_at = now() WHERE id = $1`, id)
		tx.Commit()
		deviceError(w, http.StatusBadRequest, "authorization_pending", "Esperando a aprovação.")
		return
	}

	// approved: consumed first, so that two asks at once cannot make two accesses
	if _, err := tx.ExecContext(r.Context(), `UPDATE device_pairings SET consumed_at = now(), last_poll_at = now() WHERE id = $1`, id); err != nil {
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao entregar o acesso.")
		return
	}
	label := name.String
	if label == "" {
		label = "Leitor"
	}
	var username string
	if err := tx.QueryRowContext(r.Context(), `SELECT username FROM users WHERE id = $1 AND blocked_at IS NULL`, approvedBy.String).Scan(&username); err != nil {
		deviceError(w, http.StatusBadRequest, "access_denied", "A conta que aprovou não está mais ativa.")
		return
	}
	tokenID, shown, err := h.Sessions.CreateScopedAppToken(r.Context(), approvedBy.String, label, sessions.ScopeKoreader)
	if err != nil {
		log.Println("Error creating the access of a paired device:", err)
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao entregar o acesso.")
		return
	}
	if err := tx.Commit(); err != nil {
		deviceError(w, http.StatusInternalServerError, "server_error", "Erro ao entregar o acesso.")
		return
	}
	_ = audit.Record(r.Context(), h.DB, approvedBy.String, "device.pair.token", "device_pairing", fmt.Sprint(id), map[string]any{"platform": platform.String, "tokenId": tokenID})
	cfg, _ := h.config(r)
	deviceJSON(w, http.StatusOK, map[string]any{
		"token": strings.ReplaceAll(shown, " ", ""), "tokenId": tokenID, "username": username, "config": cfg,
	})
}

// Show is what the page of approval reads: who is asking, from where and when. Only a request that is still waiting is shown.
func (h *DeviceHandler) Show(w http.ResponseWriter, r *http.Request) {
	code, ok := normalizeUserCode(chi.URLParam(r, "code"))
	if !ok {
		http.Error(w, "Pedido não encontrado ou vencido.", http.StatusNotFound)
		return
	}
	var name, platform, client, ip sql.NullString
	var created, expires time.Time
	err := h.DB.QueryRowContext(r.Context(), `
		SELECT name, platform, client, request_ip, created_at, expires_at FROM device_pairings
		WHERE user_code = $1 AND expires_at > now() AND approved_at IS NULL AND denied_at IS NULL AND consumed_at IS NULL`, code).
		Scan(&name, &platform, &client, &ip, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Pedido não encontrado ou vencido.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Erro ao ler o pedido.", http.StatusInternalServerError)
		return
	}
	deviceJSON(w, http.StatusOK, map[string]any{
		"userCode": displayUserCode(code), "name": name.String, "platform": platform.String, "client": client.String,
		"ip": ip.String, "requestedAt": created, "expiresAt": expires,
	})
}

// Approve connects the device to the account that is signed in. The person may give the device a name of their own.
func (h *DeviceHandler) Approve(w http.ResponseWriter, r *http.Request) {
	code, ok := normalizeUserCode(chi.URLParam(r, "code"))
	if !ok {
		http.Error(w, "Pedido não encontrado ou vencido.", http.StatusNotFound)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req)
	rename := cleanLabel(req.Name, pairingNameLimit)
	userID := currentUserID(r)
	var id int64
	var platform, name, ip sql.NullString
	err := h.DB.QueryRowContext(r.Context(), `
		UPDATE device_pairings SET approved_by = $2, approved_at = now(), name = COALESCE(NULLIF($3, ''), name)
		WHERE user_code = $1 AND expires_at > now() AND approved_at IS NULL AND denied_at IS NULL AND consumed_at IS NULL
		RETURNING id, platform, name, request_ip`, code, userID, rename).Scan(&id, &platform, &name, &ip)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Pedido não encontrado ou vencido.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Erro ao aprovar o pedido.", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, userID, "device.pair.approve", "device_pairing", fmt.Sprint(id), map[string]any{"platform": platform.String, "name": name.String, "ip": ip.String})
	w.WriteHeader(http.StatusNoContent)
}

// Deny turns the request down.
func (h *DeviceHandler) Deny(w http.ResponseWriter, r *http.Request) {
	code, ok := normalizeUserCode(chi.URLParam(r, "code"))
	if !ok {
		http.Error(w, "Pedido não encontrado ou vencido.", http.StatusNotFound)
		return
	}
	var id int64
	err := h.DB.QueryRowContext(r.Context(), `
		UPDATE device_pairings SET denied_at = now()
		WHERE user_code = $1 AND expires_at > now() AND approved_at IS NULL AND denied_at IS NULL AND consumed_at IS NULL RETURNING id`, code).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Pedido não encontrado ou vencido.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Erro ao negar o pedido.", http.StatusInternalServerError)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "device.pair.deny", "device_pairing", fmt.Sprint(id), nil)
	w.WriteHeader(http.StatusNoContent)
}

// deviceConfig is what a connected device applies: where this instance's catalog and sync are, and what it offers. The version is the
// shape of this answer; the ETag is its content, so a device applies only what changed.
type deviceConfig struct {
	Version  int               `json:"version"`
	Instance map[string]string `json:"instance"`
	OPDS     map[string]string `json:"opds"`
	KOSync   map[string]string `json:"kosync"`
	Features map[string]bool   `json:"features"`
	MinPlug  string            `json:"minPlugin"`
}

func (h *DeviceHandler) config(r *http.Request) (deviceConfig, error) {
	base := h.baseURL(r)
	_, _, mailErr := MailConfig(r.Context(), h.DB, h.SecretKey)
	return deviceConfig{
		Version:  deviceConfigVer,
		Instance: map[string]string{"name": "Códice", "url": base},
		OPDS:     map[string]string{"title": "Códice", "url": base + "/opds/v1.2/catalog"},
		KOSync:   map[string]string{"url": base + "/kosync"},
		Features: map[string]bool{"sendToKindle": mailErr == nil, "notesImport": true},
		MinPlug:  deviceMinPlugin,
	}, nil
}

// Config is asked by a connected device with its access, to know what to apply. Same content, same ETag: the answer is 304.
func (h *DeviceHandler) Config(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.config(r)
	if err != nil {
		http.Error(w, "Erro ao ler a configuração.", http.StatusInternalServerError)
		return
	}
	raw, _ := json.Marshal(cfg)
	sum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

// PairingSettings (owner) says and changes whether readers can be connected by code.
func (h *DeviceHandler) PairingSettings(w http.ResponseWriter, r *http.Request) {
	deviceJSON(w, http.StatusOK, map[string]bool{"enabled": PairingEnabled(r.Context(), h.DB)})
}

func (h *DeviceHandler) SetPairingSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Enabled == nil {
		http.Error(w, "enabled must be true or false", http.StatusBadRequest)
		return
	}
	raw, _ := json.Marshal(map[string]bool{"enabled": *req.Enabled})
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO settings (key, value) VALUES ($1, $2::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, pairingSettings, raw); err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "device.pairing.configure", "settings", pairingSettings, map[string]any{"enabled": *req.Enabled}); err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	deviceJSON(w, http.StatusOK, map[string]bool{"enabled": *req.Enabled})
}
