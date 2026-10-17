package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/mailer/mailertest"
)

// mailOn points the instance at a fake SMTP server and returns it.
func (s *catalogStack) mailOn(maxMB int) *mailertest.Server {
	s.t.Helper()
	srv := mailertest.Start(s.t)
	body := fmt.Sprintf(`{"enabled":true,"host":%q,"port":%d,"security":"none","from":"codice@casa.lan","password":"segredo-do-smtp","maxMB":%d}`, srv.Host, srv.Port, maxMB)
	if rec := s.do(admin, "PUT", "/admin/mail", body); rec.Code != 200 {
		s.t.Fatalf("PUT /admin/mail: %d %s", rec.Code, rec.Body.String())
	}
	return srv
}

// bookOnDisk makes a work whose managed file really exists.
func (s *catalogStack) bookOnDisk(title, author, name, format string, data []byte) int {
	s.t.Helper()
	if err := os.WriteFile(filepath.Join(s.storage, name), data, 0o600); err != nil {
		s.t.Fatal(err)
	}
	return s.addWork(title, author, name, format)
}

func TestKindle_TheMailServerIsTheOwnersAndTheSecretNeverComesBack(t *testing.T) {
	s := newCatalogStack(t)
	// off until the owner says otherwise (DEC-059)
	rec := s.do(admin, "GET", "/admin/mail", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("default: %d %s", rec.Code, rec.Body.String())
	}
	s.mailOn(25)
	rec = s.do(admin, "GET", "/admin/mail", "")
	if strings.Contains(rec.Body.String(), "segredo-do-smtp") || strings.Contains(rec.Body.String(), "passwordSealed") || !strings.Contains(rec.Body.String(), `"hasPassword":true`) {
		t.Errorf("the password must not come back: %s", rec.Body.String())
	}
	if raw := s.scalar(`SELECT value::text FROM settings WHERE key = 'mail.smtp'`); strings.Contains(raw, "segredo-do-smtp") {
		t.Errorf("the password is kept in the clear: %s", raw)
	}
	if got := s.scalar(`SELECT count(*)::text FROM audit_log WHERE action = 'mail.configure' AND details::text LIKE '%segredo%'`); got != "0" {
		t.Error("the audit entry holds the password")
	}
	// saving again without a password keeps the one kept
	if rec := s.do(admin, "PUT", "/admin/mail", `{"enabled":true,"host":"127.0.0.1","port":2525,"security":"none","from":"codice@casa.lan"}`); rec.Code != 200 {
		t.Fatalf("resave: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(s.do(admin, "GET", "/admin/mail", "").Body.String(), `"hasPassword":true`) {
		t.Error("a save with no password took the password away")
	}
	// a server that is turned on has to be complete, and the size is bounded
	if rec := s.do(admin, "PUT", "/admin/mail", `{"enabled":true,"host":"","port":587,"security":"starttls","from":"x"}`); rec.Code != 400 {
		t.Errorf("incomplete server: %d", rec.Code)
	}
	if rec := s.do(admin, "PUT", "/admin/mail", `{"enabled":false,"maxMB":80}`); rec.Code != 400 {
		t.Errorf("size over 50: %d", rec.Code)
	}
}

func TestKindle_TheOwnerTestsTheServer(t *testing.T) {
	s := newCatalogStack(t)
	srv := s.mailOn(25)
	if rec := s.do(admin, "POST", "/admin/mail/test", `{"to":"eu@casa.lan"}`); rec.Code != 200 {
		t.Fatalf("test: %d %s", rec.Code, rec.Body.String())
	}
	if got := srv.Recipients(); len(got) != 1 || !strings.Contains(got[0], "eu@casa.lan") {
		t.Errorf("recipients = %v", got)
	}
	if rec := s.do(admin, "POST", "/admin/mail/test", `{"to":"not an address"}`); rec.Code != 400 {
		t.Errorf("bad address: %d", rec.Code)
	}
	srv.Refuse = true
	if rec := s.do(admin, "POST", "/admin/mail/test", `{"to":"eu@casa.lan"}`); rec.Code != 502 {
		t.Errorf("a server that refuses: %d %s", rec.Code, rec.Body.String())
	}
}

func TestKindle_ABookGoesToTheAddressOfWhoAsksAsAnAttachment(t *testing.T) {
	s := newCatalogStack(t)
	srv := s.mailOn(25)
	epub := []byte("PK\x03\x04 epub de mentira")
	work := s.bookOnDisk("O Nome da Rosa", "Umberto Eco", "rosa.epub", "epub", epub)
	url := fmt.Sprintf("/works/%d/send-to-kindle", work)

	// without the mail server, without an address: each says what is missing
	if rec := s.do(ana, "POST", url, `{}`); rec.Code != 409 {
		t.Fatalf("no address: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.do(ana, "PUT", "/auth/kindle", `{"email":"nao e endereco"}`); rec.Code != 400 {
		t.Errorf("bad address kept: %d", rec.Code)
	}
	if rec := s.do(ana, "PUT", "/auth/kindle", `{"email":" ana_kindle@kindle.com "}`); rec.Code != 200 {
		t.Fatalf("PUT address: %d %s", rec.Code, rec.Body.String())
	}
	rec := s.do(ana, "GET", "/auth/kindle", "")
	var got struct {
		Email     string
		Available bool
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Email != "ana_kindle@kindle.com" || !got.Available {
		t.Errorf("GET /auth/kindle = %+v", got)
	}
	// another person has no address of ana's
	if rec := s.do(bob, "POST", url, `{}`); rec.Code != 409 {
		t.Errorf("bob has no address: %d", rec.Code)
	}

	rec = s.do(ana, "POST", url, `{}`)
	if rec.Code != 200 {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	if r := srv.Recipients(); len(r) != 1 || !strings.Contains(r[0], "ana_kindle@kindle.com") {
		t.Fatalf("recipients = %v", r)
	}
	msg := srv.Messages()[0]
	if !strings.Contains(msg, `filename="O Nome da Rosa - Umberto Eco.epub"`) && !strings.Contains(msg, "O Nome da Rosa - Umberto Eco.epub") {
		t.Errorf("file name not in the message:\n%.600s", msg)
	}
	if !strings.Contains(msg, "application/epub+zip") {
		t.Error("content type of the attachment")
	}
	if got := s.scalar(`SELECT count(*)::text FROM audit_log WHERE action = 'kindle.send' AND details::text LIKE '%kindle.com%'`); got != "0" {
		t.Error("the audit entry holds the address")
	}
	if got := s.scalar(`SELECT count(*)::text FROM audit_log WHERE action = 'kindle.send'`); got != "1" {
		t.Errorf("audit entries = %s", got)
	}
}

func TestKindle_OnlyWhatAKindleTakesAndWhatFitsIsSent(t *testing.T) {
	s := newCatalogStack(t)
	srv := s.mailOn(1)
	s.exec(`UPDATE users SET kindle_email = 'ana@kindle.com' WHERE id = $1`, idAna)

	cbz := s.bookOnDisk("Quadrinho", "Fulano", "q.cbz", "cbz", []byte("zip"))
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/send-to-kindle", cbz), `{}`); rec.Code != 422 {
		t.Errorf("a CBZ is not sent: %d %s", rec.Code, rec.Body.String())
	}
	big := s.bookOnDisk("Enorme", "Fulano", "big.pdf", "pdf", make([]byte, 2<<20))
	rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/send-to-kindle", big), `{}`)
	if rec.Code != 413 || !strings.Contains(rec.Body.String(), "limite") {
		t.Errorf("over the limit: %d %s", rec.Code, rec.Body.String())
	}
	if rec := s.do(ana, "POST", "/works/99999/send-to-kindle", `{}`); rec.Code != 404 {
		t.Errorf("no such work: %d", rec.Code)
	}
	// a file that is not one of the work's
	pdf := s.bookOnDisk("Pequeno", "Fulano", "small.pdf", "pdf", []byte("%PDF-1.4"))
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/send-to-kindle", pdf), fmt.Sprintf(`{"fileId":%d}`, s.primaryFile(cbz))); rec.Code != 400 {
		t.Errorf("a file of another work: %d", rec.Code)
	}
	if rec := s.do(ana, "POST", fmt.Sprintf("/works/%d/send-to-kindle", pdf), `{}`); rec.Code != 200 {
		t.Errorf("a PDF goes: %d %s", rec.Code, rec.Body.String())
	}
	if len(srv.Messages()) != 1 {
		t.Errorf("messages = %d", len(srv.Messages()))
	}
}

func TestKindle_WithoutTheServerOrTheDailyAllowanceItSaysSo(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`UPDATE users SET kindle_email = 'ana@kindle.com' WHERE id = $1`, idAna)
	work := s.bookOnDisk("Livro", "Fulano", "l.epub", "epub", []byte("x"))
	url := fmt.Sprintf("/works/%d/send-to-kindle", work)
	if rec := s.do(ana, "POST", url, `{}`); rec.Code != 503 {
		t.Errorf("mail off: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(s.do(ana, "GET", "/auth/kindle", "").Body.String(), `"available":false`) {
		t.Error("the page must know sending is off")
	}
	s.mailOn(25)
	for i := 0; i < kindleDailyLimit; i++ {
		s.exec(`INSERT INTO audit_log (actor_id, actor_username, action, target_type, target_id) VALUES ($1, 'ana', 'kindle.send', 'work', '1')`, idAna)
	}
	if rec := s.do(ana, "POST", url, `{}`); rec.Code != 429 {
		t.Errorf("daily allowance: %d %s", rec.Code, rec.Body.String())
	}
}
