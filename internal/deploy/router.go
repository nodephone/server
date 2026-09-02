package deploy

import "net/http"

// RegisterRoutes registers deployment endpoints (/deploy/status, /deploy/health, /deploy/domain) onto the HTTP ServeMux.
func RegisterRoutes(mux *http.ServeMux, handler *DeployHandler) {
	if mux == nil || handler == nil {
		return
	}

	mux.HandleFunc("/deploy/status", handler.GetStatus)
	mux.HandleFunc("/deploy/health", handler.GetHealth)
	mux.HandleFunc("/deploy/domain", handler.RouteDomain)
}
