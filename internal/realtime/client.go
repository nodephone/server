package realtime

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nodephone/server/internal/auth"
	"golang.org/x/net/websocket"
)

// Client represents a single active WebSocket connection bound to an authenticated user.
type Client struct {
	ID          string
	User        *auth.User
	hub         *Hub
	ws          *websocket.Conn
	send        chan *EventMessage
	rooms       map[string]bool
	roomsMu     sync.RWMutex
	connectedAt time.Time
}

// NewClient creates a new Client instance.
func NewClient(hub *Hub, ws *websocket.Conn, user *auth.User) *Client {
	return &Client{
		ID:          uuid.New().String(),
		User:        user,
		hub:         hub,
		ws:          ws,
		send:        make(chan *EventMessage, 256),
		rooms:       make(map[string]bool),
		connectedAt: time.Now().UTC(),
	}
}

// SubscribedRooms returns a slice of room names the client is currently subscribed to.
func (c *Client) SubscribedRooms() []string {
	c.roomsMu.RLock()
	defer c.roomsMu.RUnlock()

	rooms := make([]string, 0, len(c.rooms))
	for room := range c.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// IsSubscribed checks if the client is subscribed to a specific room.
func (c *Client) IsSubscribed(room string) bool {
	c.roomsMu.RLock()
	defer c.roomsMu.RUnlock()
	return c.rooms[room]
}

func (c *Client) addRoom(room string) {
	c.roomsMu.Lock()
	defer c.roomsMu.Unlock()
	c.rooms[room] = true
}

func (c *Client) removeRoom(room string) {
	c.roomsMu.Lock()
	defer c.roomsMu.Unlock()
	delete(c.rooms, room)
}

// Send enqueues a message into the client's write channel in a non-blocking manner.
func (c *Client) Send(msg *EventMessage) {
	select {
	case c.send <- msg:
	default:
		// Send buffer full, drop message or let write pump handle connection teardown
		fmt.Fprintf(c.hub.out, "[REALTIME] Send buffer full for client %s (user %s). Dropping message.\n", c.ID, c.User.Username)
	}
}

// ReadPump listens for incoming WebSocket JSON frames from the client connection.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.ws.Close()
	}()

	for {
		var msg EventMessage
		err := websocket.JSON.Receive(c.ws, &msg)
		if err != nil {
			break // Connection closed or network error
		}

		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now().UTC()
		}

		if c.User != nil {
			msg.SenderID = c.User.ID
			msg.SenderName = c.User.Username
		}

		switch msg.Type {
		case MessageTypePing:
			c.Send(&EventMessage{
				Type:      MessageTypePong,
				Timestamp: time.Now().UTC(),
			})
		case MessageTypeSubscribe:
			if msg.Room != "" {
				c.hub.Subscribe(c, msg.Room)
			}
		case MessageTypeUnsubscribe:
			if msg.Room != "" {
				c.hub.Unsubscribe(c, msg.Room)
			}
		case MessageTypePublish:
			if msg.Room != "" {
				c.hub.PublishToRoom(msg.Room, &msg)
			}
		default:
			// Custom event message, publish to room if specified, or broadcast
			if msg.Room != "" {
				c.hub.PublishToRoom(msg.Room, &msg)
			}
		}
	}
}

// WritePump pops messages from the send channel and writes JSON frames to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.ws.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Channel closed by hub
				_ = websocket.JSON.Send(c.ws, &EventMessage{
					Type:      MessageTypeSystem,
					Payload:   "Connection closing",
					Timestamp: time.Now().UTC(),
				})
				return
			}

			if err := websocket.JSON.Send(c.ws, msg); err != nil {
				return
			}
		case <-ticker.C:
			// Send heartbeat ping frame to client to verify connection health
			if err := websocket.JSON.Send(c.ws, &EventMessage{
				Type:      MessageTypePing,
				Timestamp: time.Now().UTC(),
			}); err != nil {
				return
			}
		}
	}
}
