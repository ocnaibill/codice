package main

import (
	"database/sql"
	"log"
	"net/http"
	"context"
	"os"
	"path/filepath"

	"github.com/ocnaibill/codice/backend/internal/config"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/redis/go-redis/v9"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/lib/pq" // Underscore initializes the driver anonymously
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
)

func main() {
	// 0. Load environment variables from .env
	config.Load()

	// 1. Connection with PostgreSQL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Ping database to verify active connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Database did not respond to ping: %v", err)
	}
	log.Println("✅ Successfully connected to PostgreSQL!")

	// Run automatic database migrations on startup
	if err := database.RunAutoMigrations(db); err != nil {
		log.Fatalf("❌ Database auto-migrations failed: %v", err)
	}

	// 2. Connection with Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}

	redisClient := redis.NewClient(opt)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis did not respond to ping: %v", err)
	}
	log.Println("✅ Successfully connected to Redis!")

	// 3. Instantiate Handlers
	libHandler := &handlers.LibraryHandler{DB: db}
	uploadHandler := &handlers.UploadHandler{
		DB:          db,
		RedisClient: redisClient,
	}
	wsHandler := &handlers.WsHandler{
		RedisClient: redisClient,
	}
	authHandler := &handlers.AuthHandler{
		DB: db,
	}

	// Start Redis PubSub listener in background goroutine
	go wsHandler.ListenToRedis()

	// 4. Configure Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:5173"
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{allowedOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("📚 Códice API is online!"))
	})

	// Public Auth & Setup Endpoints
	r.Get("/auth/setup-status", authHandler.GetSetupStatus)
	r.Post("/auth/setup", authHandler.SetupMasterAdmin)
	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)

	// Protected Application Endpoints
	r.With(appMiddleware.AuthMiddleware).Get("/works", libHandler.GetWorks)
	r.With(appMiddleware.AuthMiddleware).Get("/works/{id}", libHandler.GetWorkByID)
	r.With(appMiddleware.AuthMiddleware).Put("/works/{id}", libHandler.UpdateWork)
	r.With(appMiddleware.AuthMiddleware).Patch("/works/{id}/progress", libHandler.UpdateProgress)
	r.With(appMiddleware.AuthMiddleware).Delete("/works/{id}", libHandler.DeleteWork)
	r.With(appMiddleware.AuthMiddleware).Post("/upload", uploadHandler.HandleUpload)
	r.With(appMiddleware.AuthMiddleware).Post("/works/bulk-import", uploadHandler.HandleBulkImport)

	// WebSocket (auth handled inside handler for upgrade)
	r.Get("/ws", wsHandler.HandleWS)

	// Define base storage directory (fallback to ./uploads)
	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	// Ensure covers directory exists
	coversPath := filepath.Join(storagePath, "covers")
	os.MkdirAll(coversPath, 0755)

	// Serve cover images under /covers/ (requires auth via header or ?token= query param)
	fsCovers := http.StripPrefix("/covers/", http.FileServer(http.Dir(coversPath)))
	r.With(appMiddleware.AuthMiddleware).Get("/covers/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fsCovers.ServeHTTP(w, r)
	}))

	// Serve original files under /files/ (requires auth via header or ?token= query param)
	fsFiles := http.StripPrefix("/files/", http.FileServer(http.Dir(storagePath)))
	r.With(appMiddleware.AuthMiddleware).Get("/files/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fsFiles.ServeHTTP(w, r)
	}))

	// 5. Start HTTP Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Go server running on port %s", port)
	http.ListenAndServe(":"+port, r)
}