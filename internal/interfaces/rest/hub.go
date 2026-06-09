package rest

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

type Hub struct {
	mu      sync.RWMutex
	writeMu sync.Mutex
	clients map[*websocket.Conn]struct{}
}

type clientSocketMessage struct {
	Type   string  `json:"type"`
	SentAt float64 `json:"sent_at,omitempty"`
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) Publish(payload []byte) {
	if len(payload) == 0 {
		return
	}

	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	h.writeMu.Lock()
	defer h.writeMu.Unlock()

	for _, conn := range clients {
		if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
			h.remove(conn)
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			h.remove(conn)
		}
	}
}

func (h *Hub) writeMessage(conn *websocket.Conn, payload []byte) error {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func (h *Hub) Handle(c fiber.Ctx) error {
	upgrader := websocket.FastHTTPUpgrader{
		CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
			return true
		},
	}

	return upgrader.Upgrade(c.RequestCtx(), func(conn *websocket.Conn) {
		h.add(conn)
		defer h.remove(conn)
		defer func() {
			_ = conn.Close()
		}()

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var message clientSocketMessage
			if err := json.Unmarshal(payload, &message); err == nil && message.Type == "exchange.ping" {
				reply, _ := json.Marshal(clientSocketMessage{
					Type:   "exchange.pong",
					SentAt: message.SentAt,
				})
				if err := h.writeMessage(conn, reply); err != nil {
					h.remove(conn)
					return
				}
			}
		}
	})
}

func (h *Hub) add(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}
