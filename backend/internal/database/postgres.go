package database

import(
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ocnaibill/codice/backend/internal/config"
)

func ConnectPostgres() *pgx.Conn{
	dbURL := config.Get("DATABASE_URL", "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("	Error connecting to PostgreSQL: %v", err)
	}

	err = conn.Ping(ctx)
	if err != nil{
		log.Fatalf("	PostgreSQL isn't responding: %v", err)
	}

	log.Println("	Successfully connected to PostgreSQL!")
	return conn
}