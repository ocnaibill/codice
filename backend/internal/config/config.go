package config

import(
	"log"
	"os"

	"github.com/joho/godotenv"
)

// .env vars injection
func Load(){
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("Warning: .env file don't found. Using environment system variables.")
	}
}

func Get(key, fallback string) string{
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}