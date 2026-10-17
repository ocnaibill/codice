package mailer

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

// server is an SMTP server of mentira: it keeps what it was told.
type server struct {
	addr     string
	auth     string // the mechanisms it offers
	starttls bool
	implicit bool
	cert     tls.Certificate
	mu       sync.Mutex
	user     string
	password string
	from     string
	rcpt     string
	data     string
	refuse   string // "rcpt", "auth", "mail": what it refuses
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func startServer(t *testing.T, s *server) *server {
	t.Helper()
	s.cert = selfSigned(t)
	var ln net.Listener
	var err error
	if s.implicit {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{s.cert}})
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s.addr = ln.Addr().String()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *server) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(line string) { conn.Write([]byte(line + "\r\n")) }
	secured := s.implicit
	say("220 test ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			say("250-test")
			if s.starttls && !secured {
				say("250-STARTTLS")
			}
			if s.auth != "" && (secured || !s.starttls) {
				say("250-AUTH " + s.auth)
			}
			say("250 8BITMIME")
		case upper == "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}})
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, r, secured = tc, bufio.NewReader(tc), true
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			parts := strings.Split(string(raw), "\x00")
			if s.refuse == "auth" || len(parts) != 3 {
				say("535 5.7.8 bad credentials")
				continue
			}
			s.mu.Lock()
			s.user, s.password = parts[1], parts[2]
			s.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(upper, "AUTH LOGIN"):
			say("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
			u, _ := r.ReadString('\n')
			say("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
			p, _ := r.ReadString('\n')
			du, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(u))
			dp, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(p))
			s.mu.Lock()
			s.user, s.password = string(du), string(dp)
			s.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM"):
			if s.refuse == "mail" {
				say("550 sender refused")
				continue
			}
			s.mu.Lock()
			s.from = line
			s.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(upper, "RCPT TO"):
			if s.refuse == "rcpt" {
				say("550 recipient refused")
				continue
			}
			s.mu.Lock()
			s.rcpt = line
			s.mu.Unlock()
			say("250 ok")
		case upper == "DATA":
			say("354 go on")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				sb.WriteString(l)
			}
			s.mu.Lock()
			s.data = sb.String()
			s.mu.Unlock()
			say("250 queued")
		case upper == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func cfg(s *server, security string) Config {
	host, port, _ := net.SplitHostPort(s.addr)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return Config{Host: host, Port: p, Security: security, From: "codice@casa.example", InsecureTLS: true, Timeout: 10 * time.Second}
}

func book() Message {
	return Message{To: "maria_abc@kindle.com", Subject: "Duna — Frank Herbert", Body: "Segue o livro.\nBom proveito.",
		Attach: &Attachment{Filename: "Duna - Frank Herbert.epub", ContentType: "application/epub+zip", Data: []byte(strings.Repeat("EPUB", 500))}}
}

func (s *server) received(t *testing.T) (*mail.Message, string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	msg, err := mail.ReadMessage(strings.NewReader(s.data))
	if err != nil {
		t.Fatalf("what the server got is not a message: %v\n%s", err, s.data)
	}
	return msg, s.data
}

func TestSend_ThroughEveryWayOfProtectingTheConnection(t *testing.T) {
	for name, c := range map[string]struct {
		server   *server
		security string
	}{
		"none":     {&server{auth: "PLAIN LOGIN"}, SecurityNone},
		"starttls": {&server{auth: "PLAIN", starttls: true}, SecurityStartTLS},
		"tls":      {&server{auth: "PLAIN", implicit: true}, SecurityTLS},
		"login":    {&server{auth: "LOGIN"}, SecurityNone},
	} {
		s := startServer(t, c.server)
		conf := cfg(s, c.security)
		conf.Username, conf.Password = "codice", "s3cret!"
		if err := Send(context.Background(), conf, book()); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		s.mu.Lock()
		user, password, rcpt, from := s.user, s.password, s.rcpt, s.from
		s.mu.Unlock()
		if user != "codice" || password != "s3cret!" || !strings.Contains(rcpt, "maria_abc@kindle.com") || !strings.Contains(from, "codice@casa.example") {
			t.Errorf("%s: user %q password %q rcpt %q from %q", name, user, password, rcpt, from)
		}
		msg, _ := s.received(t)
		if got := msg.Header.Get("To"); got != "maria_abc@kindle.com" {
			t.Errorf("%s: To = %q", name, got)
		}
	}
}

