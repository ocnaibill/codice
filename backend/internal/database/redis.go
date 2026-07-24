package database

import(
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ocnaibill/codice/backend/internal/config"
)

func ConnectRedis() *redis.Client{
	redisURL := config.Get("REDIS_URL", "redis://localhost:6379/0")

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("	Error parsing Redis URL: %v", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Ping(ctx).Result()
	if err != nil{
		log.Fatalf("	Error connecting to Redis: %v", err)
	}

	log.Println("	Succcessfully connected to Redis!")
	return client
}