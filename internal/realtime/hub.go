package realtime

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/nodephone/server/internal/auth"
)

// Hub manages client connections, room subscriptions, broadcast channels, and user presence tracking.
type Hub struct {
	clients     map[*Client]bool
	rooms       map[string]map[*Client]bool
	userClients map[string]map[*Client]bool
	register    chan *Client
	unregister  chan *Client
	broadcast   chan *EventMessage
	mu          sync.RWMutex
	out         io.Writer
}

// NewHub initializes a new Hub instance.
func NewHub(out io.Writer) *Hub {
	if out == nil {
		out = io.Discard
	}
	return &Hub{
		clients:     make(map[*Client]bool),
		rooms:       make(map[string]map[*Client]bool),
		userClients: make(map[string]map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *EventMessage, 1024),
		out:         out,
	}
}

// Run executes the central Hub event loop until the context is cancelled.
func (h *Hub) Run(ctx context.Context) {
	fmt.Fprintln(h.out, "[REALTIME] Realtime Engine Hub event loop started")

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(h.out, "[REALTIME] Stopping Hub event loop...")
			h.closeAll()
			return
		case client := <-h.register:
			h.handleRegister(client)
		case client := <-h.unregister:
			h.handleUnregister(client)
		case msg := <-h.broadcast:
			h.handleBroadcast(msg)
		}
	}
}

// Stop gracefully terminates all client connections and resets the hub.
func (h *Hub) Stop() {
	h.closeAll()
}

// Register adds a client connection to the hub.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister removes a client connection from the hub.
func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

func (h *Hub) handleRegister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[c] = true

	userID := c.User.ID
	if h.userClients[userID] == nil {
		h.userClients[userID] = make(map[*Client]bool)
	}
	h.userClients[userID][c] = true

	// Auto-subscribe client to global room and user-specific room
	userRoom := "user:" + userID
	h.subscribeLocked(c, "global")
	h.subscribeLocked(c, userRoom)

	fmt.Fprintf(h.out, "[REALTIME] Client %s (user: %s) connected\n", c.ID, c.User.Username)

	// Send welcome event to client
	c.Send(&EventMessage{
		Type:       MessageTypeSystem,
		Room:       "global",
		Payload:    fmt.Sprintf("Welcome to NodePhone Realtime Engine, %s!", c.User.Username),
		SenderID:   "system",
		SenderName: "NodePhone Kernel",
		Timestamp:  time.Now().UTC(),
	})

	// Broadcast user online presence to global room
	h.publishToRoomLocked("global", &EventMessage{
		Type: MessageTypePresence,
		Room: "global",
		Payload: map[string]interface{}{
			"user_id":  userID,
			"username": c.User.Username,
			"status":   "online",
		},
		SenderID:   userID,
		SenderName: c.User.Username,
		Timestamp:  time.Now().UTC(),
	})
}

func (h *Hub) handleUnregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.clients[c]; !exists {
		return
	}

	delete(h.clients, c)

	// Remove client from all rooms
	for room := range c.rooms {
		if clients, ok := h.rooms[room]; ok {
			delete(clients, c)
			if len(clients) == 0 {
				delete(h.rooms, room)
			}
		}
	}

	userID := c.User.ID
	if uClients, ok := h.userClients[userID]; ok {
		delete(uClients, c)
		if len(uClients) == 0 {
			delete(h.userClients, userID)
			// User has no remaining active devices/connections, broadcast offline status
			h.publishToRoomLocked("global", &EventMessage{
				Type: MessageTypePresence,
				Room: "global",
				Payload: map[string]interface{}{
					"user_id":  userID,
					"username": c.User.Username,
					"status":   "offline",
				},
				SenderID:   userID,
				SenderName: c.User.Username,
				Timestamp:  time.Now().UTC(),
			})
		}
	}

	close(c.send)
	fmt.Fprintf(h.out, "[REALTIME] Client %s (user: %s) disconnected\n", c.ID, c.User.Username)
}

