package wiki

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode"
)

// The provider of the wiki (DEC-177, DEC-185): one service that writes it, configured by the owner. Every kind speaks the chat
// completions of OpenAI at {base}/chat/completions: DeepSeek at its own address, any compatible server by its address (Tokenharbor,
// NVIDIA NIM, vLLM), and Ollama at its /v1.

// SettingProvider is the key of the provider in the settings table.
const SettingProvider = "wiki.provider"

// Kinds of provider.
const (
	KindDeepSeek = "deepseek"
	KindOpenAI   = "openai"
	KindOllama   = "ollama"
)

// DeepSeekURL is the address of DeepSeek, used when its kind is chosen without one.
const DeepSeekURL = "https://api.deepseek.com"

// Provider is the setting as stored: the key only sealed, with its last characters to tell it apart.
type Provider struct {
	Kind      string `json:"kind"`
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	KeySealed string `json:"keySealed,omitempty"`
	KeyHint   string `json:"keyHint,omitempty"`
}

// ProviderInput is what the owner sends: the key only when it changes.
type ProviderInput struct {
	Kind     string `json:"kind"`
	BaseURL  string `json:"baseUrl"`
	Model    string `json:"model"`
	Key      string `json:"key"`
	ClearKey bool   `json:"clearKey"`
}

