package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// Message represents a WebSocket message
type Message struct {
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	User      string    `json:"user"`
	Timestamp time.Time `json:"timestamp"`
	Delay     int       `json:"delay,omitempty"` // Delay in milliseconds for testing
}

// Client represents a WebSocket client connection
type Client struct {
	conn     *websocket.Conn
	send     chan Message
	hub      *Hub
	userID   string
	isActive bool
	mutex    sync.RWMutex
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

// Service represents the WebSocket service
type Service struct {
	hub *Hub
}

// NewService creates a new WebSocket service
func NewService() *Service {
	hub := &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}

	service := &Service{hub: hub}
	go hub.run()

	return service
}

// GetHandler returns the WebSocket HTTP handler
func (s *Service) GetHandler() http.HandlerFunc {
	return s.handleWebSocket
}

// GetStatsHandler returns handler for connection statistics
func (s *Service) GetStatsHandler() http.HandlerFunc {
	return s.handleStats
}

// run starts the hub's main event loop
func (h *Hub) run() {
	log.Printf("🏢 Hub event loop started")
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mutex.Unlock()

			log.Printf("➕ Client registered: %s (total clients: %d)", client.userID, clientCount)

			// Send welcome message
			welcome := Message{
				Type:      "system",
				Content:   "Welcome to the chat!",
				User:      "system",
				Timestamp: time.Now(),
			}

			select {
			case client.send <- welcome:
				log.Printf("👋 Welcome message sent to %s", client.userID)
			default:
				log.Printf("❌ Failed to send welcome message to %s - closing connection", client.userID)
				close(client.send)
				h.mutex.Lock()
				delete(h.clients, client)
				h.mutex.Unlock()
			}

			// Notify others about new user
			notification := Message{
				Type:      "notification",
				Content:   client.userID + " joined the chat",
				User:      "system",
				Timestamp: time.Now(),
			}
			log.Printf("📢 Notifying other clients that %s joined", client.userID)
			h.broadcastToOthers(notification, client)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				clientCount := len(h.clients)
				h.mutex.Unlock()

				log.Printf("➖ Client unregistered: %s (remaining clients: %d)", client.userID, clientCount)

				// Notify others about user leaving
				notification := Message{
					Type:      "notification",
					Content:   client.userID + " left the chat",
					User:      "system",
					Timestamp: time.Now(),
				}
				log.Printf("📢 Notifying other clients that %s left", client.userID)
				h.broadcastToOthers(notification, client)
			} else {
				h.mutex.Unlock()
				log.Printf("⚠️ Attempted to unregister unknown client: %s", client.userID)
			}

		case message := <-h.broadcast:
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			log.Printf("📡 Broadcasting message from %s to %d clients: %s", message.User, clientCount, message.Content)

			h.mutex.RLock()
			for client := range h.clients {
				// Apply artificial delay if specified
				if message.Delay > 0 {
					log.Printf("⏱️ Applying %dms delay for message to %s", message.Delay, client.userID)
					go func(c *Client, msg Message) {
						time.Sleep(time.Duration(msg.Delay) * time.Millisecond)
						select {
						case c.send <- msg:
							log.Printf("✅ Delayed message sent to %s", c.userID)
						default:
							log.Printf("❌ Failed to send delayed message to %s - closing connection", c.userID)
							close(c.send)
							h.mutex.Lock()
							delete(h.clients, c)
							h.mutex.Unlock()
						}
					}(client, message)
				} else {
					select {
					case client.send <- message:
						log.Printf("✅ Message sent to %s", client.userID)
					default:
						log.Printf("❌ Failed to send message to %s - closing connection", client.userID)
						close(client.send)
						h.mutex.Lock()
						delete(h.clients, client)
						h.mutex.Unlock()
					}
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// broadcastToOthers sends a message to all clients except the specified one
func (h *Hub) broadcastToOthers(message Message, sender *Client) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	recipientCount := 0
	for client := range h.clients {
		if client != sender {
			recipientCount++
		}
	}

	log.Printf("📤 Broadcasting to %d other clients (excluding %s): %s", recipientCount, sender.userID, message.Content)

	for client := range h.clients {
		if client != sender {
			select {
			case client.send <- message:
				log.Printf("✅ Notification sent to %s", client.userID)
			default:
				log.Printf("❌ Failed to send notification to %s - closing connection", client.userID)
				close(client.send)
				h.mutex.Lock()
				delete(h.clients, client)
				h.mutex.Unlock()
			}
		}
	}
}

// handleWebSocket handles WebSocket connections
func (s *Service) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔗 New WebSocket connection request from %s", r.RemoteAddr)

	// Add CORS headers for WebSocket handshake
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed: %v", err)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = "anonymous_" + time.Now().Format("150405")
	}

	log.Printf("👤 WebSocket client connected: %s (from %s)", userID, r.RemoteAddr)

	client := &Client{
		conn:     conn,
		send:     make(chan Message, 256),
		hub:      s.hub,
		userID:   userID,
		isActive: true,
	}

	s.hub.register <- client

	// Start goroutines for reading and writing
	log.Printf("🚀 Starting read/write pumps for client: %s", userID)
	go client.writePump()
	go client.readPump()
}

