package main

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ocnaibill/codice/backend/internal/config"
	"github.com/ocnaibill/codice/backend/internal/database"
)

func main() {
	config.Load()

	pgConn := database.ConnectPostgres()
	defer pgConn.Close(context.Background())

	redisClient := database.ConnectRedis()
	defer redisClient.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("📚 Códice API is online and operational!"))
	})

	port := config.Get("PORT", "8080")
	log.Printf("🚀 Servidor running on port %s...", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Critic error on server: %v", err)
	}
}