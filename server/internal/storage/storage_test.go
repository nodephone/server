package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/database"
	"github.com/nodephone/server/internal/storage"
)

func TestSanitizationAndPathSafety(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "safety_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	sm, err := storage.NewStorageManager(db, filepath.Join(tempDir, "storage"), "testsecret", nil)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}

	sanitized := storage.SanitizeBucketName("My-Bucket 123!@#$")
	if sanitized != "my-bucket123" {
		t.Errorf("expected sanitized bucket name 'my-bucket123', got %q", sanitized)
	}

	// Path traversal test cases
	traversalTests := []string{
		"../../etc/passwd",
		"..\\..\\Windows\\System32\\cmd.exe",
		"/etc/shadow",
		"sub/../../../secret.txt",
	}

	for _, badPath := range traversalTests {
		_, err := sm.ResolveSafePath("my-bucket123", badPath)
		if err == nil || !errors.Is(err, storage.ErrInvalidPath) {
			t.Errorf("expected ErrInvalidPath for %q, got %v", badPath, err)
		}
	}
}

func TestBucketAndObjectOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "ops_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Insert test user to satisfy foreign key constraint
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, username, email, password_hash) VALUES ('user1', 'u1', 'u1@example.com', 'hash');")
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	sm, err := storage.NewStorageManager(db, filepath.Join(tempDir, "storage"), "testsecret", nil)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}

	// 1. Create Bucket
	b, err := sm.CreateBucket(ctx, "assets", true, "user1")
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}
	if b.Name != "assets" || !b.Public {
		t.Errorf("unexpected bucket details: %+v", b)
	}

	// 2. PutObject
	payload := "Hello, NodePhone Storage Engine!"
	body := strings.NewReader(payload)
	obj, err := sm.PutObject(ctx, "assets", "docs/hello.txt", body, int64(len(payload)), "text/plain", "user1")
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}
	if obj.Name != "docs/hello.txt" || obj.Size != int64(len(payload)) {
		t.Errorf("unexpected object details: %+v", obj)
	}

	// 3. GetObjectStream
	streamObj, reader, err := sm.GetObjectStream(ctx, "assets", "docs/hello.txt")
	if err != nil {
		t.Fatalf("GetObjectStream failed: %v", err)
	}
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("failed to read stream content: %v", err)
	}
	if string(content) != payload {
		t.Errorf("expected streamed payload %q, got %q", payload, string(content))
	}
	if streamObj.Size != obj.Size {
		t.Errorf("expected size %d, got %d", obj.Size, streamObj.Size)
	}

	// 4. ListObjects
	objects, err := sm.ListObjects(ctx, "assets")
	if err != nil {
		t.Fatalf("ListObjects failed: %v", err)
	}
	if len(objects) != 1 {
		t.Errorf("expected 1 object listed, got %d", len(objects))
	}

	// 5. DeleteObject
	if err := sm.DeleteObject(ctx, "assets", "docs/hello.txt"); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	_, _, err = sm.GetObjectStream(ctx, "assets", "docs/hello.txt")
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected ErrObjectNotFound after deletion, got %v", err)
	}

	// 6. DeleteBucket
	if err := sm.DeleteBucket(ctx, "assets"); err != nil {
		t.Fatalf("DeleteBucket failed: %v", err)
	}
}

func TestSignedURLs(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "signed_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	sm, err := storage.NewStorageManager(db, filepath.Join(tempDir, "storage"), "jwtsecret123456", nil)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}

	resp, err := sm.GenerateSignedURL("private-vault", "contract.pdf", 1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateSignedURL failed: %v", err)
	}

	if resp.Token == "" || !strings.Contains(resp.URL, "token=") {
		t.Errorf("unexpected signed URL response: %+v", resp)
	}

	// Validate correct token
	if !sm.ValidateSignedURL(resp.Token, "private-vault", "contract.pdf") {
		t.Error("expected valid token verification to pass")
	}

	// Validate tampered object name
	if sm.ValidateSignedURL(resp.Token, "private-vault", "other.pdf") {
		t.Error("expected token verification to fail for tampered object name")
	}
}

func TestStorageAPIEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "storage_api_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	authService := auth.NewAuthService(db, "testjwtsecret1234567890123456", nil)
	user, err := authService.SignUp(ctx, auth.SignUpRequest{Username: "uploader", Email: "uploader@example.com", Password: "Password123!"})
	if err != nil {
		t.Fatalf("SignUp failed: %v", err)
	}
	loginResp, err := authService.LogIn(ctx, auth.LoginRequest{Login: "uploader", Password: "Password123!"})
	if err != nil {
		t.Fatalf("LogIn failed: %v", err)
	}

	sm, err := storage.NewStorageManager(db, filepath.Join(tempDir, "storage"), "testjwtsecret1234567890123456", nil)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}

	storageHandler := storage.NewStorageHandler(sm, authService)
	authMW := auth.AuthMiddleware(authService)
	optAuthMW := auth.OptionalAuthMiddleware(authService)

	mux := http.NewServeMux()
	mux.Handle("/api/storage/buckets", authMW(http.HandlerFunc(storageHandler.CreateBucket)))
	mux.HandleFunc("/api/storage/buckets/", func(w http.ResponseWriter, r *http.Request) {
		optAuthMW(http.HandlerFunc(storageHandler.RouteBucketObject)).ServeHTTP(w, r)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// 1. Create Public Bucket
	createBody, _ := json.Marshal(storage.CreateBucketRequest{Name: "public-files", Public: true})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets", bytes.NewBuffer(createBody))
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("CreateBucket HTTP POST failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Create Private Bucket
	createPrivBody, _ := json.Marshal(storage.CreateBucketRequest{Name: "private-files", Public: false})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets", bytes.NewBuffer(createPrivBody))
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("CreateBucket Private failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Upload File to Private Bucket via Multipart Form
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "secret_data.txt")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	filePayload := "TOP SECRET DATA"
	_, _ = part.Write([]byte(filePayload))
	_ = mw.Close()

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/storage/buckets/private-files/objects", &buf)
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("UploadObject failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. GET Private Object WITHOUT Auth -> 401 Unauthorized
	resp, err = client.Get(ts.URL + "/api/storage/buckets/private-files/objects/secret_data.txt")
	if err != nil {
		t.Fatalf("Get private object failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. GET Private Object WITH Bearer Auth -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/storage/buckets/private-files/objects/secret_data.txt", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Get private object with auth failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	downloaded, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(downloaded) != filePayload {
		t.Errorf("expected payload %q, got %q", filePayload, string(downloaded))
	}

	// 6. Generate Signed URL for Private Object
	signedURLResp, err := sm.GenerateSignedURL("private-files", "secret_data.txt", 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateSignedURL failed: %v", err)
	}

	// 7. Download Private Object via Signed URL Token WITHOUT Auth Header -> 200 OK
	signedTarget := ts.URL + "/api/storage/buckets/private-files/objects/secret_data.txt?token=" + signedURLResp.Token
	resp, err = client.Get(signedTarget)
	if err != nil {
		t.Fatalf("Get via Signed URL failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK via Signed URL, got %d", resp.StatusCode)
	}
	signedDownloaded, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(signedDownloaded) != filePayload {
		t.Errorf("expected payload %q, got %q", filePayload, string(signedDownloaded))
	}

	_ = user
}