// handleStats returns connection statistics
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	s.hub.mutex.RLock()
	clientCount := len(s.hub.clients)
	s.hub.mutex.RUnlock()

	stats := map[string]interface{}{
		"active_connections": clientCount,
		"service":            "websocket",
		"timestamp":          time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(stats)
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	log.Printf("📖 ReadPump started for client: %s", c.userID)
	defer func() {
		log.Printf("📖 ReadPump ending for client: %s", c.userID)
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		log.Printf("🏓 Pong received from client: %s", c.userID)
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var message Message
		err := c.conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ WebSocket error for %s: %v", c.userID, err)
			} else {
				log.Printf("🔌 WebSocket connection closed for %s: %v", c.userID, err)
			}
			break
		}

		log.Printf("📨 Message received from %s: type=%s, content=%s", c.userID, message.Type, message.Content)

		// Add timestamp and user info
		message.Timestamp = time.Now()
		message.User = c.userID

		if message.Type == "" {
			message.Type = "message"
		}

		switch message.Type {
		case "ping":
			log.Printf("🏓 Ping received from %s, sending pong", c.userID)
			pong := Message{
				Type:      "pong",
				Content:   "pong",
				User:      "system",
				Timestamp: time.Now(),
			}
			select {
			case c.send <- pong:
				log.Printf("✅ Pong sent to %s", c.userID)
			default:
				log.Printf("❌ Failed to send pong to %s - channel full", c.userID)
				return
			}
		default:
			log.Printf("📤 Broadcasting message from %s to all clients", c.userID)
			c.hub.broadcast <- message
		}
	}
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	log.Printf("✍️ WritePump started for client: %s", c.userID)
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		log.Printf("✍️ WritePump ending for client: %s", c.userID)
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				log.Printf("💔 Send channel closed for %s", c.userID)
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			log.Printf("📤 Sending message to %s: type=%s, content=%s", c.userID, message.Type, message.Content)
			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("❌ WebSocket write error for %s: %v", c.userID, err)
				return
			}
			log.Printf("✅ Message sent successfully to %s", c.userID)

		case <-ticker.C:
			log.Printf("🏓 Sending ping to client: %s", c.userID)
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("❌ Ping failed for %s: %v", c.userID, err)
				return
			}
			log.Printf("✅ Ping sent to %s", c.userID)
		}
	}
}

// Close gracefully closes client connection and cleans resources
func (c *Client) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.isActive {
		c.isActive = false
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		close(c.send)
		c.hub.unregister <- c
		log.Printf("🛑 Client closed: %s", c.userID)
	}
}

// GetConnectedClients returns the number of connected clients
func (s *Service) GetConnectedClients() int {
	s.hub.mutex.RLock()
	defer s.hub.mutex.RUnlock()
	return len(s.hub.clients)
}

// BroadcastMessage sends a message to all connected clients
func (s *Service) BroadcastMessage(message Message) {
	message.Timestamp = time.Now()
	s.hub.broadcast <- message
}
