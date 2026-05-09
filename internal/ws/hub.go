package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     isAllowedOrigin,
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func isAllowedOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	allowed := strings.TrimSpace(os.Getenv("QUIZHUB_ALLOWED_ORIGINS"))
	if allowed != "" {
		for _, candidate := range strings.Split(allowed, ",") {
			if strings.EqualFold(strings.TrimSpace(candidate), origin) {
				return true
			}
		}
		return false
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// Event types sent over WebSocket.
const (
	EventPlayerJoined   = "player_joined"
	EventPlayerKicked   = "player_kicked"
	EventGameCountdown  = "game_countdown"
	EventNewQuestion    = "new_question"
	EventPlayerAnswered = "player_answered"
	EventTimeUp         = "time_up"
	EventYourResult     = "your_result"
	EventGameFinished   = "game_finished"
	EventGameReset      = "game_reset"
	EventLeaderboard    = "leaderboard_update"
	EventPlayersUpdate  = "players_update"
	EventError          = "error"
)

// Message is the envelope sent to clients.
type Message struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// Client represents a single WebSocket connection.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	Role     string // "player" or "admin"
	PlayerID string
	mu       sync.Mutex
}

// Hub manages all active WebSocket connections.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop. Call as a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WS: client connected (role=%s, players=%d)", client.Role, h.ClientCount())

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WS: client disconnected (role=%s, players=%d)", client.Role, h.ClientCount())

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					h.mu.RUnlock()
					h.mu.Lock()
					delete(h.clients, client)
					close(client.send)
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(event string, data interface{}) {
	msg := Message{Event: event, Data: data}
	b, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WS: marshal error: %v", err)
		return
	}
	h.broadcast <- b
}

// BroadcastToRole sends a message only to clients with a specific role.
func (h *Hub) BroadcastToRole(role, event string, data interface{}) {
	msg := Message{Event: event, Data: data}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.Role == role {
			select {
			case client.send <- b:
			default:
			}
		}
	}
}

// SendToPlayer sends a message to a specific player.
func (h *Hub) SendToPlayer(playerID, event string, data interface{}) {
	msg := Message{Event: event, Data: data}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.PlayerID == playerID {
			select {
			case client.send <- b:
			default:
			}
		}
	}
}

// DisconnectPlayer forcefully disconnects a player by ID.
func (h *Hub) DisconnectPlayer(playerID string) {
	h.mu.RLock()
	var target *Client
	for client := range h.clients {
		if client.PlayerID == playerID {
			target = client
			break
		}
	}
	h.mu.RUnlock()
	if target != nil {
		h.unregister <- target
		target.conn.Close()
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HandleWS upgrades an HTTP connection to WebSocket.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS: upgrade error: %v", err)
		return
	}

	role := r.URL.Query().Get("role")
	if role == "" {
		role = "player"
	}
	playerID := r.URL.Query().Get("player_id")

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		Role:     role,
		PlayerID: playerID,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
