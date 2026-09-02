package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nodephone/server/internal/database"
)

var (
	ErrUserExists         = errors.New("username or email already registered")
	ErrInvalidCredentials = errors.New("invalid login credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionExpired     = errors.New("session expired or invalid")
	ErrInvalidAPIKey      = errors.New("invalid API key")
	ErrInvalidInput       = errors.New("invalid input parameters")
)

// AuthService manages user registration, login, JWT token issuing, sessions, and API key management.
type AuthService struct {
	db     *database.DB
	jwtMgr *JWTManager
	out    io.Writer
}

// NewAuthService creates a new AuthService instance.
func NewAuthService(db *database.DB, jwtSecret string, out io.Writer) *AuthService {
	if out == nil {
		out = io.Discard
	}
	return &AuthService{
		db:     db,
		jwtMgr: NewJWTManager(jwtSecret),
		out:    out,
	}
}

// JWTManager returns the underlying JWTManager.
func (s *AuthService) JWTManager() *JWTManager {
	return s.jwtMgr
}

// SignUp registers a new user account with Argon2id password hashing.
func (s *AuthService) SignUp(ctx context.Context, req SignUpRequest) (*User, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("%w: username, email, and password are required", ErrInvalidInput)
	}

	// Check if username or email already exists
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE username = ? OR email = ?", req.Username, req.Email).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if count > 0 {
		return nil, ErrUserExists
	}

	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	userID := uuid.New().String()
	now := time.Now().UTC()

	query := `INSERT INTO users (id, username, email, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'user', ?, ?)`
	_, err = s.db.ExecContext(ctx, query, userID, req.Username, req.Email, passwordHash, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user into database: %w", err)
	}

	fmt.Fprintf(s.out, "[AUTH] Created user account %s (%s)\n", req.Username, userID)

	return &User{
		ID:        userID,
		Username:  req.Username,
		Email:     req.Email,
		Role:      "user",
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// LogIn authenticates a user by username or email and password, issuing access & refresh JWT tokens.
func (s *AuthService) LogIn(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	req.Login = strings.TrimSpace(req.Login)
	req.Password = strings.TrimSpace(req.Password)

	if req.Login == "" || req.Password == "" {
		return nil, fmt.Errorf("%w: login credentials are required", ErrInvalidInput)
	}

	var user User
	var passwordHash string

	query := `SELECT id, username, email, password_hash, role, created_at, updated_at FROM users WHERE username = ? OR email = ?`
	err := s.db.QueryRowContext(ctx, query, req.Login, strings.ToLower(req.Login)).Scan(
		&user.ID, &user.Username, &user.Email, &passwordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	} else if err != nil {
		return nil, fmt.Errorf("failed to query user credentials: %w", err)
	}

	match, err := VerifyPassword(req.Password, passwordHash)
	if err != nil || !match {
		return nil, ErrInvalidCredentials
	}

	sessionID := uuid.New().String()
	accessToken, expiresAt, err := s.jwtMgr.GenerateAccessToken(user.ID, user.Username, user.Role, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshExpiresAt, err := s.jwtMgr.GenerateRefreshToken(user.ID, sessionID, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	sessQuery := `INSERT INTO sessions (id, user_id, token, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, sessQuery, sessionID, user.ID, refreshToken, refreshExpiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("failed to store user session: %w", err)
	}

	fmt.Fprintf(s.out, "[AUTH] User %s logged in successfully (session: %s)\n", user.Username, sessionID)

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		User:         &user,
	}, nil
}

// LogOut revokes an active user session.
func (s *AuthService) LogOut(ctx context.Context, sessionIDOrToken string) error {
	if sessionIDOrToken == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ? OR token = ?", sessionIDOrToken, sessionIDOrToken)
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	fmt.Fprintf(s.out, "[AUTH] Revoked session: %s\n", sessionIDOrToken)
	return nil
}

// RefreshToken validates a refresh JWT token, verifies active database session, and returns new tokens.
func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenStr string) (*AuthResponse, error) {
	claims, err := s.jwtMgr.ParseToken(refreshTokenStr)
	if err != nil {
		return nil, ErrSessionExpired
	}

	if claims.TokenType != "refresh" || claims.SessionID == "" {
		return nil, ErrSessionExpired
	}

	var session Session
	sessQuery := `SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE id = ?`
	err = s.db.QueryRowContext(ctx, sessQuery, claims.SessionID).Scan(
		&session.ID, &session.UserID, &session.Token, &session.ExpiresAt, &session.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) || session.ExpiresAt.Before(time.Now().UTC()) {
		return nil, ErrSessionExpired
	} else if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	user, err := s.GetMe(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	newAccessToken, expiresAt, err := s.jwtMgr.GenerateAccessToken(user.ID, user.Username, user.Role, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	newRefreshToken, newRefreshExpiresAt, err := s.jwtMgr.GenerateRefreshToken(user.ID, session.ID, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	// Update session token and expiration in database
	_, err = s.db.ExecContext(ctx, "UPDATE sessions SET token = ?, expires_at = ? WHERE id = ?", newRefreshToken, newRefreshExpiresAt, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update session token: %w", err)
	}

	return &AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    expiresAt,
		User:         user,
	}, nil
}

// GetMe retrieves user details by User ID.
func (s *AuthService) GetMe(ctx context.Context, userID string) (*User, error) {
	var user User
	query := `SELECT id, username, email, role, created_at, updated_at FROM users WHERE id = ?`
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID, &user.Username, &user.Email, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	return &user, nil
}

// CreateAPIKey generates a new secure random API key for a user and persists its SHA256 hash.
func (s *AuthService) CreateAPIKey(ctx context.Context, userID, name string) (*APIKeyResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Default API Key"
	}

	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random API key: %w", err)
	}

	rawKey := "np_live_" + hex.EncodeToString(rawBytes)
	keyHashBytes := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(keyHashBytes[:])

	keyID := uuid.New().String()
	now := time.Now().UTC()

	query := `INSERT INTO api_keys (id, user_id, key_hash, name, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query, keyID, userID, keyHash, name, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert API key: %w", err)
	}

	fmt.Fprintf(s.out, "[AUTH] Generated API key %q for user %s\n", name, userID)

	return &APIKeyResponse{
		ID:        keyID,
		Name:      name,
		APIKey:    rawKey,
		CreatedAt: now,
	}, nil
}

// ValidateAPIKey verifies a raw API key string against stored SHA256 hashes and returns the user.
func (s *AuthService) ValidateAPIKey(ctx context.Context, rawAPIKey string) (*User, error) {
	rawAPIKey = strings.TrimSpace(rawAPIKey)
	if !strings.HasPrefix(rawAPIKey, "np_live_") {
		return nil, ErrInvalidAPIKey
	}

	keyHashBytes := sha256.Sum256([]byte(rawAPIKey))
	keyHash := hex.EncodeToString(keyHashBytes[:])

	var user User
	query := `
SELECT u.id, u.username, u.email, u.role, u.created_at, u.updated_at
FROM api_keys k
JOIN users u ON k.user_id = u.id
WHERE k.key_hash = ?`

	err := s.db.QueryRowContext(ctx, query, keyHash).Scan(
		&user.ID, &user.Username, &user.Email, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidAPIKey
	} else if err != nil {
		return nil, fmt.Errorf("failed to query API key: %w", err)
	}

	return &user, nil
}
