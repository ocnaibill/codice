// Package mailer sends one e-mail with an attachment through the SMTP server the owner configured. It is for the books a person sends to their
// own e-reader (a Kindle takes them by an address of its own); nothing essential depends on it (DEC-059).
package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Security says how the connection to the server is protected.
const (
	SecurityStartTLS = "starttls" // plain connection that is upgraded (port 587)
	SecurityTLS      = "tls"      // protected from the start (port 465)
	SecurityNone     = "none"     // not protected: only for a server on the same machine or the same network
)

// Config is the server.
type Config struct {
	Host     string
	Port     int
	Security string
	Username string
	Password string
	From     string
	// InsecureTLS accepts a certificate that is not trusted (a server of the house with its own).
	InsecureTLS bool
	// Timeout is how long the whole send may take; 90 seconds when zero.
	Timeout time.Duration
}

// Attachment is the file that goes with the message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is what is sent.
type Message struct {
	To      string
	Subject string
	Body    string
	Attach  *Attachment
}

var (
	// ErrConfig says the server is not set up enough to send.
	ErrConfig = errors.New("mail server is not configured")
	// ErrAddress says an address is not one.
	ErrAddress = errors.New("not an e-mail address")
)

// Address checks that a text is one e-mail address, with nothing that could add a header.
func Address(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "\r\n<>,;\" ") {
		return "", ErrAddress
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || !strings.Contains(a.Address[strings.LastIndex(a.Address, "@"):], ".") {
		return "", ErrAddress
	}
	return s, nil
}

// Validate says whether the configuration can send.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" || c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("%w: host and port", ErrConfig)
	}
	switch c.Security {
	case SecurityStartTLS, SecurityTLS, SecurityNone:
	default:
		return fmt.Errorf("%w: security is starttls, tls or none", ErrConfig)
	}
	if _, err := Address(c.From); err != nil {
		return fmt.Errorf("%w: the sender is not an address", ErrConfig)
	}
	return nil
}

// Compose writes the message as the bytes that go to the server: text and, when there is one, the attachment in base64. Names and the
// subject are encoded, so that what a book is called cannot break a header.
func Compose(from string, m Message, now time.Time, boundary string) []byte {
	var b bytes.Buffer
	clean := func(s string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nDate: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", clean(from), clean(m.To),
		now.Format(time.RFC1123Z), mime.QEncoding.Encode("utf-8", clean(m.Subject)))
	body := strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n")
	if m.Attach == nil {
		fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		writeBase64(&b, []byte(body))
		return b.Bytes()
	}
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", boundary)
	writeBase64(&b, []byte(body))
	name := clean(m.Attach.Filename)
	ctype := m.Attach.ContentType
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	// the name goes as RFC 2231 says (a name with accents becomes filename*=utf-8''...), and what cannot be written is left out
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disposition == "" {
		disposition = "attachment"
	}
	typed := mime.FormatMediaType(clean(ctype), map[string]string{"name": name})
	if typed == "" {
		typed = "application/octet-stream"
	}
	fmt.Fprintf(&b, "\r\n--%s\r\nContent-Type: %s\r\nContent-Disposition: %s\r\nContent-Transfer-Encoding: base64\r\n\r\n", boundary, typed, disposition)
	writeBase64(&b, m.Attach.Data)
	fmt.Fprintf(&b, "\r\n--%s--\r\n", boundary)
	return b.Bytes()
}

func writeBase64(w io.Writer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		io.WriteString(w, enc[:76]+"\r\n")
		enc = enc[76:]
	}
	io.WriteString(w, enc)
}

// loginAuth is the AUTH LOGIN some servers offer in the place of PLAIN.
type loginAuth struct{ username, password string }

func (a loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }
func (a loginAuth) Next(from []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(from))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, errors.New("unexpected question from the server")
}

// Send connects, logs in if there is a user, and sends the message. The error says what failed without the password.
func Send(ctx context.Context, c Config, m Message) error {
	if err := c.Validate(); err != nil {
		return err
	}
	to, err := Address(m.To)
	if err != nil {
		return err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	tlsConfig := &tls.Config{ServerName: c.Host, InsecureSkipVerify: c.InsecureTLS, MinVersion: tls.VersionTLS12} // #nosec G402: the owner chose it
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	if c.Security == SecurityTLS {
		conn = tls.Client(conn, tlsConfig)
	}
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("the server did not answer as a mail server: %w", err)
	}
	defer client.Close()
	if c.Security == SecurityStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("the server does not offer STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("could not protect the connection: %w", err)
		}
	}
	if c.Username != "" {
		if ok, advertised := client.Extension("AUTH"); !ok {
			return errors.New("the server does not offer to log in")
		} else {
			var auth smtp.Auth
			if strings.Contains(strings.ToUpper(advertised), "PLAIN") {
				auth = plainAuth{c.Username, c.Password}
			} else if strings.Contains(strings.ToUpper(advertised), "LOGIN") {
				auth = loginAuth{c.Username, c.Password}
			} else {
				return errors.New("the server logs in in a way this program does not know: " + advertised)
			}
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("the server refused the user or the password: %w", err)
			}
		}
	}
	if err := client.Mail(c.From); err != nil {
		return fmt.Errorf("the server refused the sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("the server refused the recipient: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Compose(c.From, Message{To: to, Subject: m.Subject, Body: m.Body, Attach: m.Attach}, time.Now(), fmt.Sprintf("codice-%d", time.Now().UnixNano()))); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("the server did not accept the message: %w", err)
	}
	return client.Quit()
}

// plainAuth is AUTH PLAIN without the check of net/smtp that refuses it on a connection that is not protected: the owner chose "none".
type plainAuth struct{ username, password string }

func (a plainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
}
func (a plainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected question from the server")
	}
	return nil, nil // the server has said yes
}
