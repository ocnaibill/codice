package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/ocnaibill/codice/backend/internal/identity"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
)

// sweepEvery is how often directory identities with a live session are checked
// against the directory (DEC-061). The ceiling is much longer, so this only sets
// how promptly a deactivated person loses access after the directory says so.
const sweepEvery = 10 * time.Minute

// unreachable stands in for the directory when LDAP is not configured. Accounts
// that were linked earlier are still subject to the ceiling: without a directory
// to confirm them their sessions end once it passes, instead of living on.
type unreachable struct{}

func (unreachable) Lookup(context.Context, string) (*ldapauth.Entry, error) {
	return nil, ldapauth.ErrUnavailable
}
func (unreachable) LookupBySubject(context.Context, string) (*ldapauth.Entry, error) {
	return nil, ldapauth.ErrUnavailable
}
func (unreachable) Authenticate(context.Context, *ldapauth.Entry, string) error {
	return ldapauth.ErrUnavailable
}
func (unreachable) Check(context.Context) error { return ldapauth.ErrUnavailable }

// startIdentitySweep revalidates directory identities in the background.
func startIdentitySweep(ctx context.Context, db *sql.DB, dir ldapauth.Directory) {
	if dir == nil {
		dir = unreachable{}
	}
	go func() {
		ticker := time.NewTicker(sweepEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			policy, err := identity.GetPolicy(ctx, db)
			if err != nil {
				continue
			}
			res, err := identity.Sweep(ctx, db, dir, time.Duration(policy.RevalidateHours)*time.Hour)
			if err != nil {
				log.Println("identity sweep:", err)
			} else if res.Revoked > 0 {
				log.Printf("identity sweep: ended the access of %d account(s) the directory no longer confirms", res.Revoked)
			}
		}
	}()
}
