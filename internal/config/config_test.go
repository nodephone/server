package config_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nodephone/server/internal/config"
)

func TestGenerateJWTSecret(t *testing.T) {
	secret1, err := config.GenerateJWTSecret()
	if err != nil {
		t.Fatalf("GenerateJWTSecret returned unexpected error: %v", err)
	}

	if len(secret1) != 64 {
		t.Errorf("expected secret length of 64 hex characters, got %d (%s)", len(secret1), secret1)
	}

	_, err = hex.DecodeString(secret1)
	if err != nil {
		t.Errorf("expected valid hex string, got error: %v", err)
	}

	secret2, err := config.GenerateJWTSecret()
	if err != nil {
		t.Fatalf("GenerateJWTSecret second call returned unexpected error: %v", err)
	}

	if secret1 == secret2 {
		t.Errorf("expected cryptographically unique secrets, got identical secrets")
	}
}

func TestInitOrLoad_FreshBoot(t *testing.T) {
	tempDir := t.TempDir()
	var logBuf bytes.Buffer

	mgr := config.NewManager(tempDir, &logBuf)
	cfg, err := mgr.InitOrLoad()
	if err != nil {
		t.Fatalf("InitOrLoad returned unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil Config returned")
	}

	// Verify server defaults
	if cfg.Server.Name != "NodePhone Server" {
		t.Errorf("expected server name 'NodePhone Server', got %q", cfg.Server.Name)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}
	if len(cfg.Auth.JWTSecret) != 64 {
		t.Errorf("expected 64-character hex JWT secret, got %q", cfg.Auth.JWTSecret)
	}

	// Verify subdirectory creation
	for _, sub := range config.DefaultSubdirectories {
		subPath := filepath.Join(tempDir, sub)
		info, err := os.Stat(subPath)
		if err != nil {
			t.Errorf("expected directory %q to exist, error: %v", subPath, err)
		} else if !info.IsDir() {
			t.Errorf("expected %q to be a directory", subPath)
		}
	}

	// Verify config.json creation
	configPath := filepath.Join(tempDir, config.ConfigFile)
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected config.json to exist: %v", err)
	}

	// Verify main.db placeholder creation
	dbPath := filepath.Join(tempDir, config.DBFile)
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Errorf("expected main.db placeholder to exist: %v", err)
	} else if dbInfo.Size() != 0 {
		t.Errorf("expected main.db size 0, got %d", dbInfo.Size())
	}

	// Verify log outputs
	logs := logBuf.String()
	expectedLogSubstrings := []string{
		"[INFO] Initializing NodePhone data directory:",
		"[OK] Data directory structure verified",
		"[INFO] Configuration file not found. Generating default configuration...",
		"[INFO] Generated cryptographically secure JWT secret",
		"[OK] Default configuration saved to",
		"[INFO] Creating database placeholder file:",
		"[OK] Database placeholder file created:",
		"[OK] Configuration Engine initialization complete",
	}

	for _, sub := range expectedLogSubstrings {
		if !strings.Contains(logs, sub) {
			t.Errorf("expected log output to contain %q, got:\n%s", sub, logs)
		}
	}
}

func TestInitOrLoad_SubsequentBoot(t *testing.T) {
	tempDir := t.TempDir()

	// Initial boot
	mgr1 := config.NewManager(tempDir, nil)
	cfg1, err := mgr1.InitOrLoad()
	if err != nil {
		t.Fatalf("first InitOrLoad failed: %v", err)
	}

	initialSecret := cfg1.Auth.JWTSecret

	// Modify config.json to test persistence
	cfg1.Server.Port = 9999
	modifiedData, err := json.MarshalIndent(cfg1, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal modified config: %v", err)
	}

	configPath := filepath.Join(tempDir, config.ConfigFile)
	if err := os.WriteFile(configPath, modifiedData, 0600); err != nil {
		t.Fatalf("failed to write modified config file: %v", err)
	}

	// Second boot
	var logBuf bytes.Buffer
	mgr2 := config.NewManager(tempDir, &logBuf)
	cfg2, err := mgr2.InitOrLoad()
	if err != nil {
		t.Fatalf("second InitOrLoad failed: %v", err)
	}

	if cfg2.Server.Port != 9999 {
		t.Errorf("expected loaded config port 9999, got %d", cfg2.Server.Port)
	}

	if cfg2.Auth.JWTSecret != initialSecret {
		t.Errorf("expected JWT secret to be preserved as %q, got %q", initialSecret, cfg2.Auth.JWTSecret)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "[INFO] Loading existing configuration from") {
		t.Errorf("expected log to indicate loading existing configuration, got:\n%s", logs)
	}
	if !strings.Contains(logs, "[OK] Existing database file verified:") {
		t.Errorf("expected log to indicate existing database file verified, got:\n%s", logs)
	}
}

func TestInitOrLoad_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	configPath := filepath.Join(tempDir, config.ConfigFile)
	if err := os.WriteFile(configPath, []byte("invalid json {{"), 0600); err != nil {
		t.Fatalf("failed to write broken config file: %v", err)
	}

	mgr := config.NewManager(tempDir, nil)
	_, err := mgr.InitOrLoad()
	if err == nil {
		t.Fatal("expected error when parsing invalid JSON config file, got nil")
	}
}

func TestNewManager_Defaults(t *testing.T) {
	mgr := config.NewManager("", nil)
	if mgr.DataDir() != config.DefaultDataDir {
		t.Errorf("expected default data dir %q, got %q", config.DefaultDataDir, mgr.DataDir())
	}
}
