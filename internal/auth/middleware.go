package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const userContextKey contextKey = "user"

// AuthMiddleware inspects incoming request headers for Bearer JWT access tokens or API keys (X-API-Key or Authorization header)
// and injects the authenticated User into the request context.
func AuthMiddleware(service *AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user *User
			var err error

			// 1. Check X-API-Key header
			apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if apiKey != "" {
				user, err = service.ValidateAPIKey(r.Context(), apiKey)
				if err == nil && user != nil {
					ctx := context.WithValue(r.Context(), userContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// 2. Check Authorization header (Bearer <jwt> or Bearer <api_key>)
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
					tokenOrKey := strings.TrimSpace(parts[1])

					if strings.HasPrefix(tokenOrKey, "np_live_") {
						// API Key passed as Bearer token
						user, err = service.ValidateAPIKey(r.Context(), tokenOrKey)
					} else {
						// JWT Access Token
						claims, parseErr := service.JWTManager().ParseToken(tokenOrKey)
						if parseErr == nil && claims != nil && claims.TokenType == "access" {
							user, err = service.GetMe(r.Context(), claims.UserID)
						}
					}

					if err == nil && user != nil {
						ctx := context.WithValue(r.Context(), userContextKey, user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			// Unauthorized response
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		})
	}
}

// UserFromContext extracts the authenticated *User from request context.
func UserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(userContextKey).(*User)
	return user, ok
}
