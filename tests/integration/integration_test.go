package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/kernel"
	"github.com/nodephone/server/internal/permissions"
	"github.com/nodephone/server/internal/realtime"
	"github.com/nodephone/server/internal/storage"
	"golang.org/x/net/websocket"
)

func TestFullKernelE2EIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "e2e_integration_*")
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

	// 1. Sign Up Admin User & Normal User
	adminSignUp, _ := json.Marshal(auth.SignUpRequest{Username: "e2eadmin", Email: "admin@e2e.com", Password: "Password123!"})
	_, _ = client.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewBuffer(adminSignUp))

	// Promote e2eadmin to admin in SQLite
	_, _ = k.DB().ExecContext(t.Context(), "UPDATE users SET role = 'admin' WHERE username = 'e2eadmin'")

	adminLogin, _ := json.Marshal(auth.LoginRequest{Login: "e2eadmin", Password: "Password123!"})
	adminResp, err := client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(adminLogin))
	if err != nil {
		t.Fatalf("Admin login failed: %v", err)
	}
	var adminAuth auth.AuthResponse
	_ = json.NewDecoder(adminResp.Body).Decode(&adminAuth)
	adminResp.Body.Close()

	userSignUp, _ := json.Marshal(auth.SignUpRequest{Username: "e2euser", Email: "user@e2e.com", Password: "Password123!"})
	_, _ = client.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewBuffer(userSignUp))

	userLogin, _ := json.Marshal(auth.LoginRequest{Login: "e2euser", Password: "Password123!"})
	userResp, err := client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(userLogin))
	if err != nil {
		t.Fatalf("User login failed: %v", err)
	}
	var userAuth auth.AuthResponse
	_ = json.NewDecoder(userResp.Body).Decode(&userAuth)
	userResp.Body.Close()

	// 2. Admin creates Row-Level Security Policy
	policyReq, _ := json.Marshal(permissions.CreatePolicyRequest{
		TableName:  "documents",
		Action:     "SELECT",
		Role:       "user",
		Expression: "user.id == row.owner_id",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/permissions/policies", bytes.NewBuffer(policyReq))
	req.Header.Set("Authorization", "Bearer "+adminAuth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("Admin policy creation failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 3. User creates Storage Bucket and Uploads File
	bktReq, _ := json.Marshal(map[string]interface{}{"name": "user-vault", "public": false})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets", bytes.NewBuffer(bktReq))
	req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("Bucket creation failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	var mpBuf bytes.Buffer
	writer := multipart.NewWriter(&mpBuf)
	part, _ := writer.CreateFormFile("file", "notes.txt")
	_, _ = part.Write([]byte("E2E Integration Secret Document Content"))
	_ = writer.Close()

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets/user-vault/objects", &mpBuf)
	req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("File upload failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Stream Download Object
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/buckets/user-vault/objects/notes.txt", nil)
	req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("File download failed: %v, status: %d", err, resp.StatusCode)
	}
	content, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(content) != "E2E Integration Secret Document Content" {
		t.Errorf("unexpected file content: %q", string(content))
	}

	// 5. Generate Signed Access URL & Fetch Object
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets/user-vault/objects/notes.txt/sign", nil)
	req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Sign URL failed: %v, status: %d", err, resp.StatusCode)
	}
	var signRes storage.SignedURLResponse
	_ = json.NewDecoder(resp.Body).Decode(&signRes)
	resp.Body.Close()

	signedURL := ts.URL + "/api/storage/buckets/user-vault/objects/notes.txt?token=" + signRes.Token
	resp, err = client.Get(signedURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Signed URL access failed: %v, status: %d", err, resp.StatusCode)
	}
	signedContent, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(signedContent) != "E2E Integration Secret Document Content" {
		t.Errorf("unexpected signed content: %q", string(signedContent))
	}

	// 6. Connect WebSocket and Verify Presence
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/realtime?token=" + userAuth.AccessToken
	ws, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer ws.Close()

	var welcomeMsg realtime.EventMessage
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := websocket.JSON.Receive(ws, &welcomeMsg); err != nil {
		t.Fatalf("WebSocket receive failed: %v", err)
	}

	// 7. Verify OpenAPI Spec & Route Meta
	resp, err = client.Get(ts.URL + "/docs/openapi.json")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("OpenAPI spec endpoint failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()
}
