package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// AuthHandler exposes HTTP handler methods for authentication endpoints.
type AuthHandler struct {
	service *AuthService
}

// NewAuthHandler creates a new AuthHandler instance.
func NewAuthHandler(service *AuthService) *AuthHandler {
	return &AuthHandler{
		service: service,
	}
}

// Service returns the underlying AuthService.
func (h *AuthHandler) Service() *AuthService {
	return h.service
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

// SignUp handles POST /api/auth/signup requests.
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	user, err := h.service.SignUp(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			writeErrorResponse(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidInput) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to register user account")
		return
	}

	writeJSONResponse(w, http.StatusCreated, map[string]interface{}{
		"user": user,
	})
}

// LogIn handles POST /api/auth/login requests.
func (h *AuthHandler) LogIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	resp, err := h.service.LogIn(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrInvalidInput) {
			writeErrorResponse(w, http.StatusUnauthorized, "Invalid username or password")
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Authentication failed")
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

// LogOut handles POST /api/auth/logout requests.
func (h *AuthHandler) LogOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RefreshRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	token := strings.TrimSpace(req.RefreshToken)
	if token == "" {
		// Attempt to parse token from Authorization header if missing in body
		authHeader := r.Header.Get("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 {
			token = parts[1]
		}
	}

	_ = h.service.LogOut(r.Context(), token)
	writeJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// Refresh handles POST /api/auth/refresh requests.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Refresh token is required")
		return
	}

	resp, err := h.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrInvalidToken) {
			writeErrorResponse(w, http.StatusUnauthorized, "Invalid or expired refresh token")
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to refresh token")
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

// Me handles GET /api/auth/me requests.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user, ok := UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

// CreateAPIKey handles POST /api/auth/keys requests.
func (h *AuthHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user, ok := UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req CreateAPIKeyRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	apiKeyResp, err := h.service.CreateAPIKey(r.Context(), user.ID, req.Name)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate API key")
		return
	}

	writeJSONResponse(w, http.StatusCreated, apiKeyResp)
}
