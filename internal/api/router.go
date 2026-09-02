package api

import (
	"io"
	"net/http"
	"time"

	"github.com/nodephone/server/internal/auth"
)

// DefaultTimeout defines the default request context timeout for the HTTP API engine.
const DefaultTimeout = 15 * time.Second

// NewRouter sets up the HTTP router with registered endpoints and global middleware.
func NewRouter(h *Handler, authHandler *auth.AuthHandler, out io.Writer, requestTimeout time.Duration) http.Handler {
	if requestTimeout <= 0 {
		requestTimeout = DefaultTimeout
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", h.HandleRoot)
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/version", h.HandleVersion)
	mux.HandleFunc("/ready", h.HandleReady)

	if authHandler != nil {
		mux.HandleFunc("/api/auth/signup", authHandler.SignUp)
		mux.HandleFunc("/api/auth/login", authHandler.LogIn)
		mux.HandleFunc("/api/auth/logout", authHandler.LogOut)
		mux.HandleFunc("/api/auth/refresh", authHandler.Refresh)

		authMW := auth.AuthMiddleware(authHandler.Service())
		mux.Handle("/api/auth/me", authMW(http.HandlerFunc(authHandler.Me)))
		mux.Handle("/api/auth/keys", authMW(http.HandlerFunc(authHandler.CreateAPIKey)))
	}

	// Middleware chain: Recovery -> Logging -> Timeout
	return Chain(
		mux,
		RecoveryMiddleware(out),
		LoggingMiddleware(out),
		TimeoutMiddleware(requestTimeout),
	)
}
