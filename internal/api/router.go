package api

import (
	"io"
	"net/http"
	"time"
)

// DefaultTimeout defines the default request context timeout for the HTTP API engine.
const DefaultTimeout = 15 * time.Second

// NewRouter sets up the HTTP router with registered endpoints and global middleware.
func NewRouter(h *Handler, out io.Writer, requestTimeout time.Duration) http.Handler {
	if requestTimeout <= 0 {
		requestTimeout = DefaultTimeout
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", h.HandleRoot)
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/version", h.HandleVersion)
	mux.HandleFunc("/ready", h.HandleReady)

	// Middleware chain: Recovery -> Logging -> Timeout
	return Chain(
		mux,
		RecoveryMiddleware(out),
		LoggingMiddleware(out),
		TimeoutMiddleware(requestTimeout),
	)
}
