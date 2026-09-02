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
			user := authenticateRequest(r, service)
			if user == nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthMiddleware inspects incoming request headers for auth credentials if present,
// injects the User into context if valid, but allows unauthenticated requests to continue.
func OptionalAuthMiddleware(service *AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := authenticateRequest(r, service)
			if user != nil {
				ctx := context.WithValue(r.Context(), userContextKey, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authenticateRequest(r *http.Request, service *AuthService) *User {
	if service == nil {
		return nil
	}

	// 1. Check X-API-Key header
	apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if apiKey != "" {
		user, err := service.ValidateAPIKey(r.Context(), apiKey)
		if err == nil && user != nil {
			return user
		}
	}

	// 2. Check Authorization header (Bearer <jwt> or Bearer <api_key>)
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			tokenOrKey := strings.TrimSpace(parts[1])

			if strings.HasPrefix(tokenOrKey, "np_live_") {
				user, err := service.ValidateAPIKey(r.Context(), tokenOrKey)
				if err == nil && user != nil {
					return user
				}
			} else {
				claims, parseErr := service.JWTManager().ParseToken(tokenOrKey)
				if parseErr == nil && claims != nil && claims.TokenType == "access" {
					user, err := service.GetMe(r.Context(), claims.UserID)
					if err == nil && user != nil {
						return user
					}
				}
			}
		}
	}

	return nil
}

// UserFromContext extracts the authenticated *User from request context.
func UserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(userContextKey).(*User)
	return user, ok
}