func TestSend_TheBookArrivesAsAnAttachmentWithItsNameAndNothingInTheHeadersThatABookCouldBreak(t *testing.T) {
	s := startServer(t, &server{})
	conf := cfg(s, SecurityNone)
	m := book()
	m.Subject = "Duna\r\nBcc: alguem@x.example"
	m.Attach.Filename = "Título com acentuação — e \"aspas\"\r\nX-Injected: 1.epub"
	if err := Send(context.Background(), conf, m); err != nil {
		t.Fatal(err)
	}
	msg, raw := s.received(t)
	if msg.Header.Get("Bcc") != "" || msg.Header.Get("X-Injected") != "" || strings.Contains(raw, "\r\nBcc:") {
		t.Errorf("a header was added by the title of the book:\n%s", raw)
	}
	if subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); !strings.HasPrefix(subject, "Duna") {
		t.Errorf("subject = %q", subject)
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("%q %v", mediaType, err)
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	text, _ := reader.NextPart()
	body := new(strings.Builder)
	readAll(text, body, text.Header.Get("Content-Transfer-Encoding"))
	if !strings.Contains(body.String(), "Segue o livro.") {
		t.Errorf("text part = %q", body.String())
	}
	file, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	name := file.FileName()
	if !strings.HasPrefix(name, "Título com acentuação") || !strings.HasSuffix(strings.TrimSpace(name), ".epub") {
		t.Errorf("file name = %q", name)
	}
	data := new(strings.Builder)
	readAll(file, data, file.Header.Get("Content-Transfer-Encoding"))
	if data.String() != strings.Repeat("EPUB", 500) {
		t.Errorf("the book came back different: %d bytes", data.Len())
	}
}

func readAll(r interface{ Read([]byte) (int, error) }, w *strings.Builder, encoding string) {
	buf := new(strings.Builder)
	b := make([]byte, 4096)
	for {
		n, err := r.Read(b)
		buf.Write(b[:n])
		if err != nil {
			break
		}
	}
	text := buf.String()
	if strings.EqualFold(encoding, "base64") {
		raw, _ := base64.StdEncoding.DecodeString(strings.NewReplacer("\r", "", "\n", "").Replace(text))
		text = string(raw)
	}
	w.WriteString(text)
}

func TestSend_WhatTheServerRefusesIsSaidWithoutThePassword(t *testing.T) {
	for _, c := range []struct{ refuse, want string }{{"auth", "user or the password"}, {"mail", "sender"}, {"rcpt", "recipient"}} {
		s := startServer(t, &server{auth: "PLAIN", refuse: c.refuse})
		conf := cfg(s, SecurityNone)
		conf.Username, conf.Password = "codice", "s3cret!"
		err := Send(context.Background(), conf, book())
		if err == nil || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), "s3cret!") {
			t.Errorf("%s: %v", c.refuse, err)
		}
	}
	// no STARTTLS offered when it was asked, and a server that is not there
	s := startServer(t, &server{})
	if err := Send(context.Background(), cfg(s, SecurityStartTLS), book()); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("starttls: %v", err)
	}
	gone := Config{Host: "127.0.0.1", Port: 1, Security: SecurityNone, From: "a@b.example", Timeout: 2 * time.Second}
	if err := Send(context.Background(), gone, book()); err == nil || !strings.Contains(err.Error(), "could not reach") {
		t.Errorf("unreachable: %v", err)
	}
}

func TestSend_ARefusedCertificateIsOnlyAcceptedWhenTheOwnerSaidSo(t *testing.T) {
	s := startServer(t, &server{implicit: true})
	conf := cfg(s, SecurityTLS)
	conf.InsecureTLS = false
	if err := Send(context.Background(), conf, book()); err == nil {
		t.Error("a certificate that is not trusted was accepted")
	}
}

func TestAddressAndConfig(t *testing.T) {
	for _, ok := range []string{"maria@kindle.com", "maria_x1y2@kindle.com", "a.b+c@free.kindle.com"} {
		if got, err := Address(ok); err != nil || got != ok {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "maria", "maria@", "@kindle.com", "a b@kindle.com", "a@b", "a@b.com\r\nBcc: x@y.com", "Maria <a@b.com>", "a@b.com, c@d.com", "a@b.com;c@d.com"} {
		if _, err := Address(bad); err == nil {
			t.Errorf("%q is not an address", bad)
		}
	}
	good := Config{Host: "smtp.example", Port: 587, Security: SecurityStartTLS, From: "a@b.example"}
	if err := good.Validate(); err != nil {
		t.Error(err)
	}
	for _, bad := range []Config{{Port: 587, Security: SecurityTLS, From: "a@b.example"}, {Host: "h", Port: 0, Security: SecurityTLS, From: "a@b.example"},
		{Host: "h", Port: 25, Security: "ssl", From: "a@b.example"}, {Host: "h", Port: 25, Security: SecurityNone, From: "nope"}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v is not a configuration", bad)
		}
	}
}
