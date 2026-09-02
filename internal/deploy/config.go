// Package deploy implements the secure global HTTPS tunneling engine, automatic TLS handling,
// custom domain ownership verification, health check monitoring, auto-reconnection workers,
// and deployment REST API endpoints for NodePhone.
package deploy

import "time"

// DeployConfig holds configuration settings for the deployment subsystem.
type DeployConfig struct {
	Enabled             bool          `json:"enabled"`
	Mode                string        `json:"mode"`                  // "development" or "production"
	PublicDomain        string        `json:"public_domain"`         // Base public domain, e.g. "nodephone.dev"
	TunnelHost          string        `json:"tunnel_host"`           // Public gateway host
	AutoReconnect       bool          `json:"auto_reconnect"`        // Restore tunnel on disconnect
	ReconnectInterval   time.Duration `json:"reconnect_interval"`   // Interval between reconnection attempts
	HeartbeatInterval   time.Duration `json:"heartbeat_interval"`   // Heartbeat check interval
}

// DefaultDeployConfig returns production-ready default deployment configuration settings.
func DefaultDeployConfig() DeployConfig {
	return DeployConfig{
		Enabled:           true,
		Mode:              "development",
		PublicDomain:      "nodephone.dev",
		TunnelHost:        "tunnel.nodephone.dev",
		AutoReconnect:     true,
		ReconnectInterval: 5 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}
