package storage_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/kernel"
	"github.com/nodephone/server/internal/storage"
)

func TestStorageSubsystemFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage_test_*")
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

	// 1. SignUp User
	signUpBody, _ := json.Marshal(auth.SignUpRequest{Username: "storageuser", Email: "storage@example.com", Password: "Password123!"})
	_, _ = client.Post(ts.URL+"/api/auth/signup", "application/json", bytes.NewBuffer(signUpBody))

	loginBody, _ := json.Marshal(auth.LoginRequest{Login: "storageuser", Password: "Password123!"})
	resp, err := client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	var loginRes auth.AuthResponse
	_ = json.NewDecoder(resp.Body).Decode(&loginRes)
	resp.Body.Close()

	token := loginRes.AccessToken

	// 2. Create Bucket
	bucketPayload, _ := json.Marshal(map[string]interface{}{"name": "documents", "public": false})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets", bytes.NewBuffer(bucketPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Create bucket failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for bucket, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Upload File Object via Multipart
	var mpBuf bytes.Buffer
	writer := multipart.NewWriter(&mpBuf)
	part, _ := writer.CreateFormFile("file", "report.txt")
	_, _ = part.Write([]byte("NodePhone Storage Engine Test Document Content"))
	_ = writer.Close()

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets/documents/objects", &mpBuf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Upload object failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for object upload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Stream Download Object
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/buckets/documents/objects/report.txt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Download object failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for download, got %d", resp.StatusCode)
	}
	downloadContent, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(downloadContent) != "NodePhone Storage Engine Test Document Content" {
		t.Errorf("unexpected content: %q", string(downloadContent))
	}

	// 5. Generate Signed Access URL
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets/documents/objects/report.txt/sign", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Sign URL failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for sign, got %d", resp.StatusCode)
	}
	var signResult storage.SignedURLResponse
	_ = json.NewDecoder(resp.Body).Decode(&signResult)
	resp.Body.Close()

	if signResult.URL == "" && signResult.Token == "" {
		t.Fatal("expected non-empty signed url or token")
	}

	// Access object using ?token= query parameter
	signedURL := ts.URL + "/api/storage/buckets/documents/objects/report.txt?token=" + signResult.Token
	resp, err = client.Get(signedURL)
	if err != nil {
		t.Fatalf("Signed URL request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK via signed URL, got %d", resp.StatusCode)
	}
	signedContent, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(signedContent) != "NodePhone Storage Engine Test Document Content" {
		t.Errorf("unexpected signed URL content: %q", string(signedContent))
	}

	// 7. Delete Object
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/storage/buckets/documents/objects/report.txt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Delete object failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for delete object, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
