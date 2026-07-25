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
	_ "github.com/lib/pq" // O underscore inicializa o driver de forma anônima
)

func main() {
	// 0. Carrega variáveis de ambiente do .env
	config.Load()

	// 1. Conexão com o PostgreSQL
	// Como estamos rodando local, apontamos direto para as credenciais do docker-compose
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Falha ao abrir conexão com o banco: %v", err)
	}
	defer db.Close()

	// Testa se o banco está vivo de fato
	if err := db.Ping(); err != nil {
		log.Fatalf("O banco não respondeu ao ping: %v", err)
	}
	log.Println("✅ Conectado ao PostgreSQL com sucesso!")


	// Conexão com o Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Falha ao configurar Redis: %v", err)
	}
	
	redisClient := redis.NewClient(opt)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("O Redis não respondeu ao ping: %v", err)
	}
	log.Println("✅ Conectado ao Redis com sucesso!")

	// 3. Instancia os Handlers
	libHandler := &handlers.LibraryHandler{DB: db}
	uploadHandler := &handlers.UploadHandler{
		DB:          db, 
		RedisClient: redisClient,
	}

	// 4. Configura o Roteador
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
		w.Write([]byte("📚 Códice API está online!"))
	})

	r.Get("/works", libHandler.GetWorks)
	r.Post("/upload", uploadHandler.HandleUpload)

	// 4. Inicia o Servidor
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Servidor Go rodando na porta %s", port)
	http.ListenAndServe(":"+port, r)
}