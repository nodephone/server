package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/database"
)

func TestArgon2idHashing(t *testing.T) {
	password := "SecretPassword123!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("expected argon2id hash format, got %s", hash)
	}

	match, err := auth.VerifyPassword(password, hash)
	if err != nil || !match {
		t.Errorf("expected password verification success, got match=%v err=%v", match, err)
	}

	wrongMatch, err := auth.VerifyPassword("WrongPassword!", hash)
	if err != nil || wrongMatch {
		t.Errorf("expected password verification failure, got match=%v err=%v", wrongMatch, err)
	}
}

func TestJWTManager(t *testing.T) {
	jwtSecret := "supersecretjwtkey1234567890123456"
	mgr := auth.NewJWTManager(jwtSecret)

	accessToken, _, err := mgr.GenerateAccessToken("user123", "alice", "admin", 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	claims, err := mgr.ParseToken(accessToken)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}

	if claims.UserID != "user123" || claims.Username != "alice" || claims.Role != "admin" || claims.TokenType != "access" {
		t.Errorf("unexpected access token claims: %+v", claims)
	}

	refreshToken, _, err := mgr.GenerateRefreshToken("user123", "sess456", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}

	refClaims, err := mgr.ParseToken(refreshToken)
	if err != nil {
		t.Fatalf("ParseToken refresh token failed: %v", err)
	}

	if refClaims.UserID != "user123" || refClaims.SessionID != "sess456" || refClaims.TokenType != "refresh" {
		t.Errorf("unexpected refresh token claims: %+v", refClaims)
	}

	// Test invalid secret token validation failure
	wrongMgr := auth.NewJWTManager("wrongsecret12345678901234567890")
	_, err = wrongMgr.ParseToken(accessToken)
	if err == nil {
		t.Error("expected ParseToken to fail with mismatched secret")
	}
}

func TestAuthServiceFlow(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "auth_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	jwtSecret := "testjwtsecretkey1234567890123456"
	var logBuf bytes.Buffer
	service := auth.NewAuthService(db, jwtSecret, &logBuf)

	// 1. SignUp
	signUpReq := auth.SignUpRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "Password123!",
	}
	user, err := service.SignUp(ctx, signUpReq)
	if err != nil {
		t.Fatalf("SignUp failed: %v", err)
	}
	if user.Username != "bob" || user.Email != "bob@example.com" {
		t.Errorf("unexpected user profile: %+v", user)
	}

	// Test duplicate signup failure
	_, err = service.SignUp(ctx, signUpReq)
	if err == nil {
		t.Error("expected duplicate SignUp to fail")
	}

	// 2. LogIn
	loginReq := auth.LoginRequest{
		Login:    "bob@example.com",
		Password: "Password123!",
	}
	authResp, err := service.LogIn(ctx, loginReq)
	if err != nil {
		t.Fatalf("LogIn failed: %v", err)
	}
	if authResp.AccessToken == "" || authResp.RefreshToken == "" {
		t.Error("expected access and refresh tokens returned")
	}

	// Test wrong password login failure
	_, err = service.LogIn(ctx, auth.LoginRequest{Login: "bob", Password: "WrongPassword"})
	if err == nil {
		t.Error("expected wrong password LogIn to fail")
	}

	// 3. RefreshToken
	newAuthResp, err := service.RefreshToken(ctx, authResp.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if newAuthResp.AccessToken == "" || newAuthResp.RefreshToken == "" {
		t.Error("expected new access and refresh tokens")
	}

	// 4. GetMe
	profile, err := service.GetMe(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if profile.ID != user.ID || profile.Username != "bob" {
		t.Errorf("unexpected profile: %+v", profile)
	}

	// 5. Create & Validate API Key
	keyResp, err := service.CreateAPIKey(ctx, user.ID, "CLI Key")
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}
	if !strings.HasPrefix(keyResp.APIKey, "np_live_") {
		t.Errorf("expected API key prefix 'np_live_', got %s", keyResp.APIKey)
	}

	apiKeyUser, err := service.ValidateAPIKey(ctx, keyResp.APIKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}
	if apiKeyUser.ID != user.ID {
		t.Errorf("expected user ID %s, got %s", user.ID, apiKeyUser.ID)
	}

	// 6. LogOut
	if err := service.LogOut(ctx, newAuthResp.RefreshToken); err != nil {
		t.Fatalf("LogOut failed: %v", err)
	}

	// Verify refresh token fails after logout
	_, err = service.RefreshToken(ctx, newAuthResp.RefreshToken)
	if err == nil {
		t.Error("expected RefreshToken to fail after LogOut")
	}
}

func TestAuthAPIEndpointsAndMiddleware(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "auth_api_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	service := auth.NewAuthService(db, "testsecret123456789012345678901234", nil)
	handler := auth.NewAuthHandler(service)
	authMW := auth.AuthMiddleware(service)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/signup", handler.SignUp)
	mux.HandleFunc("/api/auth/login", handler.LogIn)
	mux.HandleFunc("/api/auth/logout", handler.LogOut)
	mux.HandleFunc("/api/auth/refresh", handler.Refresh)
	mux.Handle("/api/auth/me", authMW(http.HandlerFunc(handler.Me)))
	mux.Handle("/api/auth/keys", authMW(http.HandlerFunc(handler.CreateAPIKey)))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// 1. SignUp
	signUpBody, _ := json.Marshal(auth.SignUpRequest{
		Username: "charlie",
		Email:    "charlie@example.com",
		Password: "MySecretPassword1!",
	})
	resp, err := client.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewBuffer(signUpBody))
	if err != nil {
		t.Fatalf("SignUp HTTP POST failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. LogIn
	loginBody, _ := json.Marshal(auth.LoginRequest{
		Login:    "charlie",
		Password: "MySecretPassword1!",
	})
	resp, err = client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("LogIn HTTP POST failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}

	var authResp auth.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		t.Fatalf("failed to decode AuthResponse: %v", err)
	}
	resp.Body.Close()

	// 3. /api/auth/me without header -> 401 Unauthorized
	resp, err = client.Get(ts.URL + "/api/auth/me")
	if err != nil {
		t.Fatalf("Get /me failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. /api/auth/me with Bearer JWT Access Token -> 200 OK
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Get /me with token failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK with Bearer token, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. POST /api/auth/keys with Bearer JWT -> 201 Created
	keyReqBody, _ := json.Marshal(auth.CreateAPIKeyRequest{Name: "Dev Laptop"})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/auth/keys", bytes.NewBuffer(keyReqBody))
	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /keys failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}

	var keyResp auth.APIKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
		t.Fatalf("failed to decode APIKeyResponse: %v", err)
	}
	resp.Body.Close()

	// 6. /api/auth/me with X-API-Key header -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	req.Header.Set("X-API-Key", keyResp.APIKey)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Get /me with X-API-Key failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK with X-API-Key, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
