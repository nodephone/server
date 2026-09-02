package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/api"
	"github.com/nodephone/server/internal/config"
)

func TestEndpoints(t *testing.T) {
	cfg := config.DefaultConfig("testdata", "testsecret")
	version := "v0.1.0-test"

	h := api.NewHandler(cfg, version)
	var logBuf bytes.Buffer
	router := api.NewRouter(h, nil, nil, &logBuf, 5*time.Second)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:           "GET Root /",
			method:         http.MethodGet,
			path:           "/",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var res api.RootResponse
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("failed to unmarshal RootResponse: %v", err)
				}
				if res.App != "NodePhone Server" {
					t.Errorf("expected App 'NodePhone Server', got %q", res.App)
				}
				if res.Version != version {
					t.Errorf("expected Version %q, got %q", version, res.Version)
				}
				if res.Status != "running" {
					t.Errorf("expected Status 'running', got %q", res.Status)
				}
			},
		},
		{
			name:           "GET /health",
			method:         http.MethodGet,
			path:           "/health",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var res api.HealthResponse
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("failed to unmarshal HealthResponse: %v", err)
				}
				if res.Status != "ok" {
					t.Errorf("expected Status 'ok', got %q", res.Status)
				}
				if res.Timestamp.IsZero() {
					t.Error("expected non-zero timestamp")
				}
			},
		},
		{
			name:           "GET /version",
			method:         http.MethodGet,
			path:           "/version",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var res api.VersionResponse
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("failed to unmarshal VersionResponse: %v", err)
				}
				if res.Version != version {
					t.Errorf("expected Version %q, got %q", version, res.Version)
				}
				if res.GoVersion == "" || res.OS == "" || res.Arch == "" {
					t.Errorf("expected runtime metadata populated, got %+v", res)
				}
			},
		},
		{
			name:           "GET /ready",
			method:         http.MethodGet,
			path:           "/ready",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var res api.ReadyResponse
				if err := json.Unmarshal(body, &res); err != nil {
					t.Fatalf("failed to unmarshal ReadyResponse: %v", err)
				}
				if res.Status != "ready" {
					t.Errorf("expected Status 'ready', got %q", res.Status)
				}
				if res.Checks["config"] != "ok" || res.Checks["storage"] != "ok" {
					t.Errorf("expected valid readiness checks, got %+v", res.Checks)
				}
			},
		},
		{
			name:           "POST /health Method Not Allowed",
			method:         http.MethodPost,
			path:           "/health",
			expectedStatus: http.StatusMethodNotAllowed,
			checkBody:      nil,
		},
		{
			name:           "GET /unknown Not Found",
			method:         http.MethodGet,
			path:           "/unknown",
			expectedStatus: http.StatusNotFound,
			checkBody:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, res.StatusCode)
			}

			contentType := res.Header.Get("Content-Type")
			if !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", contentType)
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			if tc.checkBody != nil {
				tc.checkBody(t, body)
			}
		})
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	var logBuf bytes.Buffer
	panickingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went horribly wrong!")
	})

	recoveredHandler := api.RecoveryMiddleware(&logBuf)(panickingHandler)

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	recoveredHandler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500 InternalServerError, got %d", res.StatusCode)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "[ERROR] Panic recovered in HTTP handler: something went horribly wrong!") {
		t.Errorf("expected panic log output, got:\n%s", logs)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var logBuf bytes.Buffer
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	loggedHandler := api.LoggingMiddleware(&logBuf)(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test-log", nil)
	rec := httptest.NewRecorder()

	loggedHandler.ServeHTTP(rec, req)

	logs := logBuf.String()
	if !strings.Contains(logs, "[HTTP] GET /test-log 200") {
		t.Errorf("expected log entry for request, got:\n%s", logs)
	}
}

func TestServerStartupAndShutdown(t *testing.T) {
	// Find a free TCP port for testing
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cfg := config.DefaultConfig("testdata", "testsecret")
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = port

	var logBuf bytes.Buffer
	srv := api.NewServer(cfg, "v0.1.0-test", &logBuf, nil, nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for server to accept connections
	targetURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 1 * time.Second}

	var resp *http.Response
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, err = client.Get(targetURL)
		if err == nil {
			break
		}
	}

	if err != nil {
		t.Fatalf("failed to connect to HTTP API engine at %s: %v", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected GET /health to return 200, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("server.Shutdown() failed: %v", err)
	}

	startErr := <-errCh
	if startErr != nil {
		t.Errorf("srv.Start() returned error after shutdown: %v", startErr)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "NodePhone HTTP API Engine") {
		t.Errorf("expected banner in logs, got:\n%s", logs)
	}
	if !strings.Contains(logs, "[OK] HTTP API Engine stopped cleanly") {
		t.Errorf("expected clean shutdown log, got:\n%s", logs)
	}
}

func TestListenAndServeWithGracefulShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cfg := config.DefaultConfig("testdata", "testsecret")
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = port

	var logBuf bytes.Buffer
	srv := api.NewServer(cfg, "v0.1.0-test", &logBuf, nil, nil)

	stopCh := make(chan os.Signal, 1)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServeWithGracefulShutdown(stopCh)
	}()

	// Wait briefly then send fake signal
	time.Sleep(100 * time.Millisecond)
	stopCh <- os.Interrupt

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ListenAndServeWithGracefulShutdown returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for ListenAndServeWithGracefulShutdown to exit")
	}
}
