package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Load environment variables from .env file.
// Searches multiple paths to be resilient regardless of working directory.
func Load() {
	paths := []string{
		"../.env",       // Running from backend/ (go run ./cmd/api)
		".env",          // Running from project root
		filepath.Join(os.Getenv("CODICE_STORAGE_PATH"), "..", ".env"), // Relative to storage
	}

	loaded := false
	for _, p := range paths {
		err := godotenv.Load(p)
		if err == nil {
			log.Printf("Config loaded from: %s", p)
			loaded = true
			break
		}
	}

	if !loaded {
		log.Println("Warning: No .env file found. Using system environment variables.")
	}
}

func Get(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}