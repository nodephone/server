package openapi

import "net/http"

// RegisterRoutes registers OpenAPI endpoints (/docs, /docs/openapi.json, /docs/routes) onto the HTTP ServeMux.
func RegisterRoutes(mux *http.ServeMux, handler *OpenAPIHandler) {
	if mux == nil || handler == nil {
		return
	}

	wrapper := &hOpenAPIHandlerWrapper{OpenAPIHandler: handler}

	mux.HandleFunc("/docs", wrapper.RouteDocs)
	mux.HandleFunc("/docs/", wrapper.RouteDocs)
}
