// Package ldapauth talks to an LDAP directory (DEC-072 to DEC-075). The directory
// is only ever asked two things: who is this person (a search with the service
// account, which never sees the password) and does this password belong to that
// entry (a bind as the person). It requires LDAPS or StartTLS (spec 11.2), uses
// timeouts on everything, escapes every value that goes into a filter, and never
// treats an empty password as a login: many servers accept an empty simple bind
// as an anonymous, "successful" one.
package ldapauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// Provider is the name under which LDAP identities are stored.
const Provider = "ldap"

var (
	// ErrNotFound means the directory has no (single, usable) entry for the query.
	ErrNotFound = errors.New("no such entry in the directory")
	// ErrInvalidCredentials means the directory refused the password.
	ErrInvalidCredentials = errors.New("the directory refused the credentials")
	// ErrUnavailable means the directory could not answer: an operational state,
	// distinct from a wrong password (spec 11.2).
	ErrUnavailable = errors.New("the directory is unavailable")
)

// Config describes the connection and how entries map to accounts.
type Config struct {
	URL          string // ldaps://host:636 or ldap://host:389 (then StartTLS)
	BindDN       string // service account, read-only
	BindPassword string // never logged, never stored in the database
	BaseDN       string // where users are searched
	// UserFilter finds a person by the name typed at login; {username} is replaced
	// by the escaped name, e.g. (uid={username}).
	UserFilter   string
	UsernameAttr string // attribute holding the login name, e.g. uid
	IDAttr       string // stable identifier, e.g. entryUUID; never the name
	EmailAttr    string // e.g. mail
	StartTLS     bool   // required for ldap:// unless AllowPlaintext
	CAFile       string // extra CA bundle, for a private CA
	Timeout      time.Duration
	// AllowPlaintext lets ldap:// run without StartTLS. For a test directory on
	// the same machine only; it is not read from the environment.
	AllowPlaintext bool
}

// ConfigFromEnv reads the connection from LDAP_* variables. It reports false when
// LDAP_URL is unset, which means the feature is off. The secret comes from
// LDAP_BIND_PASSWORD or from the file named by LDAP_BIND_PASSWORD_FILE.
func ConfigFromEnv() (Config, bool) {
	get := func(k, def string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		return def
	}
	if get("LDAP_URL", "") == "" {
		return Config{}, false
	}
	c := Config{
		URL:          get("LDAP_URL", ""),
		BindDN:       get("LDAP_BIND_DN", ""),
		BindPassword: os.Getenv("LDAP_BIND_PASSWORD"),
		BaseDN:       get("LDAP_BASE_DN", ""),
		UserFilter:   get("LDAP_USER_FILTER", "(uid={username})"),
		UsernameAttr: get("LDAP_USERNAME_ATTR", "uid"),
		IDAttr:       get("LDAP_ID_ATTR", "entryUUID"),
		EmailAttr:    get("LDAP_EMAIL_ATTR", "mail"),
		StartTLS:     get("LDAP_START_TLS", "true") != "false",
		CAFile:       get("LDAP_CA_FILE", ""),
		Timeout:      10 * time.Second,
	}
	if file := get("LDAP_BIND_PASSWORD_FILE", ""); file != "" {
		if raw, err := os.ReadFile(file); err == nil {
			c.BindPassword = strings.TrimRight(string(raw), "\r\n")
		} else {
			log.Printf("LDAP: cannot read LDAP_BIND_PASSWORD_FILE: %v", err)
		}
	}
	return c, true
}

// Validate refuses a configuration that would be unsafe or unusable.
func (c Config) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "ldap" && u.Scheme != "ldaps") || u.Host == "" {
		return errors.New("LDAP_URL must be ldaps://host or ldap://host")
	}
	if u.Scheme == "ldap" && !c.StartTLS && !c.AllowPlaintext {
		return errors.New("LDAP over plain ldap:// needs StartTLS (LDAP_START_TLS=true) or an ldaps:// URL: credentials must not travel in the clear")
	}
	if c.BindDN == "" || c.BindPassword == "" {
		return errors.New("LDAP_BIND_DN and LDAP_BIND_PASSWORD (or _FILE) are required: the service account looks people up")
	}
	if c.BaseDN == "" {
		return errors.New("LDAP_BASE_DN is required")
	}
	if !strings.Contains(c.UserFilter, "{username}") {
		return errors.New("LDAP_USER_FILTER must contain {username}")
	}
	if c.IDAttr == "" || c.UsernameAttr == "" {
		return errors.New("LDAP_ID_ATTR and LDAP_USERNAME_ATTR are required")
	}
	return nil
}

// Entry is a person in the directory.
type Entry struct {
	DN       string
	Subject  string // the stable identifier: what an account is linked to
	Username string
	Email    string
}

// Directory is what the sign-in needs from LDAP. The real implementation is
// Client; tests can supply their own.
type Directory interface {
	// Lookup finds the entry with this login name, using the service account.
	// The person's password is not involved.
	Lookup(ctx context.Context, username string) (*Entry, error)
	// LookupBySubject finds an entry by its stable identifier, for accounts that
	// are already linked (a rename in the directory must not lose the account).
	LookupBySubject(ctx context.Context, subject string) (*Entry, error)
	// Authenticate checks the password by binding as the entry.
	Authenticate(ctx context.Context, entry *Entry, password string) error
	// Check verifies the connection and the service account.
	Check(ctx context.Context) error
}

