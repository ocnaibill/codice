package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/audit"
	"github.com/ocnaibill/codice/backend/internal/mailer"
	"github.com/ocnaibill/codice/backend/internal/sealed"
)

// mailPurpose binds the sealed password to what it is for.
const mailPurpose = "codice:smtp-password"

// MailSettings is the mail server as it is kept (in the settings, key mail.smtp): the password only sealed.
type MailSettings struct {
	Enabled        bool   `json:"enabled"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Security       string `json:"security"`
	Username       string `json:"username"`
	PasswordSealed string `json:"passwordSealed,omitempty"`
	From           string `json:"from"`
	InsecureTLS    bool   `json:"insecureTLS"`
	// MaxMB is the biggest book sent, in megabytes: what the server of the house takes in a message (25 is the usual) and never more than 50.
	MaxMB int `json:"maxMB"`
}

const (
	defaultMailMB = 25
	maxMailMB     = 50
)

// MailAdminHandler is the tab of the e-mail of the administration (DEC-059: optional, off by default): the owner says which server sends the
// books a person mails to their e-reader, and tests it.
type MailAdminHandler struct {
	DB *sql.DB
	// SecretKey is the master key of the deployment (CODICE_SECRET_KEY): without it a password cannot be kept.
	SecretKey string
}

func loadMailSettings(ctx context.Context, db *sql.DB) (MailSettings, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'mail.smtp'`).Scan(&raw)
	var s MailSettings
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(raw, &s)
	return s, nil
}

// MailConfig is the server as the mailer wants it, when it is on and complete. The error is a sentence for the person.
func MailConfig(ctx context.Context, db *sql.DB, master string) (mailer.Config, MailSettings, error) {
	s, err := loadMailSettings(ctx, db)
	if err != nil {
		return mailer.Config{}, s, err
	}
	if !s.Enabled {
		return mailer.Config{}, s, errMailOff
	}
	c := mailer.Config{Host: s.Host, Port: s.Port, Security: s.Security, Username: s.Username, From: s.From, InsecureTLS: s.InsecureTLS}
	if s.PasswordSealed != "" {
		if c.Password, err = sealed.Open(master, mailPurpose, s.PasswordSealed); err != nil {
			return c, s, errMailPassword
		}
	}
	if err := c.Validate(); err != nil {
		return c, s, errMailOff
	}
	return c, s, nil
}

var (
	errMailOff      = errors.New("O envio por e-mail não está ligado nesta instância.")
	errMailPassword = errors.New("A senha do servidor de e-mail não abre com a chave mestra deste servidor: digite-a de novo na administração.")
)

type mailView struct {
	MailSettings
	PasswordSealed string `json:"-"`
	HasPassword    bool   `json:"hasPassword"`
	// MasterKey says whether this deployment has the master key a password needs.
	MasterKey bool `json:"masterKey"`
}

func (h *MailAdminHandler) view(s MailSettings) mailView {
	if s.MaxMB == 0 {
		s.MaxMB = defaultMailMB
	}
	if s.Port == 0 {
		s.Port = 587
	}
	if s.Security == "" {
		s.Security = mailer.SecurityStartTLS
	}
	return mailView{MailSettings: s, HasPassword: s.PasswordSealed != "", MasterKey: len(h.SecretKey) >= sealed.MinMasterKey}
}

// Get shows the server (never the password).
func (h *MailAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := loadMailSettings(r.Context(), h.DB)
	if err != nil {
		log.Println("Error reading the mail settings:", err)
		http.Error(w, "Error reading the settings", http.StatusInternalServerError)
		return
	}
	v := h.view(s)
	v.MailSettings.PasswordSealed = ""
	writeJSON(w, http.StatusOK, v)
}

type mailRequest struct {
	Enabled     bool    `json:"enabled"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Security    string  `json:"security"`
	Username    string  `json:"username"`
	Password    *string `json:"password"` // absent keeps the one kept; "" takes it away
	From        string  `json:"from"`
	InsecureTLS bool    `json:"insecureTLS"`
	MaxMB       int     `json:"maxMB"`
}

// Put saves the server. It is checked as a whole (a server that is turned on must be complete); the password is sealed under the master key.
func (h *MailAdminHandler) Put(w http.ResponseWriter, r *http.Request) {
	var req mailRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	current, err := loadMailSettings(r.Context(), h.DB)
	if err != nil {
		http.Error(w, "Error reading the settings", http.StatusInternalServerError)
		return
	}
	next := MailSettings{Enabled: req.Enabled, Host: strings.TrimSpace(req.Host), Port: req.Port, Security: req.Security,
		Username: strings.TrimSpace(req.Username), From: strings.TrimSpace(req.From), InsecureTLS: req.InsecureTLS, MaxMB: req.MaxMB,
		PasswordSealed: current.PasswordSealed}
	if next.MaxMB == 0 {
		next.MaxMB = defaultMailMB
	}
	if next.MaxMB < 1 || next.MaxMB > maxMailMB {
		http.Error(w, "O tamanho máximo é de 1 a 50 MB.", http.StatusBadRequest)
		return
	}
	if strings.ContainsAny(next.Host+next.Username+next.From, "\r\n") || len(next.Host) > 253 || len(next.Username) > 254 {
		http.Error(w, "Algum campo tem caracteres que não podem ficar aí.", http.StatusBadRequest)
		return
	}
	if req.Password != nil {
		if *req.Password == "" {
			next.PasswordSealed = ""
		} else {
			if len(*req.Password) > 512 {
				http.Error(w, "A senha é longa demais.", http.StatusBadRequest)
				return
			}
			sealedPassword, err := sealed.Seal(h.SecretKey, mailPurpose, *req.Password)
			if errors.Is(err, sealed.ErrNoMasterKey) {
				http.Error(w, "Falta a chave mestra deste servidor (CODICE_SECRET_KEY): sem ela a senha não pode ser guardada.", http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(w, "Error saving", http.StatusInternalServerError)
				return
			}
			next.PasswordSealed = sealedPassword
		}
	}
	if next.Enabled {
		if err := (mailer.Config{Host: next.Host, Port: next.Port, Security: next.Security, From: next.From}).Validate(); err != nil {
			http.Error(w, "Para ligar, preencha o servidor, a porta, a proteção da conexão e o remetente (um endereço de e-mail).", http.StatusBadRequest)
			return
		}
	}
	raw, _ := json.Marshal(next)
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO settings (key, value) VALUES ('mail.smtp', $1::jsonb) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, raw); err != nil {
		log.Println("Error saving the mail settings:", err)
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	// the entry says what changed and never the password
	if err := audit.Record(r.Context(), tx, currentUserID(r), "mail.configure", "settings", "mail.smtp",
		map[string]any{"enabled": next.Enabled, "host": next.Host, "port": next.Port, "security": next.Security, "passwordChanged": req.Password != nil}); err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error saving", http.StatusInternalServerError)
		return
	}
	v := h.view(next)
	v.MailSettings.PasswordSealed = ""
	writeJSON(w, http.StatusOK, v)
}

// Test sends a short message to the address the owner gives, through the server as it is kept.
func (h *MailAdminHandler) Test(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	to, err := mailer.Address(req.To)
	if err != nil {
		http.Error(w, "Digite um endereço de e-mail para receber o teste.", http.StatusBadRequest)
		return
	}
	cfg, _, err := MailConfig(r.Context(), h.DB, h.SecretKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := mailer.Send(r.Context(), cfg, mailer.Message{To: to, Subject: "Teste do Códice", Body: "Se você recebeu esta mensagem, o envio de e-mail do Códice está funcionando."}); err != nil {
		http.Error(w, "Não foi possível enviar: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = audit.Record(r.Context(), h.DB, currentUserID(r), "mail.test", "settings", "mail.smtp", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}
