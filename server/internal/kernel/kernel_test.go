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
	expected := "v1.0.0"
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
	time.Sleep(200 * time.Millisecond)
	stopCh <- os.Interrupt

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("k.Boot() returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for k.Boot() to exit")
	}

	outStr := buf.String()
	if !strings.Contains(outStr, "[INFO] Starting NodePhone Server Kernel (v1.0.0)...") {
		t.Errorf("expected boot output to contain version v1.0.0, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "[OK] NodePhone Kernel initialized successfully.") {
		t.Errorf("expected boot output to contain initialization completion, got:\n%s", outStr)
	}
}

func TestGetInfo(t *testing.T) {
	k := kernel.New(nil)
	info := k.GetInfo()

	if info.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", info.Version)
	}
	if info.OS == "" || info.Arch == "" || info.NumCPU <= 0 || info.GoVer == "" {
		t.Errorf("expected diagnostic info fully populated, got %+v", info)
	}
}

func TestPackageLevelBootNonBlocking(t *testing.T) {
	tempDir := t.TempDir()
	createTestConfigWithPort(t, tempDir, 0)

	var buf bytes.Buffer
	stopCh := make(chan os.Signal, 1)

	k := kernel.New(&buf, kernel.WithDataDir(tempDir), kernel.WithStopChannel(stopCh), kernel.WithNonBlocking(true))

	if err := k.Boot(); err != nil {
		t.Fatalf("k.Boot() non-blocking mode failed: %v", err)
	}
	defer k.Close()

	if k.Config() == nil {
		t.Error("expected k.Config() to be non-nil")
	}
	if k.DB() == nil {
		t.Error("expected k.DB() to be non-nil")
	}
	if k.AuthService() == nil {
		t.Error("expected k.AuthService() to be non-nil")
	}
	if k.StorageManager() == nil {
		t.Error("expected k.StorageManager() to be non-nil")
	}
	if k.RealtimeHub() == nil {
		t.Error("expected k.RealtimeHub() to be non-nil")
	}
	if k.FunctionManager() == nil {
		t.Error("expected k.FunctionManager() to be non-nil")
	}
	if k.PolicyManager() == nil {
		t.Error("expected k.PolicyManager() to be non-nil")
	}
	if k.OpenAPIEngine() == nil {
		t.Error("expected k.OpenAPIEngine() to be non-nil")
	}
	if k.APIServer() == nil {
		t.Error("expected k.APIServer() to be non-nil")
	}
}
