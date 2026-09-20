package backup

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// pgConn is a connection target for the PostgreSQL tools. The password travels in the
// environment of the child process only, never on its command line (where any local
// user could read it from the process list).
type pgConn struct {
	env    []string
	dbname string
	url    *url.URL
}

func parseConn(dsn string) (pgConn, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return pgConn{}, fmt.Errorf("DATABASE_URL must be a postgres:// URL")
	}
	c := pgConn{url: u, dbname: strings.TrimPrefix(u.Path, "/")}
	c.env = append(os.Environ(), "PGHOST="+u.Hostname(), "PGDATABASE="+c.dbname)
	if p := u.Port(); p != "" {
		c.env = append(c.env, "PGPORT="+p)
	}
	if user := u.User.Username(); user != "" {
		c.env = append(c.env, "PGUSER="+user)
	}
	if pw, ok := u.User.Password(); ok {
		c.env = append(c.env, "PGPASSWORD="+pw)
	}
	if ssl := u.Query().Get("sslmode"); ssl != "" {
		c.env = append(c.env, "PGSSLMODE="+ssl)
	}
	return c, nil
}

// withDB is the same server and account, another database.
func (c pgConn) withDB(name string) pgConn {
	u := *c.url
	u.Path = "/" + name
	out, _ := parseConn(u.String())
	return out
}

// open returns a database/sql handle for this target.
func (c pgConn) open() (*sql.DB, error) {
	return sql.Open("postgres", c.url.String())
}

// tool finds pg_dump or pg_restore. PG_DUMP and PG_RESTORE override the search.
func tool(name string) (string, error) {
	if v := os.Getenv(strings.ToUpper(name)); v != "" {
		return v, nil
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", ErrNoTool
	}
	return p, nil
}

// run executes a PostgreSQL tool. Its own message goes into the error (it never holds
// the password, which is only in the environment).
func (c pgConn) run(ctx context.Context, name string, stdin io.Reader, stdout io.Writer, args ...string) error {
	bin, err := tool(name)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = c.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = stdout
	cmd.Stdin = stdin
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 800 {
			msg = msg[len(msg)-800:]
		}
		return fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return nil
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// major reads the major version from "16.14", "pg_dump (PostgreSQL) 16.4" or similar.
func major(s string) int {
	for _, f := range strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(s)) {
		var n int
		if _, err := fmt.Sscanf(f, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// toolMajor is the major version of a PostgreSQL client tool.
func toolMajor(ctx context.Context, name string) (int, error) {
	var out bytes.Buffer
	if err := (pgConn{}).run(ctx, name, nil, &out, "--version"); err != nil {
		return 0, err
	}
	return major(out.String()), nil
}