// LoadProvider is the provider of the wiki, or nil when there is none.
func LoadProvider(ctx context.Context, db *sql.DB) (*Provider, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, SettingProvider).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p Provider
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Apply checks what the owner sent and makes the provider to store from it and the one stored before (whose key is kept when no
// new key is sent). master is the master key of the deployment, needed only to keep a new key.
func Apply(in ProviderInput, before *Provider, master string) (*Provider, error) {
	p := &Provider{Kind: strings.TrimSpace(in.Kind), BaseURL: strings.TrimRight(strings.TrimSpace(in.BaseURL), "/"), Model: strings.TrimSpace(in.Model)}
	switch p.Kind {
	case KindDeepSeek:
		if p.BaseURL == "" {
			p.BaseURL = DeepSeekURL
		}
	case KindOpenAI, KindOllama:
		if p.BaseURL == "" {
			return nil, InputError{"Diga o endereço do servidor, como https://exemplo.com/v1."}
		}
	default:
		return nil, InputError{"Escolha o tipo do provedor: DeepSeek, compatível com OpenAI ou Ollama."}
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, InputError{"O endereço do servidor tem de ser http:// ou https://, sem usuário, senha nem parâmetros."}
	}
	if p.Model == "" || len(p.Model) > 128 || strings.ContainsFunc(p.Model, unicode.IsSpace) {
		return nil, InputError{"Diga o modelo, como deepseek-chat, sem espaços (até 128 caracteres)."}
	}
	key := strings.TrimSpace(in.Key)
	switch {
	case key != "":
		if len(key) > 512 || strings.ContainsFunc(key, unicode.IsSpace) {
			return nil, InputError{"A chave não pode ter espaços nem passar de 512 caracteres."}
		}
		sealed, err := Seal(master, key)
		if err != nil {
			return nil, err
		}
		p.KeySealed, p.KeyHint = sealed, hint(key)
	case !in.ClearKey && before != nil:
		p.KeySealed, p.KeyHint = before.KeySealed, before.KeyHint
	}
	if p.Kind == KindDeepSeek && p.KeySealed == "" {
		return nil, InputError{"O DeepSeek pede uma chave de API."}
	}
	return p, nil
}

// hint is the end of a key, to tell it apart: "…4f2a". A short key gives nothing away.
func hint(key string) string {
	if len(key) < 12 {
		return ""
	}
	return "…" + key[len(key)-4:]
}

// SaveProvider stores the provider.
func SaveProvider(ctx context.Context, db *sql.DB, p *Provider, userID string) error {
	value, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_by) VALUES ($1, $2::jsonb, NULLIF($3, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		SettingProvider, value, userID)
	return err
}

// DeleteProvider takes the provider away: no generation starts without one (DEC-177).
func DeleteProvider(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM settings WHERE key = $1`, SettingProvider)
	return err
}

// Remote says whether the text of the books leaves this server for the provider: anything but this machine, the private network,
// a name of the local network or a service of the same Compose (a name with no dot).
func (p *Provider) Remote() bool {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return false
	}
	for _, suffix := range []string{".local", ".lan", ".internal", ".home.arpa", ".localhost"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return true
}

// Host is the name of the server, for saying where the text goes.
func (p *Provider) Host() string {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Policy is what is known of what the provider does with the text it receives, for the owner to read before turning it on
// (DEC-177). Unknown services get the general warning.
func (p *Provider) Policy() string {
	host := p.Host()
	switch {
	case !p.Remote():
		return "O texto dos livros fica na sua rede: o servidor do modelo é desta máquina ou da rede local."
	case strings.HasSuffix(host, "deepseek.com"):
		return "A DeepSeek recebe o texto dos capítulos. Pela política dela, os dados enviados pela API ficam em servidores na China e podem ser guardados conforme a lei de lá."
	case strings.HasSuffix(host, "tokenharbor.ai"):
		return "O Tokenharbor recebe o texto dos capítulos. Nos modelos gratuitos, os termos dele permitem guardar e usar o que é enviado para melhorar o serviço."
	case strings.HasSuffix(host, "nvidia.com"):
		return "A NVIDIA recebe o texto dos capítulos pelo NIM. Confira os termos da conta: o acesso gratuito é para teste e tem limite de pedidos."
	}
	return "Este serviço recebe o texto dos capítulos. Leia a política de dados dele antes de gerar: o Códice não sabe o que ele guarda."
}

// TestResult is the answer to "Testar": whether the model answered, in how long, and what went wrong.
type TestResult struct {
	OK      bool   `json:"ok"`
	Millis  int64  `json:"ms"`
	Reply   string `json:"reply,omitempty"`
	Problem string `json:"problem,omitempty"`
}

// Test asks the model for one short answer. key is the key opened from the seal ("" when there is none); it is never part of what
// is said back, even when the server repeats it in an error.
func Test(ctx context.Context, client *http.Client, p *Provider, key string) TestResult {
	body := map[string]any{
		"model":      p.Model,
		"messages":   []map[string]string{{"role": "user", "content": "Responda apenas: OK"}},
		"max_tokens": 16,
		"stream":     false,
	}
	if p.Kind == KindDeepSeek {
		body["thinking"] = map[string]string{"type": "disabled"}
	}
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return TestResult{Problem: "O endereço do servidor não serve para um pedido."}
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	start := time.Now()
	resp, err := client.Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return TestResult{Millis: ms, Problem: "O servidor não respondeu em 60 segundos."}
		}
		return TestResult{Millis: ms, Problem: netProblem(err, p.Host())}
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return TestResult{Millis: ms, Problem: statusProblem(resp.StatusCode) + detail(text, key)}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(text, &out); err != nil || len(out.Choices) == 0 {
		return TestResult{Millis: ms, Problem: "O servidor respondeu, mas não no formato de chat do OpenAI."}
	}
	reply := strings.TrimSpace(out.Choices[0].Message.Content)
	if len([]rune(reply)) > 80 {
		reply = string([]rune(reply)[:80]) + "…"
	}
	return TestResult{OK: true, Millis: ms, Reply: scrub(reply, key)}
}

// netProblem says why the server could not be reached, in words: the name, the port, the certificate.
func netProblem(err error, host string) string {
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	switch {
	case errors.As(err, &dns):
		return "O nome " + host + " não foi achado: confira o endereço (num contêiner, localhost é o próprio contêiner)."
	case errors.Is(err, syscall.ECONNREFUSED):
		return "Nada atende nesse endereço e porta: o servidor está ligado?"
	case errors.As(err, &cert):
		return "O certificado HTTPS do servidor não é válido."
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "O servidor não respondeu a tempo."
	}
	return "Não foi possível falar com o servidor."
}

func statusProblem(code int) string {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "O servidor recusou a chave."
	case http.StatusNotFound:
		return "O servidor não conhece este endereço ou este modelo."
	case http.StatusPaymentRequired:
		return "O servidor diz que falta saldo na conta."
	case http.StatusTooManyRequests:
		return "O servidor pediu para esperar: limite de pedidos."
	}
	return fmt.Sprintf("O servidor respondeu com o erro %d.", code)
}

// detail is the message of an error answer, short and without the key.
func detail(text []byte, key string) string {
	var e struct {
		Error any `json:"error"`
	}
	msg := ""
	if json.Unmarshal(text, &e) == nil {
		switch v := e.Error.(type) {
		case string:
			msg = v
		case map[string]any:
			msg, _ = v["message"].(string)
		}
	}
	msg = strings.TrimSpace(scrub(msg, key))
	if msg == "" {
		return ""
	}
	if len([]rune(msg)) > 200 {
		msg = string([]rune(msg)[:200]) + "…"
	}
	return " (" + msg + ")"
}

func scrub(text, key string) string {
	if key == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "[chave]")
}
