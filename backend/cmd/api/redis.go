package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// connectRedis validates configuration, but availability never gates the API:
// jobs are durable in PostgreSQL. Keep the client so notifications and PubSub
// can reconnect when Redis returns, without restarting the API.
func connectRedis(ctx context.Context, rawURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(rawURL)
	if err != nil {
		// Parse errors may contain credentials from the URL.
		return nil, errors.New("invalid REDIS_URL configuration")
	}
	opt.ContextTimeoutEnabled = true
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Println("Redis unavailable; continuing with PostgreSQL jobs. Live notifications will resume when Redis reconnects.")
	} else {
		log.Println("Successfully connected to Redis")
	}
	return client, nil
}
