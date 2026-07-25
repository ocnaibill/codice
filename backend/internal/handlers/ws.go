package handlers

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// Configure WebSocket upgrader to allow cross-origin requests from React
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WsHandler manages active WebSocket client connections and listens for Redis PubSub events
type WsHandler struct {
	RedisClient *redis.Client
	Clients     map[*websocket.Conn]bool
	mu          sync.Mutex
}

// HandleWS upgrades HTTP connection to WebSocket and registers active client
func (h *WsHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
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
