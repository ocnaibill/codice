// Package ldaptest is a small in-process LDAP directory for tests. It behaves
// like a real one where it matters to security: a simple bind with an empty
// password "succeeds" (an unauthenticated bind), searches need a bound account,
// and it records every bind so a test can see what the directory was sent.
package ldaptest

import (
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/glauth/ldap"
)

const (
	BaseDN          = "dc=test"
	ServiceDN       = "cn=svc,dc=test"
	ServicePassword = "svc-pass"
)

// User is one entry.
type User struct {
	Name, Password, Email, UUID string
}

func (u User) dn() string { return "uid=" + u.Name + ",ou=people," + BaseDN }

type Server struct {
	t        testing.TB
	srv      *ldap.Server
	ln       net.Listener
	mu       sync.Mutex
	users    map[string]User
	binds    []string // "dn|password" of every simple bind
	searches []string // filters received
	down     bool
	Addr     string
}

// Start runs a directory on a free local port until the test ends.
func Start(t testing.TB, users ...User) *Server {
	t.Helper()
	s := &Server{t: t, users: map[string]User{}}
	for _, u := range users {
		s.users[u.Name] = u
	}
	s.srv = ldap.NewServer()
	s.srv.BindFunc("", s)
	s.srv.SearchFunc("", s)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	s.Addr = ln.Addr().String()
	go s.srv.Serve(ln)
	t.Cleanup(func() { ln.Close(); s.srv.Close() })
	return s
}

func (s *Server) URL() string { return "ldap://" + s.Addr }

func (s *Server) Set(u User)         { s.mu.Lock(); s.users[u.Name] = u; s.mu.Unlock() }
func (s *Server) Remove(name string) { s.mu.Lock(); delete(s.users, name); s.mu.Unlock() }
func (s *Server) SetDown(down bool)  { s.mu.Lock(); s.down = down; s.mu.Unlock() }

// Rename keeps the entry (and its UUID) but changes the login name.
func (s *Server) Rename(from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[from]
	delete(s.users, from)
	u.Name = to
	s.users[to] = u
}

// Binds returns "dn|password" for every bind received so far.
func (s *Server) Binds() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.binds...)
}

// SawPassword reports whether any bind carried this password.
func (s *Server) SawPassword(pw string) bool {
	for _, b := range s.Binds() {
		if strings.HasSuffix(b, "|"+pw) {
			return true
		}
	}
	return false
}

func (s *Server) Bind(bindDN, pw string, _ net.Conn) (ldap.LDAPResultCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return ldap.LDAPResultUnavailable, nil
	}
	s.binds = append(s.binds, bindDN+"|"+pw)
	if pw == "" {
		return ldap.LDAPResultSuccess, nil // the unauthenticated bind trap
	}
	if bindDN == ServiceDN {
		if pw == ServicePassword {
			return ldap.LDAPResultSuccess, nil
		}
		return ldap.LDAPResultInvalidCredentials, nil
	}
	for _, u := range s.users {
		if strings.EqualFold(u.dn(), bindDN) && u.Password == pw {
			return ldap.LDAPResultSuccess, nil
		}
	}
	return ldap.LDAPResultInvalidCredentials, nil
}

func (s *Server) Search(boundDN string, req ldap.SearchRequest, _ net.Conn) (ldap.ServerSearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return ldap.ServerSearchResult{ResultCode: ldap.LDAPResultUnavailable}, nil
	}
	s.searches = append(s.searches, req.Filter)
	if boundDN == "" {
		return ldap.ServerSearchResult{ResultCode: ldap.LDAPResultInsufficientAccessRights}, nil
	}
	filter, err := ldap.CompileFilter(req.Filter)
	if err != nil {
		return ldap.ServerSearchResult{ResultCode: ldap.LDAPResultOperationsError}, nil
	}
	var out []*ldap.Entry
	for _, u := range s.users {
		e := &ldap.Entry{DN: u.dn(), Attributes: []*ldap.EntryAttribute{
			{Name: "uid", Values: []string{u.Name}},
			{Name: "cn", Values: []string{u.Name}},
			{Name: "entryUUID", Values: []string{u.UUID}},
			{Name: "mail", Values: []string{u.Email}},
			{Name: "objectClass", Values: []string{"person"}},
		}}
		if ok, _ := ldap.ServerApplyFilter(filter, e); ok {
			out = append(out, e)
		}
	}
	if req.SizeLimit > 0 && len(out) > req.SizeLimit {
		return ldap.ServerSearchResult{Entries: out[:req.SizeLimit], ResultCode: ldap.LDAPResultSizeLimitExceeded}, nil
	}
	return ldap.ServerSearchResult{Entries: out, ResultCode: ldap.LDAPResultSuccess}, nil
}

// Searches returns the filters received so far.
func (s *Server) Searches() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.searches...)
}