func (h *Hub) subscribeLocked(c *Client, room string) {
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*Client]bool)
	}
	h.rooms[room][c] = true
	c.addRoom(room)
}

func (h *Hub) unsubscribeLocked(c *Client, room string) {
	if clients, ok := h.rooms[room]; ok {
		delete(clients, c)
		if len(clients) == 0 {
			delete(h.rooms, room)
		}
	}
	c.removeRoom(room)
}

// Subscribe adds a client to a target room.
func (h *Hub) Subscribe(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.subscribeLocked(c, room)
	c.Send(&EventMessage{
		Type:      MessageTypeSystem,
		Room:      room,
		Payload:   fmt.Sprintf("Subscribed to room %s", room),
		Timestamp: time.Now().UTC(),
	})
}

// Unsubscribe removes a client from a target room.
func (h *Hub) Unsubscribe(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.unsubscribeLocked(c, room)
	c.Send(&EventMessage{
		Type:      MessageTypeSystem,
		Room:      room,
		Payload:   fmt.Sprintf("Unsubscribed from room %s", room),
		Timestamp: time.Now().UTC(),
	})
}

func (h *Hub) publishToRoomLocked(room string, msg *EventMessage) {
	if clients, ok := h.rooms[room]; ok {
		for c := range clients {
			c.Send(msg)
		}
	}
}

func (h *Hub) handleBroadcast(msg *EventMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if msg.Room != "" {
		h.publishToRoomLocked(msg.Room, msg)
	} else {
		for c := range h.clients {
			c.Send(msg)
		}
	}
}

// PublishToRoom dispatches a message to all clients subscribed to a specific room.
func (h *Hub) PublishToRoom(room string, msg *EventMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	msg.Room = room

	h.mu.RLock()
	defer h.mu.RUnlock()
	h.publishToRoomLocked(room, msg)
}

// PublishToUser dispatches a message to all active connections belonging to a specific user.
func (h *Hub) PublishToUser(userID string, msg *EventMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.userClients[userID]; ok {
		for c := range clients {
			c.Send(msg)
		}
	}
}

// Broadcast enqueues a message to be sent to all connected clients globally.
func (h *Hub) Broadcast(msg *EventMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	h.broadcast <- msg
}

// GetOnlineUsers returns a list of unique authenticated users currently connected to the Hub.
func (h *Hub) GetOnlineUsers() []*auth.User {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userMap := make(map[string]*auth.User)
	for uID, clients := range h.userClients {
		for c := range clients {
			if c.User != nil {
				userMap[uID] = c.User
				break
			}
		}
	}

	users := make([]*auth.User, 0, len(userMap))
	for _, u := range userMap {
		users = append(users, u)
	}
	return users
}

// GetPresenceInfo returns presence metadata for all online users.
func (h *Hub) GetPresenceInfo() []*PresenceInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userMap := make(map[string]*PresenceInfo)
	for uID, clients := range h.userClients {
		var firstClient *Client
		roomsSet := make(map[string]bool)

		for c := range clients {
			if firstClient == nil {
				firstClient = c
			}
			for r := range c.rooms {
				roomsSet[r] = true
			}
		}

		if firstClient != nil && firstClient.User != nil {
			roomsList := make([]string, 0, len(roomsSet))
			for r := range roomsSet {
				roomsList = append(roomsList, r)
			}

			userMap[uID] = &PresenceInfo{
				UserID:      firstClient.User.ID,
				Username:    firstClient.User.Username,
				Status:      "online",
				Rooms:       roomsList,
				ConnectedAt: firstClient.connectedAt,
			}
		}
	}

	presence := make([]*PresenceInfo, 0, len(userMap))
	for _, p := range userMap {
		presence = append(presence, p)
	}
	return presence
}

// ActiveClientCount returns total active WebSocket connection count.
func (h *Hub) ActiveClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		close(c.send)
		_ = c.ws.Close()
	}
	h.clients = make(map[*Client]bool)
	h.rooms = make(map[string]map[*Client]bool)
	h.userClients = make(map[string]map[*Client]bool)
}
