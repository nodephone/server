package deploy

import (
	"sync"
	"time"
)

// HealthMonitor tracks deployment vitality, heartbeat timestamps, and tunnel latency metrics.
type HealthMonitor struct {
	lastHeartbeat time.Time
	latencyMs     int64
	localAlive    bool
	tunnelAlive   bool
	mu            sync.RWMutex
}

// NewHealthMonitor initializes a new HealthMonitor instance.
func NewHealthMonitor() *HealthMonitor {
	return &HealthMonitor{
		lastHeartbeat: time.Now().UTC(),
		latencyMs:     5,
		localAlive:    true,
		tunnelAlive:   true,
	}
}

// RecordHeartbeat records a successful heartbeat check with measured latency and component vitality flags.
func (hm *HealthMonitor) RecordHeartbeat(latencyMs int64, localAlive, tunnelAlive bool) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.lastHeartbeat = time.Now().UTC()
	hm.latencyMs = latencyMs
	hm.localAlive = localAlive
	hm.tunnelAlive = tunnelAlive
}

// GetHealth returns current deployment health details.
func (hm *HealthMonitor) GetHealth() DeploymentHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	status := "healthy"
	if !hm.localAlive || !hm.tunnelAlive || time.Since(hm.lastHeartbeat) > 30*time.Second {
		status = "unhealthy"
	}

	return DeploymentHealth{
		Status:      status,
		TunnelAlive: hm.tunnelAlive,
		LocalAlive:  hm.localAlive,
		SSLValid:    true,
		LatencyMs:   hm.latencyMs,
		LastCheck:   hm.lastHeartbeat,
	}
}

// GetLastHeartbeat returns the timestamp of the last recorded heartbeat.
func (hm *HealthMonitor) GetLastHeartbeat() time.Time {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.lastHeartbeat
}

// GetLatencyMs returns the current measured tunnel latency in milliseconds.
func (hm *HealthMonitor) GetLatencyMs() int64 {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.latencyMs
}
