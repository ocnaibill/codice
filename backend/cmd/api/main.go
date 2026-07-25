package main

import (
	"database/sql"
	"log"
	"net/http"
	"context"
	"os"

	"github.com/ocnaibill/codice/backend/internal/config"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/redis/go-redis/v9"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/lib/pq" // Underscore initializes the driver anonymously
)

func main() {
	// 0. Load environment variables from .env
	config.Load()

	// 1. Connection with PostgreSQL
	// Default to local docker-compose credentials if DATABASE_URL is not set
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
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("📚 Códice API is online!"))
	})

	r.Get("/works", libHandler.GetWorks)
	r.Get("/works/{id}", libHandler.GetWorkByID)
	r.Put("/works/{id}", libHandler.UpdateWork)
	r.Post("/upload", uploadHandler.HandleUpload)
	r.Get("/ws", wsHandler.HandleWS)

	// Serve cover images as static files under /covers/
	os.MkdirAll("./uploads/covers", 0755)
	fsCovers := http.StripPrefix("/covers/", http.FileServer(http.Dir("./uploads/covers")))
	r.Get("/covers/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fsCovers.ServeHTTP(w, r)
	}))

	// Serve original files under /files/
	fsFiles := http.StripPrefix("/files/", http.FileServer(http.Dir("./uploads")))
	r.Get("/files/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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