package realtime

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nodephone/server/internal/auth"
	"golang.org/x/net/websocket"
)

// RealtimeHandler exposes WebSocket connection upgrade endpoints and presence metadata APIs.
type RealtimeHandler struct {
	hub         *Hub
	authService *auth.AuthService
}

// NewRealtimeHandler creates a new RealtimeHandler instance.
func NewRealtimeHandler(hub *Hub, authService *auth.AuthService) *RealtimeHandler {
	return &RealtimeHandler{
		hub:         hub,
		authService: authService,
	}
}

// Hub returns the underlying Hub instance.
func (h *RealtimeHandler) Hub() *Hub {
	return h.hub
}

// ServeWS handles WebSocket connection upgrade requests at /realtime.
func (h *RealtimeHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var user *auth.User
	var err error

	// 1. Check token in URL query parameter (?token=...)
	tokenParam := strings.TrimSpace(r.URL.Query().Get("token"))
	if tokenParam != "" {
		if strings.HasPrefix(tokenParam, "np_live_") {
			user, err = h.authService.ValidateAPIKey(ctx, tokenParam)
		} else {
			claims, parseErr := h.authService.JWTManager().ParseToken(tokenParam)
			if parseErr == nil && claims != nil && claims.TokenType == "access" {
				user, err = h.authService.GetMe(ctx, claims.UserID)
			}
		}
	}

	// 2. Check X-API-Key header if query token is absent
	if user == nil {
		apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if apiKey != "" {
			user, err = h.authService.ValidateAPIKey(ctx, apiKey)
		}
	}

	// 3. Check Authorization header (Bearer <token>)
	if user == nil {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				tokenOrKey := strings.TrimSpace(parts[1])
				if strings.HasPrefix(tokenOrKey, "np_live_") {
					user, err = h.authService.ValidateAPIKey(ctx, tokenOrKey)
				} else {
					claims, parseErr := h.authService.JWTManager().ParseToken(tokenOrKey)
					if parseErr == nil && claims != nil && claims.TokenType == "access" {
						user, err = h.authService.GetMe(ctx, claims.UserID)
					}
				}
			}
		}
	}

	// 4. Reject unauthenticated WebSocket connection attempts
	if user == nil || err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized WebSocket connection attempt"})
		return
	}

	// Upgrade HTTP connection to WebSocket frame connection
	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		client := NewClient(h.hub, ws, user)
		h.hub.Register(client)

		go client.WritePump()
		client.ReadPump()
	})

	wsHandler.ServeHTTP(w, r)
}

// GetPresence handles GET /api/realtime/presence requests.
func (h *RealtimeHandler) GetPresence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	presence := h.hub.GetPresenceInfo()
	count := h.hub.ActiveClientCount()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"total_connections": count,
		"online_users":      presence,
	})
}
