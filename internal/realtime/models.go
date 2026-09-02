// Package realtime provides WebSocket hub/client concurrency, room subscriptions, broadcast channels,
// user presence tracking, heartbeat ping/pong messages, and event dispatching for NodePhone Server.
package realtime

import "time"

const (
	MessageTypeSubscribe   = "subscribe"
	MessageTypeUnsubscribe = "unsubscribe"
	MessageTypePublish     = "publish"
	MessageTypePing        = "ping"
	MessageTypePong        = "pong"
	MessageTypePresence    = "presence"
	MessageTypeSystem      = "system"
	MessageTypeEvent       = "event"
)

// EventMessage defines the structured JSON frame exchanged over WebSockets and published across rooms.
type EventMessage struct {
	Type       string      `json:"type"`
	Room       string      `json:"room,omitempty"`
	Payload    interface{} `json:"payload,omitempty"`
	SenderID   string      `json:"sender_id,omitempty"`
	SenderName string      `json:"sender_name,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}

// PresenceInfo holds real-time status and active room subscriptions for a connected user.
type PresenceInfo struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Status      string    `json:"status"` // "online", "away", "offline"
	Rooms       []string  `json:"rooms"`
	ConnectedAt time.Time `json:"connected_at"`
}
