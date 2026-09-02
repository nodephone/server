package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/kernel"
)

func TestAuthenticationFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	stopCh := make(chan os.Signal, 1)
	k := kernel.New(nil, kernel.WithDataDir(tempDir), kernel.WithStopChannel(stopCh), kernel.WithNonBlocking(true))
	if err := k.Boot(); err != nil {
		t.Fatalf("kernel.Boot failed: %v", err)
	}
	defer k.Close()

	ts := httptest.NewServer(k.APIServer().Handler())
	defer ts.Close()

	client := ts.Client()

	// 1. Sign Up User
	signUpPayload, _ := json.Marshal(auth.SignUpRequest{
		Username: "authuser",
		Email:    "authuser@example.com",
		Password: "Password123!",
	})
	resp, err := client.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewBuffer(signUpPayload))
	if err != nil {
		t.Fatalf("POST /api/auth/signup failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}
	var signUpResult map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&signUpResult)
	resp.Body.Close()

	// 2. Log In User
	loginPayload, _ := json.Marshal(auth.LoginRequest{
		Login:    "authuser",
		Password: "Password123!",
	})
	resp, err = client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil {
		t.Fatalf("POST /api/auth/login failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	var loginResult auth.AuthResponse
	_ = json.NewDecoder(resp.Body).Decode(&loginResult)
	resp.Body.Close()

	if loginResult.AccessToken == "" || loginResult.RefreshToken == "" {
		t.Fatal("expected non-empty access_token and refresh_token")
	}

	// 3. Invalid Password Log In -> 401 Unauthorized
	badLoginPayload, _ := json.Marshal(auth.LoginRequest{
		Login:    "authuser",
		Password: "WrongPassword!",
	})
	resp, err = client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(badLoginPayload))
	if err != nil {
		t.Fatalf("POST /api/auth/login with bad password failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized for bad password, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. GET /api/auth/me (Authenticated Profile)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+loginResult.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/auth/me failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for /api/auth/me, got %d", resp.StatusCode)
	}
	var meResult struct {
		User auth.User `json:"user"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&meResult)
	resp.Body.Close()

	if meResult.User.Username != "authuser" {
		t.Errorf("expected username 'authuser', got %q", meResult.User.Username)
	}

	// 5. POST /api/auth/keys (Generate API Key)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/auth/keys", nil)
	req.Header.Set("Authorization", "Bearer "+loginResult.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/auth/keys failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created for API Key, got %d", resp.StatusCode)
	}
	var apiKeyResult map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&apiKeyResult)
	resp.Body.Close()

	if apiKeyStr, ok := apiKeyResult["api_key"].(string); !ok || len(apiKeyStr) < 10 {
		t.Errorf("expected valid api_key string, got %v", apiKeyResult["api_key"])
	}

	// 6. POST /api/auth/refresh (Issue New Tokens)
	refreshPayload, _ := json.Marshal(map[string]string{"refresh_token": loginResult.RefreshToken})
	resp, err = client.Post(ts.URL+"/api/auth/refresh", "application/json", bytes.NewBuffer(refreshPayload))
	if err != nil {
		t.Fatalf("POST /api/auth/refresh failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for refresh, got %d", resp.StatusCode)
	}
	var refreshedResult auth.AuthResponse
	_ = json.NewDecoder(resp.Body).Decode(&refreshedResult)
	resp.Body.Close()

	if refreshedResult.AccessToken == "" {
		t.Error("expected new access_token from refresh endpoint")
	}

	// 7. POST /api/auth/logout (Revoke Session)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+loginResult.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/auth/logout failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for logout, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
