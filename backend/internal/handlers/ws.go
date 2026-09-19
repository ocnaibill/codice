package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/redis/go-redis/v9"
)

// Configure WebSocket upgrader with origin validation
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Allow requests without Origin header (CLI, curl, etc.)
			return true
		}
		// In production, validate against CORS_ALLOWED_ORIGINS
		allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
		if allowedOrigins == "" {
			return true
		}
		for _, allowed := range strings.Split(allowedOrigins, ",") {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
		log.Printf("WebSocket origin rejected: %s (allowed: %s)", origin, allowedOrigins)
		return false
	},
}

// WsHandler manages active WebSocket client connections and listens for Redis PubSub events
type WsHandler struct {
	RedisClient *redis.Client
	Auth        middleware.Authenticator
	Clients     map[*websocket.Conn]bool
	mu          sync.Mutex
}

// HandleWS upgrades HTTP connection to WebSocket and registers active client.
// Browsers cannot set headers on a WebSocket, so they present a short-lived
// "ws" ticket (?ticket=, from POST /auth/resource-token); session tokens never
// travel in the query string.
func (h *WsHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.Auth.AuthenticateWS(r); err != nil {
		if errors.Is(err, middleware.ErrInvalidSession) {
			http.Error(w, "Access denied: Authentication required", http.StatusUnauthorized)
		} else {
			http.Error(w, "Authentication unavailable", http.StatusInternalServerError)
		}
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	h.mu.Lock()
	if h.Clients == nil {
		h.Clients = make(map[*websocket.Conn]bool)
	}
	h.Clients[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.Clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	// Keep connection alive by listening for read disconnections
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// ListenToRedis subscribes to Redis PubSub channel 'codice_updates' and broadcasts events to all WebSocket clients
func (h *WsHandler) ListenToRedis() {
	ctx := context.Background()
	pubsub := h.RedisClient.Subscribe(ctx, "codice_updates")
	defer pubsub.Close()

	log.Println("📡 Redis PubSub listener started on channel 'codice_updates'")
	ch := pubsub.Channel()

	for msg := range ch {
		h.mu.Lock()
		for client := range h.Clients {
			err := client.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
			if err != nil {
				client.Close()
				delete(h.Clients, client)
			}
		}
		h.mu.Unlock()
	}
}
