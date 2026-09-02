package realtime_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/kernel"
	"github.com/nodephone/server/internal/realtime"
	"golang.org/x/net/websocket"
)

func TestRealtimeSubsystemFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "realtime_test_*")
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

	// 1. SignUp & Login User
	signUpBody, _ := json.Marshal(auth.SignUpRequest{Username: "wsuser", Email: "ws@example.com", Password: "Password123!"})
	_, _ = client.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewBuffer(signUpBody))

	loginBody, _ := json.Marshal(auth.LoginRequest{Login: "wsuser", Password: "Password123!"})
	resp, err := client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	var loginRes auth.AuthResponse
	_ = json.NewDecoder(resp.Body).Decode(&loginRes)
	resp.Body.Close()

	token := loginRes.AccessToken

	// 2. Connect via WebSocket to /realtime?token=...
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/realtime?token=" + token
	ws, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket.Dial failed: %v", err)
	}
	defer ws.Close()

	// Read welcome message
	var welcomeMsg realtime.EventMessage
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := websocket.JSON.Receive(ws, &welcomeMsg); err != nil {
		t.Fatalf("failed to receive welcome message: %v", err)
	}

	if welcomeMsg.Type != realtime.MessageTypeSystem {
		t.Errorf("expected system message type, got %s", welcomeMsg.Type)
	}

	// 3. Query Online User Presence Endpoint
	resp, err = client.Get(ts.URL + "/api/realtime/presence")
	if err != nil {
		t.Fatalf("GET /api/realtime/presence failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for presence, got %d", resp.StatusCode)
	}
	var presenceResult map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&presenceResult)
	resp.Body.Close()

	if total, ok := presenceResult["total_connections"].(float64); !ok || total < 1 {
		t.Errorf("expected total_connections >= 1, got %v", presenceResult["total_connections"])
	}
}
