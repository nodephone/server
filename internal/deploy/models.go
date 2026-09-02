package deploy

import "time"

// DeployStatus represents the current runtime deployment status of NodePhone Server.
type DeployStatus struct {
	Status            string    `json:"status"`             // OFFLINE, CONNECTING, LIVE, RECONNECTING
	PublicURL         string    `json:"public_url"`         // e.g. "https://my-app.nodephone.dev"
	CustomDomain      string    `json:"custom_domain,omitempty"`
	SSLActive         bool      `json:"ssl_active"`
	TunnelLatencyMs   int64     `json:"tunnel_latency_ms"`
	ActiveConnections int       `json:"active_connections"`
	LastHeartbeat     time.Time `json:"last_heartbeat"`
	Mode              string    `json:"mode"`               // development or production
	UptimeSeconds     int64     `json:"uptime_seconds"`
}

// CustomDomainInfo encapsulates custom domain binding state and TLS verification details.
type CustomDomainInfo struct {
	Domain     string    `json:"domain"`
	Status     string    `json:"status"`      // "pending", "verified", "active", "error"
	Verified   bool      `json:"verified"`
	SSLEnabled bool      `json:"ssl_enabled"`
	BoundAt    time.Time `json:"bound_at"`
}

// BindDomainRequest defines the payload required to attach a custom domain.
type BindDomainRequest struct {
	Domain string `json:"domain"`
}

// DeploymentHealth represents detailed health metrics returned by remote probes.
type DeploymentHealth struct {
	Status        string    `json:"status"`         // "healthy" or "unhealthy"
	TunnelAlive   bool      `json:"tunnel_alive"`
	LocalAlive    bool      `json:"local_alive"`
	SSLValid      bool      `json:"ssl_valid"`
	LatencyMs     int64     `json:"latency_ms"`
	LastCheck     time.Time `json:"last_check"`
}
