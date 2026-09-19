// Package admincli is the command run on the server to recover ownership of the
// instance (DEC-057, RF-038). It is deliberately not part of the API: it needs
// access to the machine and to the database, which is what proves the person is
// the operator. Nothing here is reachable over HTTP, and no environment variable
// or trigger file starts it; the operator types the command.
package admincli

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/ownership"
)

const usage = `Usage:
  codice-admin recover-owner [--base-url URL]
      The owner lost their password or second factor. Ends every session and app
      token of the owner and prints a one-time reset link, valid for one hour.

  codice-admin transfer-owner --to USERNAME [--former-role admin|reader] [--yes]
      The owner is unavailable. Makes an existing account the owner in one step.
      The former owner becomes a reader unless --former-role says otherwise.

Both actions are audited and shown to the affected people at their next sign-in.
`

// Run executes one command. Output meant for the operator goes to out; the reset
// link is printed only there and is never logged or stored in the clear.
func Run(ctx context.Context, db *sql.DB, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return errors.New("a command is required")
	}
	switch args[0] {
	case "recover-owner":
		return recoverOwner(ctx, db, args[1:], out)
	case "transfer-owner":
		return transferOwner(ctx, db, args[1:], in, out)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		fmt.Fprint(out, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func flags(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

func recoverOwner(ctx context.Context, db *sql.DB, args []string, out io.Writer) error {
	fs := flags("recover-owner", out)
	base := fs.String("base-url", "", "address of this instance, to print a complete link")
	if err := fs.Parse(args); err != nil {
		return err
	}
	token, expires, owner, err := ownership.RecoverAccess(ctx, db)
	if err != nil {
		return err
	}
	link := "/?reset=" + token
	if *base != "" {
		link = strings.TrimRight(*base, "/") + link
	}
	fmt.Fprintf(out, "The sessions and app tokens of the owner %q were ended.\n\n", owner)
	fmt.Fprintf(out, "Open this link to choose a new password. It works once and expires at %s:\n\n  %s\n\n",
		expires.Format("2006-01-02 15:04:05 MST"), link)
	fmt.Fprintln(out, "It is shown only here. Running the command again cancels this link and prints a new one.")
	return nil
}

func transferOwner(ctx context.Context, db *sql.DB, args []string, in io.Reader, out io.Writer) error {
	fs := flags("transfer-owner", out)
	to := fs.String("to", "", "username of the account that becomes the owner")
	formerRole := fs.String("former-role", "reader", "what the former owner becomes: admin or reader")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *to == "" {
		return errors.New("--to is required")
	}
	if !*yes {
		fmt.Fprintf(out, "This makes %q the owner NOW and turns the current owner into a %s, ending their sessions.\n", *to, *formerRole)
		fmt.Fprintf(out, "Type the username %q to confirm: ", *to)
		line, _ := bufio.NewReader(in).ReadString('\n')
		if strings.TrimSpace(line) != *to {
			return errors.New("not confirmed; nothing was changed")
		}
	}
	from, err := ownership.RecoverTransfer(ctx, db, *to, *formerRole)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Done: %q is now the owner. %q is now a %s and was signed out everywhere.\n", *to, from, *formerRole)
	return nil
}
