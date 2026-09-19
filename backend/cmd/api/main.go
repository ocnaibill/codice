package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq" // Underscore initializes the driver anonymously
	"github.com/ocnaibill/codice/backend/internal/config"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/ocnaibill/codice/backend/internal/storage"
)

func main() {
	// 0. Load environment variables from .env
	config.Load()

	// Fail fast: the API must not start without a JWT secret (no default exists).
	appMiddleware.GetJWTSecret()

	// 1. Connection with PostgreSQL (PERF-02: connection pooling)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Ping database to verify active connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Database did not respond to ping: %v", err)
	}
	log.Println("✅ Successfully connected to PostgreSQL!")

	// Run automatic database migrations on startup
	if err := database.Migrate(db); err != nil {
		log.Fatalf("❌ Database auto-migrations failed: %v", err)
	}

	// 2. Connection with Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	redisClient, err := connectRedis(context.Background(), redisURL)
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	sessionStore := &sessions.Store{DB: db}
	authenticator := appMiddleware.Authenticator{
		Sessions: sessionStore.CheckSession,
		Basic:    sessionStore.VerifyAppToken,
	}

	wsHandler := &handlers.WsHandler{RedisClient: redisClient, Auth: authenticator}

	// Start Redis PubSub listener in background goroutine
	go wsHandler.ListenToRedis()

	// Define base storage directory (fallback to ./uploads).
	// Try to resolve relative paths from the project root by walking up from CWD.
	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	// If the path is relative, resolve it relative to the project root.
	// We find the project root by looking for a known marker file/dir.
	if !filepath.IsAbs(storagePath) {
		cwd, _ := os.Getwd()

		// Walk up from CWD looking for the backend/ directory or go.mod
		// The project root is the parent of backend/
		candidate := cwd
		for i := 0; i < 5; i++ {
			// Check if we're in the backend directory
			if filepath.Base(candidate) == "backend" {
				candidate = filepath.Dir(candidate) // go up to project root
				break
			}
			// Check if go.mod exists (project root)
			if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break // reached root, stop
			}
			candidate = parent
		}
		storagePath = filepath.Join(candidate, storagePath)
	}

	log.Printf("📂 Storage path resolved to: %s", storagePath)

	// File jobs (organizing, and later scanning and transferring) run in this
	// process; the Python worker only takes ingestion.
	mover := &storage.Mover{DB: db, Root: storagePath}
	handlers := fileJobHandlers(db, mover)
	types := make([]string, 0, len(handlers))
	for t := range handlers {
		types = append(types, t)
	}
	startFileJobs(context.Background(), &jobs.Runner{
		DB: db, Owner: jobs.NewOwnerName("api"), Types: types, MaxRunning: 1, LeaseSeconds: 300, Handlers: handlers,
	}, mover, &storage.Trash{DB: db, Root: storagePath})

	// 4. Configure Router
	r := newRouter(routerDeps{
		DB:          db,
		RedisClient: redisClient,
		Sessions:    sessionStore,
		Auth:        authenticator,
		WS:          wsHandler,
		StoragePath: storagePath,
		Mover:       mover,
	})

	// 5. Start HTTP Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Go server running on port %s", port)
	http.ListenAndServe(":"+port, r)
}
