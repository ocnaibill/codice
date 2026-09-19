// Package database applies the versioned schema migrations (RNF-014).
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func newProvider(db *sql.DB) (*goose.Provider, error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	// A session lock makes two instances starting together apply migrations one
	// after the other instead of racing.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSessionLocker(locker))
}

// Migrate brings the database to the latest schema version. The baseline is
// idempotent, so a database created by the former startup routine is adopted
// as it is; later migrations are recorded in goose_db_version.
func Migrate(db *sql.DB) error {
	return MigrateTo(db, 0)
}

// MigrateTo applies migrations up to and including version (0 means all).
func MigrateTo(db *sql.DB, version int64) error {
	log.Println("🔄 Applying database migrations...")
	p, err := newProvider(db)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	ctx := context.Background()
	var results []*goose.MigrationResult
	if version == 0 {
		results, err = p.Up(ctx)
	} else {
		results, err = p.UpTo(ctx, version)
	}
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	for _, r := range results {
		log.Printf("   applied %s", r.Source.Path)
	}
	log.Println("✅ Database migrations completed successfully!")
	return nil
}

// RollbackTo reverts migrations down to and excluding version. It exists for
// tests and for recovering a failed upgrade in a copy of the database; the
// baseline itself cannot be rolled back.
func RollbackTo(db *sql.DB, version int64) error {
	p, err := newProvider(db)
	if err != nil {
		return err
	}
	_, err = p.DownTo(context.Background(), version)
	return err
}

// Version returns the current schema version (0 when nothing is applied).
func Version(db *sql.DB) (int64, error) {
	p, err := newProvider(db)
	if err != nil {
		return 0, err
	}
	return p.GetDBVersion(context.Background())
}