// Client is the LDAP implementation of Directory.
type Client struct{ cfg Config }

// New validates the configuration and returns a client.
func New(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{cfg: cfg}, nil
}

// Host is where the directory is, for showing to the owner; it never includes a secret.
func (c *Client) Host() string {
	u, _ := url.Parse(c.cfg.URL)
	if u == nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// BaseDN is the search base, for showing to the owner.
func (c *Client) BaseDN() string { return c.cfg.BaseDN }

func (c *Client) tlsConfig(host string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	if c.cfg.CAFile != "" {
		pem, err := os.ReadFile(c.cfg.CAFile)
		if err != nil {
			return nil, err
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("LDAP_CA_FILE holds no certificate")
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

// dial opens a connection with timeouts and the transport security the
// configuration demands.
func (c *Client) dial(ctx context.Context) (*ldap.Conn, error) {
	u, _ := url.Parse(c.cfg.URL)
	tlsCfg, err := c.tlsConfig(u.Hostname())
	if err != nil {
		log.Printf("LDAP: TLS configuration: %v", err)
		return nil, ErrUnavailable
	}
	dialer := &net.Dialer{Timeout: c.cfg.Timeout}
	conn, err := ldap.DialURL(c.cfg.URL, ldap.DialWithDialer(dialer), ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	conn.SetTimeout(c.cfg.Timeout)
	if u.Scheme == "ldap" && c.cfg.StartTLS {
		if err := conn.StartTLS(tlsCfg); err != nil {
			conn.Close()
			return nil, fmt.Errorf("%w: StartTLS: %v", ErrUnavailable, err)
		}
	}
	return conn, nil
}

// serviceConn is a connection bound as the service account.
func (c *Client) serviceConn(ctx context.Context) (*ldap.Conn, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Bind(c.cfg.BindDN, c.cfg.BindPassword); err != nil {
		conn.Close()
		// A service account that cannot sign in is a configuration problem, not a
		// wrong password from a user, so it is reported as the directory being unusable.
		log.Printf("LDAP: the service account could not bind: %v", err)
		return nil, fmt.Errorf("%w: service account", ErrUnavailable)
	}
	return conn, nil
}

func (c *Client) attrs() []string {
	set := []string{c.cfg.IDAttr, c.cfg.UsernameAttr}
	if c.cfg.EmailAttr != "" {
		set = append(set, c.cfg.EmailAttr)
	}
	return set
}

func (c *Client) search(ctx context.Context, filter string) (*Entry, error) {
	conn, err := c.serviceConn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	res, err := conn.Search(ldap.NewSearchRequest(c.cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, int(c.cfg.Timeout.Seconds()), false, filter, c.attrs(), nil))
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			log.Printf("LDAP: more than one entry matches %s; refusing to guess", filter)
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: search: %v", ErrUnavailable, err)
	}
	switch len(res.Entries) {
	case 0:
		return nil, ErrNotFound
	case 1:
	default:
		log.Printf("LDAP: more than one entry matches %s; refusing to guess", filter)
		return nil, ErrNotFound
	}
	e := res.Entries[0]
	entry := &Entry{
		DN:       e.DN,
		Subject:  e.GetAttributeValue(c.cfg.IDAttr),
		Username: e.GetAttributeValue(c.cfg.UsernameAttr),
	}
	if c.cfg.EmailAttr != "" {
		entry.Email = strings.ToLower(strings.TrimSpace(e.GetAttributeValue(c.cfg.EmailAttr)))
	}
	if entry.Subject == "" {
		// Without a stable identifier an account could only be tied to a name, which
		// is exactly what the design forbids.
		log.Printf("LDAP: the entry %q has no %q attribute; check LDAP_ID_ATTR", entry.DN, c.cfg.IDAttr)
		return nil, ErrNotFound
	}
	return entry, nil
}

// userFilter puts the typed name into the configured filter. The name is escaped
// (RFC 4515), so it can only ever be a value: it cannot add a clause, widen the
// match with a wildcard or close the filter early.
func userFilter(template, username string) string {
	return strings.ReplaceAll(template, "{username}", ldap.EscapeFilter(username))
}

func subjectFilter(idAttr, subject string) string {
	return "(" + idAttr + "=" + ldap.EscapeFilter(subject) + ")"
}

func validName(s string) bool { return s != "" && len(s) <= 256 && !strings.ContainsAny(s, "\x00\r\n") }

// Lookup implements Directory.
func (c *Client) Lookup(ctx context.Context, username string) (*Entry, error) {
	if !validName(username) {
		return nil, ErrNotFound
	}
	return c.search(ctx, userFilter(c.cfg.UserFilter, username))
}

// LookupBySubject implements Directory.
func (c *Client) LookupBySubject(ctx context.Context, subject string) (*Entry, error) {
	if !validName(subject) {
		return nil, ErrNotFound
	}
	return c.search(ctx, subjectFilter(c.cfg.IDAttr, subject))
}

// Authenticate implements Directory.
func (c *Client) Authenticate(ctx context.Context, entry *Entry, password string) error {
	// An empty password would be an "unauthenticated bind", which a directory may
	// answer with success. It is never a login.
	if password == "" || entry == nil || entry.DN == "" {
		return ErrInvalidCredentials
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Bind(entry.DN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: bind: %v", ErrUnavailable, err)
	}
	return nil
}

// Check implements Directory.
func (c *Client) Check(ctx context.Context) error {
	conn, err := c.serviceConn(ctx)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
