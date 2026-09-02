package api

import (
	"io"
	"net/http"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/functions"
	"github.com/nodephone/server/internal/permissions"
	"github.com/nodephone/server/internal/realtime"
	"github.com/nodephone/server/internal/storage"
)

// DefaultTimeout defines the default request context timeout for the HTTP API engine.
const DefaultTimeout = 15 * time.Second

// NewRouter sets up the HTTP router with registered endpoints and global middleware.
func NewRouter(h *Handler, authHandler *auth.AuthHandler, storageHandler *storage.StorageHandler, realtimeHandler *realtime.RealtimeHandler, functionHandler *functions.FunctionHandler, policyHandler *permissions.PolicyHandler, out io.Writer, requestTimeout time.Duration) http.Handler {
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

	if storageHandler != nil && authHandler != nil {
		authMW := auth.AuthMiddleware(authHandler.Service())
		optAuthMW := auth.OptionalAuthMiddleware(authHandler.Service())

		mux.Handle("/api/storage/buckets", authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				storageHandler.ListBuckets(w, r)
			} else {
				storageHandler.CreateBucket(w, r)
			}
		})))

		mux.HandleFunc("/api/storage/buckets/", func(w http.ResponseWriter, r *http.Request) {
			optAuthMW(http.HandlerFunc(storageHandler.RouteBucketObject)).ServeHTTP(w, r)
		})
	}

	if realtimeHandler != nil {
		mux.HandleFunc("/realtime", realtimeHandler.ServeWS)
		mux.HandleFunc("/api/realtime/presence", realtimeHandler.GetPresence)
	}

	if functionHandler != nil {
		mux.HandleFunc("/api/functions", functionHandler.RouteFunctions)
		mux.HandleFunc("/api/functions/", functionHandler.RouteFunctions)
	}

	if policyHandler != nil && authHandler != nil {
		authMW := auth.AuthMiddleware(authHandler.Service())
		mux.Handle("/api/permissions/policies", authMW(http.HandlerFunc(policyHandler.RoutePolicies)))
		mux.Handle("/api/permissions/policies/", authMW(http.HandlerFunc(policyHandler.RoutePolicies)))
	}

	// Middleware chain: Recovery -> Logging -> Timeout
	return Chain(
		mux,
		RecoveryMiddleware(out),
		LoggingMiddleware(out),
		TimeoutMiddleware(requestTimeout),
	)
}
