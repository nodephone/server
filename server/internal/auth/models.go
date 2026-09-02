package auth

import "time"

// User represents a user identity record in NodePhone.
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Session represents an active authenticated user session.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKey represents an API key credential associated with a user.
type APIKey struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	KeyHash   string    `json:"key_hash"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SignUpRequest holds payload for user registration.
type SignUpRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest holds payload for user authentication.
type LoginRequest struct {
	Login    string `json:"login"` // Username or Email
	Password string `json:"password"`
}

// RefreshRequest holds payload for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// CreateAPIKeyRequest holds payload for generating an API key.
type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

// AuthResponse holds the response payload returned on successful authentication.
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         *User     `json:"user"`
}

// APIKeyResponse holds the response payload when a new API key is created.
type APIKeyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key"`
	CreatedAt time.Time `json:"created_at"`
}
