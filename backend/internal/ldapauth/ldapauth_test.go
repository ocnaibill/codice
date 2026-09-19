package ldapauth_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/ldapauth"
	"github.com/ocnaibill/codice/backend/internal/ldapauth/ldaptest"
)

var ctx = context.Background()

func testConfig(s *ldaptest.Server) ldapauth.Config {
	return ldapauth.Config{
		URL: s.URL(), BindDN: ldaptest.ServiceDN, BindPassword: ldaptest.ServicePassword, BaseDN: ldaptest.BaseDN,
		UserFilter: "(uid={username})", UsernameAttr: "uid", IDAttr: "entryUUID", EmailAttr: "mail",
		AllowPlaintext: true,
	}
}

func client(t *testing.T, s *ldaptest.Server, edit ...func(*ldapauth.Config)) *ldapauth.Client {
	t.Helper()
	cfg := testConfig(s)
	for _, e := range edit {
		e(&cfg)
	}
	c, err := ldapauth.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var ana = ldaptest.User{Name: "ana", Password: "senha-da-ana", Email: "Ana@Example.com", UUID: "uuid-ana"}

func TestConfig_RefusesWhatWouldBeUnsafeOrUnusable(t *testing.T) {
	good := ldapauth.Config{URL: "ldaps://ldap.example", BindDN: "cn=svc", BindPassword: "x", BaseDN: "dc=x",
		UserFilter: "(uid={username})", UsernameAttr: "uid", IDAttr: "entryUUID"}
	if err := good.Validate(); err != nil {
		t.Fatalf("a good config was refused: %v", err)
	}
	for name, edit := range map[string]func(*ldapauth.Config){
		"plain ldap:// without StartTLS": func(c *ldapauth.Config) { c.URL = "ldap://ldap.example"; c.StartTLS = false },
		"a URL that is not ldap":         func(c *ldapauth.Config) { c.URL = "http://ldap.example" },
		"no host":                        func(c *ldapauth.Config) { c.URL = "ldaps://" },
		"no service account":             func(c *ldapauth.Config) { c.BindDN = "" },
		"no service password":            func(c *ldapauth.Config) { c.BindPassword = "" },
		"no base":                        func(c *ldapauth.Config) { c.BaseDN = "" },
		"a filter without the name":      func(c *ldapauth.Config) { c.UserFilter = "(objectClass=*)" },
		"no stable identifier":           func(c *ldapauth.Config) { c.IDAttr = "" },
	} {
		c := good
		edit(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	ok := good
	ok.URL, ok.StartTLS = "ldap://ldap.example", true
	if err := ok.Validate(); err != nil {
		t.Errorf("ldap:// with StartTLS was refused: %v", err)
	}
}

func TestConfigFromEnv_IsOffWithoutAURLAndKeepsTheSecretOutOfTheDefaults(t *testing.T) {
	t.Setenv("LDAP_URL", "")
	if _, on := ldapauth.ConfigFromEnv(); on {
		t.Error("LDAP is on without LDAP_URL")
	}
	t.Setenv("LDAP_URL", "ldaps://ldap.example")
	t.Setenv("LDAP_BIND_PASSWORD", "seg")
	c, on := ldapauth.ConfigFromEnv()
	if !on || c.BindPassword != "seg" || !c.StartTLS || c.IDAttr != "entryUUID" || c.UserFilter != "(uid={username})" {
		t.Errorf("config = %+v", c)
	}
	t.Setenv("LDAP_BIND_PASSWORD", "")
	dir := t.TempDir()
	if err := writeFile(dir+"/pw", "do-arquivo\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LDAP_BIND_PASSWORD_FILE", dir+"/pw")
	if c, _ := ldapauth.ConfigFromEnv(); c.BindPassword != "do-arquivo" {
		t.Errorf("the password file was not read: %q", c.BindPassword)
	}
}

func TestLookup_FindsByNameAndKeepsThePasswordOutOfIt(t *testing.T) {
	s := ldaptest.Start(t, ana)
	c := client(t, s)
	e, err := c.Lookup(ctx, "ana")
	if err != nil || e.Subject != "uuid-ana" || e.Username != "ana" || e.Email != "ana@example.com" || !strings.HasPrefix(e.DN, "uid=ana") {
		t.Fatalf("entry = %+v, err = %v", e, err)
	}
	if _, err := c.Lookup(ctx, "ninguem"); !errors.Is(err, ldapauth.ErrNotFound) {
		t.Errorf("unknown name: %v, want ErrNotFound", err)
	}
	// Looking someone up sends nothing of theirs: the service account does it.
	if s.SawPassword("senha-da-ana") {
		t.Error("the person's password reached the directory during a lookup")
	}
	for _, b := range s.Binds() {
		if !strings.HasPrefix(b, ldaptest.ServiceDN+"|") {
			t.Errorf("a lookup bound as %q", b)
		}
	}
}

func TestLookup_RejectsNamesThatCannotBeLoginNames(t *testing.T) {
	s := ldaptest.Start(t, ana)
	c := client(t, s)
	for _, bad := range []string{"", strings.Repeat("a", 300), "a\x00b", "a\nb"} {
		if _, err := c.Lookup(ctx, bad); !errors.Is(err, ldapauth.ErrNotFound) {
			t.Errorf("name %q: %v, want ErrNotFound", bad, err)
		}
	}
	if n := len(s.Searches()); n != 0 {
		t.Errorf("%d searches were sent for names that are not valid", n)
	}
}

func TestLookup_RefusesToGuessWhenSeveralEntriesMatchOrTheStableIDIsMissing(t *testing.T) {
	s := ldaptest.Start(t,
		ldaptest.User{Name: "a1", Password: "x", Email: "shared@example.com", UUID: "u1"},
		ldaptest.User{Name: "a2", Password: "x", Email: "shared@example.com", UUID: "u2"},
		ldaptest.User{Name: "noid", Password: "x", Email: "n@example.com", UUID: ""})
	byMail := client(t, s, func(c *ldapauth.Config) { c.UserFilter = "(mail={username})" })
	if _, err := byMail.Lookup(ctx, "shared@example.com"); !errors.Is(err, ldapauth.ErrNotFound) {
		t.Errorf("ambiguous match: %v, want ErrNotFound", err)
	}
	if _, err := client(t, s).Lookup(ctx, "noid"); !errors.Is(err, ldapauth.ErrNotFound) {
		t.Errorf("an entry with no stable id: %v, want ErrNotFound", err)
	}
}

func TestAuthenticate_BindsAsThePersonAndNeverAcceptsAnEmptyPassword(t *testing.T) {
	s := ldaptest.Start(t, ana)
	c := client(t, s)
	e, _ := c.Lookup(ctx, "ana")

	if err := c.Authenticate(ctx, e, "senha-da-ana"); err != nil {
		t.Errorf("the right password: %v", err)
	}
	if err := c.Authenticate(ctx, e, "errada"); !errors.Is(err, ldapauth.ErrInvalidCredentials) {
		t.Errorf("a wrong password: %v, want ErrInvalidCredentials", err)
	}
	before := len(s.Binds())
	if err := c.Authenticate(ctx, e, ""); !errors.Is(err, ldapauth.ErrInvalidCredentials) {
		t.Errorf("an empty password: %v, want ErrInvalidCredentials (the directory would accept it as anonymous)", err)
	}
	if len(s.Binds()) != before {
		t.Error("an empty password was even sent to the directory")
	}
	if err := c.Authenticate(ctx, nil, "x"); !errors.Is(err, ldapauth.ErrInvalidCredentials) {
		t.Errorf("no entry: %v", err)
	}
}

func TestLookupBySubject_SurvivesARenameInTheDirectory(t *testing.T) {
	s := ldaptest.Start(t, ana)
	c := client(t, s)
	s.Rename("ana", "ana.silva")
	if _, err := c.Lookup(ctx, "ana"); !errors.Is(err, ldapauth.ErrNotFound) {
		t.Errorf("the old name still resolves: %v", err)
	}
	e, err := c.LookupBySubject(ctx, "uuid-ana")
	if err != nil || e.Username != "ana.silva" {
		t.Fatalf("by subject: %+v %v", e, err)
	}
	s.Remove("ana.silva")
	if _, err := c.LookupBySubject(ctx, "uuid-ana"); !errors.Is(err, ldapauth.ErrNotFound) {
		t.Errorf("a removed entry: %v, want ErrNotFound", err)
	}
}

func TestUnavailableIsNotTheSameAsWrongCredentials(t *testing.T) {
	s := ldaptest.Start(t, ana)
	c := client(t, s)
	e, _ := c.Lookup(ctx, "ana")

	s.SetDown(true)
	if _, err := c.Lookup(ctx, "ana"); !errors.Is(err, ldapauth.ErrUnavailable) {
		t.Errorf("lookup while down: %v, want ErrUnavailable", err)
	}
	if err := c.Authenticate(ctx, e, "senha-da-ana"); !errors.Is(err, ldapauth.ErrUnavailable) || errors.Is(err, ldapauth.ErrInvalidCredentials) {
		t.Errorf("authenticate while down: %v, want ErrUnavailable", err)
	}
	if err := c.Check(ctx); !errors.Is(err, ldapauth.ErrUnavailable) {
		t.Errorf("check while down: %v", err)
	}
	s.SetDown(false)

	badService := client(t, s, func(c *ldapauth.Config) { c.BindPassword = "errada" })
	if _, err := badService.Lookup(ctx, "ana"); !errors.Is(err, ldapauth.ErrUnavailable) {
		t.Errorf("a wrong service password: %v, want ErrUnavailable (a configuration problem, not a user's)", err)
	}
	nowhere := client(t, s, func(c *ldapauth.Config) { c.URL = "ldap://127.0.0.1:1" })
	if _, err := nowhere.Lookup(ctx, "ana"); !errors.Is(err, ldapauth.ErrUnavailable) {
		t.Errorf("an unreachable directory: %v", err)
	}
}

func TestStartTLS_NeverFallsBackToClearText(t *testing.T) {
	s := ldaptest.Start(t, ana) // speaks plain LDAP only
	secure := client(t, s, func(c *ldapauth.Config) { c.StartTLS = true })
	if _, err := secure.Lookup(ctx, "ana"); !errors.Is(err, ldapauth.ErrUnavailable) {
		t.Errorf("StartTLS against a server without it: %v, want a refusal, not a silent plaintext session", err)
	}
	if len(s.Binds()) != 0 {
		t.Errorf("credentials were sent in the clear: %v", s.Binds())
	}
	if err := client(t, s).Check(ctx); err != nil {
		t.Errorf("the explicit plaintext test mode should work: %v", err)
	}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }
