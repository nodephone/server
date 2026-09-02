package kernel_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nodephone/server/internal/config"
	"github.com/nodephone/server/internal/kernel"
)

func TestVersion(t *testing.T) {
	expected := "v0.1.0-dev"
	if kernel.Version != expected {
		t.Errorf("expected Version %q, got %q", expected, kernel.Version)
	}
}

func TestKernelBoot(t *testing.T) {
	tempDir := t.TempDir()
	var buf bytes.Buffer

	k := kernel.New(&buf, kernel.WithDataDir(tempDir))

	err := k.Boot()
	if err != nil {
		t.Fatalf("kernel.Boot() returned unexpected error: %v", err)
	}

	output := buf.String()

	expectedSubstrings := []string{
		"[INFO] Starting NodePhone Server Kernel (v0.1.0-dev)...",
		"[INFO] Operating System:",
		"[INFO] CPU Architecture:",
		"[INFO] Initializing NodePhone data directory:",
		"[OK] Data directory structure verified",
		"[INFO] Configuration file not found. Generating default configuration...",
		"[INFO] Generated cryptographically secure JWT secret",
		"[OK] Default configuration saved to",
		"[INFO] Creating database placeholder file:",
		"[OK] Database placeholder file created:",
		"[OK] Configuration Engine initialization complete",
		"[OK] NodePhone Kernel initialized successfully.",
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

func TestPackageLevelBoot(t *testing.T) {
	tempDir := t.TempDir()
	// Change working directory temporarily to test package-level Boot creating nodephone-data
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

	err = kernel.Boot()
	if err != nil {
		t.Fatalf("package-level Boot() returned error: %v", err)
	}

	// Verify nodephone-data was created in current working dir
	if _, err := os.Stat(filepath.Join(tempDir, config.DefaultDataDir, config.ConfigFile)); err != nil {
		t.Errorf("expected default data dir config.json to exist: %v", err)
	}
}
