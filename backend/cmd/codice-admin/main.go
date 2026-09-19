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
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "the database did not answer; is DATABASE_URL right and the database running?")
		os.Exit(1)
	}
	if err := admincli.Run(context.Background(), db, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
