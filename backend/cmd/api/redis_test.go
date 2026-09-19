package main

import (
	"context"
	"testing"
	"time"
)

func TestConnectRedis_UnavailableDoesNotPreventStartup(t *testing.T) {
	// A missing Unix socket is deterministic and cannot contact a real Redis.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	client, err := connectRedis(ctx, "unix://"+t.TempDir()+"/missing.sock")
	if err != nil || client == nil {
		t.Fatalf("optional Redis prevented startup: %v", err)
	}
	defer client.Close()
	if err := client.Ping(ctx).Err(); err == nil {
		t.Fatal("expected Redis to be unavailable")
	}
}

func TestConnectRedis_InvalidConfigurationIsRejectedWithoutCredentials(t *testing.T) {
	client, err := connectRedis(context.Background(), "https://user:secret@localhost")
	if client != nil || err == nil || err.Error() != "invalid REDIS_URL configuration" {
		t.Fatalf("unexpected configuration result: client=%v error=%v", client, err)
	}
}
