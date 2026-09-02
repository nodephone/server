package deploy

import (
	"context"
	"io"
	"sync"
	"time"
)

// DeploymentEngine coordinates HTTPS tunneling, automatic TLS handling, custom domain verification,
// health check monitoring, and deployment REST API endpoints for NodePhone.
type DeploymentEngine struct {
	cfg           DeployConfig
	tunnel        *Tunnel
	tlsManager    *TLSManager
	domainManager *DomainManager
	healthMonitor *HealthMonitor
	startTime     time.Time
	out           io.Writer
	mu            sync.RWMutex
}

// NewDeploymentEngine initializes a new DeploymentEngine instance.
func NewDeploymentEngine(cfg DeployConfig, out io.Writer) *DeploymentEngine {
	if out == nil {
		out = io.Discard
	}

	tlsMgr := NewTLSManager()
	domainMgr := NewDomainManager(tlsMgr)
	health := NewHealthMonitor()
	tunnel := NewTunnel(cfg, health, out)

	return &DeploymentEngine{
		cfg:           cfg,
		tunnel:        tunnel,
		tlsManager:    tlsMgr,
		domainManager: domainMgr,
		healthMonitor: health,
		startTime:     time.Now().UTC(),
		out:           out,
	}
}

// Start opens the secure tunnel and activates background monitoring workers.
func (de *DeploymentEngine) Start(ctx context.Context, localPort int) error {
	de.mu.Lock()
	defer de.mu.Unlock()
	return de.tunnel.Connect(ctx, localPort)
}

// GetStatus returns the current deployment status details.
func (de *DeploymentEngine) GetStatus() DeployStatus {
	de.mu.RLock()
	defer de.mu.RUnlock()

	customDomainStr := ""
	if domainInfo := de.domainManager.GetDomainInfo(); domainInfo != nil {
		customDomainStr = domainInfo.Domain
	}

	return DeployStatus{
		Status:            de.tunnel.GetStatus(),
		PublicURL:         de.tunnel.GetPublicURL(),
		CustomDomain:      customDomainStr,
		SSLActive:         de.tlsManager.IsSSLActive(),
		TunnelLatencyMs:   de.healthMonitor.GetLatencyMs(),
		ActiveConnections: de.tunnel.GetActiveConnections(),
		LastHeartbeat:     de.healthMonitor.GetLastHeartbeat(),
		Mode:              de.cfg.Mode,
		UptimeSeconds:     int64(time.Since(de.startTime).Seconds()),
	}
}

// GetHealth returns current deployment health details.
func (de *DeploymentEngine) GetHealth() DeploymentHealth {
	return de.healthMonitor.GetHealth()
}

// GetDomainInfo returns custom domain metadata if attached.
func (de *DeploymentEngine) GetDomainInfo() *CustomDomainInfo {
	return de.domainManager.GetDomainInfo()
}

// BindDomain binds a custom domain to the deployment engine.
func (de *DeploymentEngine) BindDomain(domain string) (*CustomDomainInfo, error) {
	return de.domainManager.BindCustomDomain(domain)
}

// Stop terminates the deployment tunnel and background workers cleanly.
func (de *DeploymentEngine) Stop() error {
	return de.tunnel.Close()
}
