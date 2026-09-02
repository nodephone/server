// Package api provides the HTTP API engine, router, middleware, and request handlers for NodePhone.
package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/nodephone/server/internal/config"
)

// Handler contains dependencies for HTTP request handlers.
type Handler struct {
	cfg     *config.Config
	version string
}

// NewHandler creates a new Handler instance with the provided config and version.
func NewHandler(cfg *config.Config, version string) *Handler {
	return &Handler{
		cfg:     cfg,
		version: version,
	}
}

// writeJSON writes a JSON payload with the specified HTTP status code.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
		}
	}
}

// RootResponse defines the payload returned by the GET / endpoint.
type RootResponse struct {
	App     string `json:"app"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// HandleRoot handles GET / requests.
func (h *Handler) HandleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Endpoint not found"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	appName := "NodePhone Server"
	if h.cfg != nil && h.cfg.Server.Name != "" {
		appName = h.cfg.Server.Name
	}

	res := RootResponse{
		App:     appName,
		Version: h.version,
		Status:  "running",
	}
	writeJSON(w, http.StatusOK, res)
}

// HealthResponse defines the payload returned by the GET /health endpoint.
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// HandleHealth handles GET /health requests.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	res := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, res)
}

// VersionResponse defines the payload returned by the GET /version endpoint.
type VersionResponse struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// HandleVersion handles GET /version requests.
func (h *Handler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	res := VersionResponse{
		Version:   h.version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	writeJSON(w, http.StatusOK, res)
}

// ReadyResponse defines the payload returned by the GET /ready endpoint.
type ReadyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// HandleReady handles GET /ready requests.
func (h *Handler) HandleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	checks := map[string]string{
		"config":  "ok",
		"storage": "ok",
	}

	res := ReadyResponse{
		Status: "ready",
		Checks: checks,
	}
	writeJSON(w, http.StatusOK, res)
}
