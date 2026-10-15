package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const master = "0123456789abcdef0123456789abcdef"

func TestSeal_OpensWithTheSameMasterKeyOnly(t *testing.T) {
	sealed, err := Seal(master, "sk-segredo-1234")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "segredo") || !strings.HasPrefix(sealed, "v1:") {
		t.Errorf("sealed = %q", sealed)
	}
	if again, _ := Seal(master, "sk-segredo-1234"); again == sealed {
		t.Error("two seals of the same key are equal: the nonce is not random")
	}
	if got, err := Open(master, sealed); err != nil || got != "sk-segredo-1234" {
		t.Errorf("Open = %q, %v", got, err)
	}
	if _, err := Open(strings.Repeat("x", 32), sealed); !errors.Is(err, ErrUnreadable) {
		t.Errorf("another master key opened it: %v", err)
	}
	tampered := sealed[:len(sealed)-4] + "AAAA"
	if _, err := Open(master, tampered); !errors.Is(err, ErrUnreadable) {
		t.Errorf("a changed seal opened: %v", err)
	}
	for _, short := range []string{"", "curta"} {
		if _, err := Seal(short, "k"); !errors.Is(err, ErrNoMasterKey) {
			t.Errorf("Seal with master %q: %v", short, err)
		}
	}
}

// The worker opens the same seal in Python (worker/wiki_secret.py): this vector is in its tests too.
func TestSeal_KnownVectorForTheWorker(t *testing.T) {
	const vector = "v1:AAECAwQFBgcICQoLEhqiAGK5tK8XIukvvXcM6gmq//yh3OvU"
	if got, err := Open(master, vector); err != nil || got != "sk-teste" {
		t.Errorf("Open(vector) = %q, %v", got, err)
	}
}

func TestApply_ChecksWhatTheOwnerSentAndKeepsTheKeyUnlessItChanges(t *testing.T) {
	p, err := Apply(ProviderInput{Kind: "deepseek", Model: "deepseek-chat", Key: "sk-0123456789abcd"}, nil, master)
	if err != nil {
		t.Fatal(err)
	}
	if p.BaseURL != DeepSeekURL || p.KeyHint != "…abcd" || p.KeySealed == "" {
		t.Errorf("provider = %+v", p)
	}
	kept, err := Apply(ProviderInput{Kind: "deepseek", Model: "deepseek-reasoner"}, p, master)
	if err != nil || kept.KeySealed != p.KeySealed || kept.Model != "deepseek-reasoner" {
		t.Errorf("without a new key = %+v, %v", kept, err)
	}
	if _, err := Apply(ProviderInput{Kind: "deepseek", Model: "deepseek-chat", ClearKey: true}, p, master); err == nil {
		t.Error("DeepSeek was accepted without a key")
	}
	local, err := Apply(ProviderInput{Kind: "ollama", BaseURL: "http://ollama:11434/v1/", Model: "qwen3:32b", ClearKey: true}, p, master)
	if err != nil || local.KeySealed != "" || local.BaseURL != "http://ollama:11434/v1" {
		t.Errorf("Ollama without a key = %+v, %v", local, err)
	}
	if _, err := Apply(ProviderInput{Kind: "deepseek", Model: "m", Key: "sk-0123456789abcd"}, nil, ""); !errors.Is(err, ErrNoMasterKey) {
		t.Errorf("a key without a master key: %v", err)
	}
	bad := map[string]ProviderInput{
		"no kind":            {Model: "m"},
		"an unknown kind":    {Kind: "gpt", Model: "m"},
		"no address":         {Kind: "openai", Model: "m"},
		"an ftp address":     {Kind: "openai", BaseURL: "ftp://x.com", Model: "m"},
		"a user in the URL":  {Kind: "openai", BaseURL: "https://u:p@x.com/v1", Model: "m"},
		"no model":           {Kind: "ollama", BaseURL: "http://ollama:11434/v1"},
		"a model with space": {Kind: "ollama", BaseURL: "http://ollama:11434/v1", Model: "a b"},
		"a key with space":   {Kind: "openai", BaseURL: "https://x.com/v1", Model: "m", Key: "a b"},
	}
	for name, in := range bad {
		var ie InputError
		if _, err := Apply(in, nil, master); !errors.As(err, &ie) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRemote_IsAnythingButThisMachineAndTheLocalNetwork(t *testing.T) {
	for url, want := range map[string]bool{
		"https://api.deepseek.com":          true,
		"https://tokenharbor.ai/v1":         true,
		"http://localhost:11434/v1":         false,
		"http://127.0.0.1:11434/v1":         false,
		"http://192.168.0.10:11434/v1":      false,
		"http://10.1.2.3/v1":                false,
		"http://ollama:11434/v1":            false,
		"http://gpu.lan:8000/v1":            false,
		"http://host.docker.internal:11434": false,
		"http://8.8.8.8/v1":                 true,
	} {
		if got := (&Provider{BaseURL: url}).Remote(); got != want {
			t.Errorf("Remote(%s) = %v", url, got)
		}
	}
}

func TestTest_SaysWhetherTheModelAnsweredAndNeverRepeatsTheKey(t *testing.T) {
	var asked map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		asked = map[string]any{}
		json.NewDecoder(r.Body).Decode(&asked)
		switch r.URL.Path {
		case "/ok/chat/completions":
			w.Write([]byte(`{"choices":[{"message":{"content":" OK "}}]}`))
		case "/denied/chat/completions":
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"Invalid key sk-0123456789abcd"}}`))
		default:
			w.Write([]byte(`<html>not a chat</html>`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	res := Test(ctx, srv.Client(), &Provider{Kind: KindDeepSeek, BaseURL: srv.URL + "/ok", Model: "deepseek-chat"}, "sk-0123456789abcd")
	if !res.OK || res.Reply != "OK" || auth != "Bearer sk-0123456789abcd" || asked["model"] != "deepseek-chat" || asked["thinking"] == nil {
		t.Errorf("ok = %+v, auth %q, asked %v", res, auth, asked)
	}
	res = Test(ctx, srv.Client(), &Provider{Kind: KindOpenAI, BaseURL: srv.URL + "/denied", Model: "m"}, "sk-0123456789abcd")
	if res.OK || !strings.Contains(res.Problem, "recusou a chave") || strings.Contains(res.Problem, "sk-0123") || !strings.Contains(res.Problem, "[chave]") {
		t.Errorf("denied = %+v", res)
	}
	if asked["thinking"] != nil {
		t.Error("a compatible server was sent the DeepSeek parameter")
	}
	if res := Test(ctx, srv.Client(), &Provider{Kind: KindOllama, BaseURL: "http://nao-existe.invalid/v1", Model: "m"}, ""); !strings.Contains(res.Problem, "nao-existe.invalid não foi achado") {
		t.Errorf("an unknown name = %+v", res)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	if res := Test(ctx, srv.Client(), &Provider{Kind: KindOllama, BaseURL: closedURL, Model: "m"}, ""); !strings.Contains(res.Problem, "Nada atende") {
		t.Errorf("a closed port = %+v", res)
	}
	res = Test(ctx, srv.Client(), &Provider{Kind: KindOllama, BaseURL: srv.URL + "/html", Model: "m"}, "")
	if res.OK || !strings.Contains(res.Problem, "formato") || auth != "" {
		t.Errorf("html = %+v, auth %q", res, auth)
	}
}
