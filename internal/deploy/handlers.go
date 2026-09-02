package deploy

import (
	"encoding/json"
	"net/http"
)

// DeployHandler exposes REST API endpoints for deployment status, health probes, and custom domain management.
type DeployHandler struct {
	engine *DeploymentEngine
}

// NewDeployHandler creates a new DeployHandler instance.
func NewDeployHandler(engine *DeploymentEngine) *DeployHandler {
	return &DeployHandler{
		engine: engine,
	}
}

// Engine returns the underlying DeploymentEngine.
func (h *DeployHandler) Engine() *DeploymentEngine {
	return h.engine
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	writeJSONResponse(w, statusCode, map[string]string{"error": message})
}

// GetStatus handles GET /deploy/status.
func (h *DeployHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	status := h.engine.GetStatus()
	writeJSONResponse(w, http.StatusOK, status)
}

// GetHealth handles GET /deploy/health.
func (h *DeployHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	health := h.engine.GetHealth()
	statusCode := http.StatusOK
	if health.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSONResponse(w, statusCode, health)
}

// RouteDomain handles GET /deploy/domain and POST /deploy/domain.
func (h *DeployHandler) RouteDomain(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetDomain(w, r)
	case http.MethodPost:
		h.BindDomain(w, r)
	default:
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// GetDomain handles GET /deploy/domain.
func (h *DeployHandler) GetDomain(w http.ResponseWriter, r *http.Request) {
	domainInfo := h.engine.GetDomainInfo()
	if domainInfo == nil {
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"domain": nil,
			"status": "unbound",
		})
		return
	}
	writeJSONResponse(w, http.StatusOK, domainInfo)
}

// BindDomain handles POST /deploy/domain.
func (h *DeployHandler) BindDomain(w http.ResponseWriter, r *http.Request) {
	var req BindDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	domainInfo, err := h.engine.BindDomain(req.Domain)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSONResponse(w, http.StatusOK, domainInfo)
}
