package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid or expired JWT token")
)

// Claims represents custom JWT claims for NodePhone authentication.
type Claims struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username,omitempty"`
	Role      string `json:"role,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

// JWTManager handles signing and parsing of JWT access and refresh tokens.
type JWTManager struct {
	secret []byte
}

// NewJWTManager creates a new JWTManager instance with the specified secret string.
func NewJWTManager(secret string) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
	}
}

// GenerateAccessToken generates a short-lived access JWT (default 15 minutes).
func (m *JWTManager) GenerateAccessToken(userID, username, role string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	claims := &Claims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "nodephone",
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedStr, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign access token: %w", err)
	}
	return signedStr, expiresAt, nil
}

// GenerateRefreshToken generates a long-lived refresh JWT (default 7 days).
func (m *JWTManager) GenerateRefreshToken(userID, sessionID string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	claims := &Claims{
		UserID:    userID,
		SessionID: sessionID,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "nodephone",
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedStr, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign refresh token: %w", err)
	}
	return signedStr, expiresAt, nil
}

// ParseToken validates and extracts claims from a JWT token string.
func (m *JWTManager) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
