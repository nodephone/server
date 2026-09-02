package openapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

// OpenAPIHandler exposes public endpoints for OpenAPI 3.1.0 JSON, route metadata, and interactive documentation UI.
type OpenAPIHandler struct {
	engine *Engine
}

// NewOpenAPIHandler creates a new OpenAPIHandler instance.
func NewOpenAPIHandler(engine *Engine) *OpenAPIHandler {
	return &OpenAPIHandler{
		engine: engine,
	}
}

// Engine returns the underlying Engine instance.
func (h *OpenAPIHandler) Engine() *Engine {
	return h.engine
}

// RouteDocs routes HTTP requests for /docs, /docs/openapi.json, and /docs/routes.
func (h *hOpenAPIHandlerWrapper) RouteDocs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")

	switch path {
	case "/docs/openapi.json":
		h.ServeOpenAPISpec(w, r)
	case "/docs/routes":
		h.ServeRoutesMetadata(w, r)
	case "/docs":
		h.ServeInteractiveDocs(w, r)
	default:
		if strings.HasPrefix(path, "/docs") {
			h.ServeInteractiveDocs(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

type hOpenAPIHandlerWrapper struct {
	*OpenAPIHandler
}

// ServeOpenAPISpec handles GET /docs/openapi.json.
func (h *OpenAPIHandler) ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	bytes := h.engine.GetSpecBytes()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bytes)
}

// ServeRoutesMetadata handles GET /docs/routes.
func (h *OpenAPIHandler) ServeRoutesMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	routes := h.engine.GetRoutes()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"total_routes": len(routes),
		"routes":       routes,
	})
}

// ServeInteractiveDocs handles GET /docs serving an interactive API explorer UI.
func (h *OpenAPIHandler) ServeInteractiveDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NodePhone Server — Interactive API Documentation</title>
  <script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
  <style>
    body { margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background-color: #0f172a; }
  </style>
</head>
<body>
  <rapi-doc
    spec-url="/docs/openapi.json"
    theme="dark"
    bg-color="#0f172a"
    text-color="#f8fafc"
    primary-color="#38bdf8"
    nav-bg-color="#1e293b"
    nav-hover-bg-color="#334155"
    nav-accent-color="#38bdf8"
    render-style="view"
    show-header="true"
    show-info="true"
    allow-try="true"
    allow-authentication="true"
    allow-server-selection="true"
    api-key-name="Authorization"
    api-key-location="header"
  >
  </rapi-doc>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
