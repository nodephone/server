package kernel_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/config"
	"github.com/nodephone/server/internal/kernel"
)

func createTestConfigWithPort(t *testing.T, dataDir string, port int) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}
	cfg := config.DefaultConfig(dataDir, "testsecret")
	cfg.Server.Port = port
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	configPath := filepath.Join(dataDir, config.ConfigFile)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}

func TestVersion(t *testing.T) {
	expected := "v0.1.0-dev"
	if kernel.Version != expected {
		t.Errorf("expected Version %q, got %q", expected, kernel.Version)
	}
}

func TestKernelBoot(t *testing.T) {
	tempDir := t.TempDir()
	createTestConfigWithPort(t, tempDir, 0)

	var buf bytes.Buffer
	stopCh := make(chan os.Signal, 1)

	k := kernel.New(&buf, kernel.WithDataDir(tempDir), kernel.WithStopChannel(stopCh))

	errCh := make(chan error, 1)
	go func() {
		errCh <- k.Boot()
	}()

	// Send shutdown signal after boot starts
	time.Sleep(150 * time.Millisecond)
	stopCh <- os.Interrupt

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("k.Boot() returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for k.Boot() to complete shutdown")
	}

	output := buf.String()

	expectedSubstrings := []string{
		"[INFO] Starting NodePhone Server Kernel (v0.1.0-dev)...",
		"[INFO] Operating System:",
		"[INFO] CPU Architecture:",
		"[INFO] Initializing NodePhone data directory:",
		"[OK] Data directory structure verified",
		"[INFO] Loading existing configuration from",
		"[OK] Existing configuration loaded successfully",
		"[OK] Configuration Engine initialization complete",
		"[DB] Connecting to SQLite database at",
		"[OK] SQLite database connection established",
		"[DB] Running database migrations...",
		"[OK] All database migrations verified",
		"[OK] NodePhone Kernel initialized successfully.",
		"NodePhone HTTP API Engine",
		"[INFO] HTTP API Engine listening on",
		"[INFO] Received termination signal",
		"[OK] HTTP API Engine stopped cleanly",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(output, sub) {
			t.Errorf("expected boot output to contain %q, got:\n%s", sub, output)
		}
	}

	// Verify loaded config is accessible via k.Config()
	cfg := k.Config()
	if cfg == nil {
		t.Fatal("expected k.Config() to return non-nil config after boot")
	}

	if cfg.Server.Name != "NodePhone Server" {
		t.Errorf("expected server name 'NodePhone Server', got %q", cfg.Server.Name)
	}

	// Verify DB instance
	if k.DB() == nil {
		t.Fatal("expected k.DB() to return non-nil database instance")
	}

	// Verify API server instance
	if k.APIServer() == nil {
		t.Fatal("expected k.APIServer() to return non-nil server")
	}
}

func TestGetInfo(t *testing.T) {
	var buf bytes.Buffer
	k := kernel.New(&buf)
	info := k.GetInfo()

	if info.Version != kernel.Version {
		t.Errorf("expected info.Version %q, got %q", kernel.Version, info.Version)
	}

	if info.OS == "" {
		t.Error("expected non-empty info.OS")
	}

	if info.Arch == "" {
		t.Error("expected non-empty info.Arch")
	}

	if info.NumCPU <= 0 {
		t.Errorf("expected positive NumCPU, got %d", info.NumCPU)
	}
}

func TestPackageLevelBootNonBlocking(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, config.DefaultDataDir)
	createTestConfigWithPort(t, dataDir, 0)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir to tempDir: %v", err)
	}

	var buf bytes.Buffer
	stopCh := make(chan os.Signal, 1)
	k := kernel.New(&buf, kernel.WithStopChannel(stopCh))

	errCh := make(chan error, 1)
	go func() {
		errCh <- k.Boot()
	}()

	time.Sleep(150 * time.Millisecond)
	stopCh <- os.Interrupt

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("k.Boot() returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for k.Boot() to complete shutdown")
	}

	// Verify nodephone-data exists
	if _, err := os.Stat(filepath.Join(dataDir, config.ConfigFile)); err != nil {
		t.Errorf("expected default data dir config.json to exist: %v", err)
	}
}
