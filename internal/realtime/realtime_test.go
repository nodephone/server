package realtime_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/database"
	"github.com/nodephone/server/internal/realtime"
	"golang.org/x/net/websocket"
)

func TestHubUnitOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := realtime.NewHub(nil)
	go hub.Run(ctx)

	u1 := &auth.User{ID: "usr-1", Username: "alice"}
	c1 := realtime.NewClient(hub, nil, u1)

	hub.Register(c1)
	time.Sleep(20 * time.Millisecond)

	if hub.ActiveClientCount() != 1 {
		t.Errorf("expected 1 active client, got %d", hub.ActiveClientCount())
	}

	online := hub.GetOnlineUsers()
	if len(online) != 1 || online[0].Username != "alice" {
		t.Errorf("expected online user alice, got %+v", online)
	}

	hub.Subscribe(c1, "lobby")
	time.Sleep(20 * time.Millisecond)

	if !c1.IsSubscribed("lobby") {
		t.Error("expected client to be subscribed to lobby")
	}

	hub.Unsubscribe(c1, "lobby")
	time.Sleep(20 * time.Millisecond)

	if c1.IsSubscribed("lobby") {
		t.Error("expected client to be unsubscribed from lobby")
	}

	hub.Unregister(c1)
	time.Sleep(20 * time.Millisecond)

	if hub.ActiveClientCount() != 0 {
		t.Errorf("expected 0 active clients after unregister, got %d", hub.ActiveClientCount())
	}
}

func readNextMatchingMessage(ws *websocket.Conn, msgType string, timeout time.Duration) (*realtime.EventMessage, error) {
	deadline := time.Now().Add(timeout)
	_ = ws.SetReadDeadline(deadline)

	for {
		var msg realtime.EventMessage
		err := websocket.JSON.Receive(ws, &msg)
		if err != nil {
			return nil, err
		}
		if msg.Type == msgType {
			return &msg, nil
		}
	}
}

func TestWebSocketEndpointAndMessaging(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "realtime_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	authService := auth.NewAuthService(db, "testjwtsecret1234567890123456", nil)
	hub := realtime.NewHub(nil)
	go hub.Run(ctx)

	realtimeHandler := realtime.NewRealtimeHandler(hub, authService)

	mux := http.NewServeMux()
	mux.HandleFunc("/realtime", realtimeHandler.ServeWS)
	mux.HandleFunc("/api/realtime/presence", realtimeHandler.GetPresence)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. Unauthenticated connection attempt -> 401 Unauthorized
	resp, err := ts.Client().Get(ts.URL + "/realtime")
	if err != nil {
		t.Fatalf("GET /realtime failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Create User 1 & User 2
	_, _ = authService.SignUp(ctx, auth.SignUpRequest{Username: "alice", Email: "alice@example.com", Password: "Password123!"})
	login1, err := authService.LogIn(ctx, auth.LoginRequest{Login: "alice", Password: "Password123!"})
	if err != nil {
		t.Fatalf("LogIn alice failed: %v", err)
	}

	_, _ = authService.SignUp(ctx, auth.SignUpRequest{Username: "bob", Email: "bob@example.com", Password: "Password123!"})
	login2, err := authService.LogIn(ctx, auth.LoginRequest{Login: "bob", Password: "Password123!"})
	if err != nil {
		t.Fatalf("LogIn bob failed: %v", err)
	}

	// 3. Connect Alice via WebSocket
	wsURL1 := strings.Replace(ts.URL, "http://", "ws://", 1) + "/realtime?token=" + login1.AccessToken
	ws1, err := websocket.Dial(wsURL1, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket.Dial Alice failed: %v", err)
	}
	defer ws1.Close()

	aliceWelcome, err := readNextMatchingMessage(ws1, realtime.MessageTypeSystem, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to receive welcome message for Alice: %v", err)
	}
	if !strings.Contains(fmt.Sprintf("%v", aliceWelcome.Payload), "Welcome") {
		t.Errorf("unexpected Alice welcome payload: %v", aliceWelcome.Payload)
	}

	// 4. Connect Bob via WebSocket
	wsURL2 := strings.Replace(ts.URL, "http://", "ws://", 1) + "/realtime?token=" + login2.AccessToken
	ws2, err := websocket.Dial(wsURL2, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket.Dial Bob failed: %v", err)
	}
	defer ws2.Close()

	bobWelcome, err := readNextMatchingMessage(ws2, realtime.MessageTypeSystem, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to receive welcome message for Bob: %v", err)
	}
	if !strings.Contains(fmt.Sprintf("%v", bobWelcome.Payload), "Welcome") {
		t.Errorf("unexpected Bob welcome payload: %v", bobWelcome.Payload)
	}

	// 5. Test Room Subscription & Broadcasting: Alice subscribes to "chat" room
	subFrame := realtime.EventMessage{
		Type: realtime.MessageTypeSubscribe,
		Room: "chat",
	}
	if err := websocket.JSON.Send(ws1, subFrame); err != nil {
		t.Fatalf("failed to send subscribe frame: %v", err)
	}

	aliceSubConf, err := readNextMatchingMessage(ws1, realtime.MessageTypeSystem, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to receive sub confirmation for Alice: %v", err)
	}
	if aliceSubConf.Room != "chat" {
		t.Errorf("expected sub confirmation for chat room, got %+v", aliceSubConf)
	}

	// Bob subscribes to "chat" room
	_ = websocket.JSON.Send(ws2, subFrame)
	bobSubConf, err := readNextMatchingMessage(ws2, realtime.MessageTypeSystem, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to receive sub confirmation for Bob: %v", err)
	}
	if bobSubConf.Room != "chat" {
		t.Errorf("expected sub confirmation for Bob, got %+v", bobSubConf)
	}

	// Alice publishes message to "chat" room
	pubFrame := realtime.EventMessage{
		Type:    realtime.MessageTypePublish,
		Room:    "chat",
		Payload: "Hello Bob!",
	}
	if err := websocket.JSON.Send(ws1, pubFrame); err != nil {
		t.Fatalf("failed to publish frame: %v", err)
	}

	// Bob receives published message from Alice
	bobRecv, err := readNextMatchingMessage(ws2, realtime.MessageTypePublish, 2*time.Second)
	if err != nil {
		t.Fatalf("Bob failed to receive published message: %v", err)
	}
	if fmt.Sprintf("%v", bobRecv.Payload) != "Hello Bob!" {
		t.Errorf("expected Bob to receive 'Hello Bob!', got %v", bobRecv.Payload)
	}

	// 6. Test Ping/Pong Heartbeat
	pingFrame := realtime.EventMessage{Type: realtime.MessageTypePing}
	if err := websocket.JSON.Send(ws1, pingFrame); err != nil {
		t.Fatalf("failed to send ping frame: %v", err)
	}

	pongResp, err := readNextMatchingMessage(ws1, realtime.MessageTypePong, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to receive pong frame: %v", err)
	}
	if pongResp.Type != realtime.MessageTypePong {
		t.Errorf("expected pong response, got %s", pongResp.Type)
	}
}
