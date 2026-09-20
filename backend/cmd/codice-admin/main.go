// Command codice-admin recovers ownership of a Códice instance. Run it on the
// server, with access to the database; see `codice-admin help`.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/admincli"
	"github.com/ocnaibill/codice/backend/internal/config"
)

func main() {
	config.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot open the database connection")
		os.Exit(1)
	}
	defer db.Close()

	if len(os.Args) > 1 && (os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help") {
		admincli.Run(context.Background(), db, os.Args[1:], os.Stdin, os.Stdout)
		return
	}
	// Checking a package or pruning a folder needs no database, so they work on any machine.
	if needsDatabase(os.Args[1:]) {
		if err := db.Ping(); err != nil {
			fmt.Fprintln(os.Stderr, "the database did not answer; is DATABASE_URL right and the database running?")
			os.Exit(1)
		}
	}
	storage := os.Getenv("CODICE_STORAGE_PATH")
	if storage == "" {
		storage = "./uploads"
	}
	env := admincli.Env{DatabaseURL: dbURL, StorageRoot: storage, Stderr: os.Stderr}
	if err := admincli.RunEnv(context.Background(), db, env, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// needsDatabase says whether a command talks to the database. verify-backup only does so
// for --deep, which restores into a temporary database.
func needsDatabase(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "help", "-h", "--help", "prune-backups":
		return false
	case "restore":
		// The restore opens its own connections and needs the target database to have no
		// others: a connection kept open here would make the command refuse to run.
		return false
	case "verify-backup":
		for _, a := range args[1:] {
			if a == "--deep" || a == "-deep" {
				return true
			}
		}
		return false
	}
	return true
}
