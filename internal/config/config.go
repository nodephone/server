// Package config provides configuration management and data directory initialization for the NodePhone server.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Default paths and file names for the NodePhone data directory.
const (
	DefaultDataDir = "nodephone-data"
	ConfigFile     = "config.json"
	DBFile         = "main.db"
	JWTSecretFile  = "secrets/jwt.key"
)

// DefaultSubdirectories defines the required folder hierarchy inside nodephone-data.
var DefaultSubdirectories = []string{
	"logs",
	"certs",
	"media",
	"storage",
	"secrets",
}

// Config represents the complete system configuration structure for NodePhone.
type Config struct {
	Server   ServerConfig   `json:"server"`
	Auth     AuthConfig     `json:"auth"`
	Database DatabaseConfig `json:"database"`
	Logging  LoggingConfig  `json:"logging"`
}

// ServerConfig defines server binding and environment settings.
type ServerConfig struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Environment string `json:"environment"`
}

// AuthConfig defines security parameters including the JWT secret key.
type AuthConfig struct {
	JWTSecret string `json:"jwt_secret"`
}

// DatabaseConfig defines database connection parameters.
type DatabaseConfig struct {
	Driver string `json:"driver"`
	Path   string `json:"path"`
}

// LoggingConfig defines log verbosity and directory configuration.
type LoggingConfig struct {
	Level string `json:"level"`
	Dir   string `json:"dir"`
}

// Manager manages initialization, loading, and persistence of system configuration.
type Manager struct {
	dataDir string
	out     io.Writer
}

// NewManager creates a new Manager instance for the target data directory.
// If dataDir is empty, DefaultDataDir is used.
// If out is nil, io.Discard is used to suppress log output.
func NewManager(dataDir string, out io.Writer) *Manager {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	if out == nil {
		out = io.Discard
	}
	return &Manager{
		dataDir: dataDir,
		out:     out,
	}
}

// GenerateJWTSecret produces a 32-byte cryptographically secure random secret encoded as a hex string.
func GenerateJWTSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate cryptographically secure JWT secret: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// LoadOrGenerateJWTSecret loads the JWT secret from nodephone-data/secrets/jwt.key,
// or generates and persists a new 32-byte secret if missing.
func LoadOrGenerateJWTSecret(dataDir string, out io.Writer) (string, error) {
	if out == nil {
		out = io.Discard
	}
	secretPath := filepath.Join(dataDir, JWTSecretFile)
	secretsDir := filepath.Dir(secretPath)

	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create secrets directory %q: %w", secretsDir, err)
	}

	data, err := os.ReadFile(secretPath)
	if err == nil {
		secret := strings.TrimSpace(string(data))
		if secret != "" {
			fmt.Fprintf(out, "[INFO] Loaded JWT secret key from %s\n", secretPath)
			return secret, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("failed to read JWT secret file %q: %w", secretPath, err)
	}

	fmt.Fprintf(out, "[INFO] Secret file %s not found. Generating new JWT secret key...\n", secretPath)
	secret, err := GenerateJWTSecret()
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(secretPath, []byte(secret+"\n"), 0600); err != nil {
		return "", fmt.Errorf("failed to persist JWT secret key to %q: %w", secretPath, err)
	}

	fmt.Fprintf(out, "[OK] Generated and saved cryptographically secure JWT secret to %s\n", secretPath)
	return secret, nil
}

// DefaultConfig generates a new Config populated with standard defaults.
func DefaultConfig(dataDir string, jwtSecret string) *Config {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	return &Config{
		Server: ServerConfig{
			Name:        "NodePhone Server",
			Host:        "0.0.0.0",
			Port:        8080,
			Environment: "development",
		},
		Auth: AuthConfig{
			JWTSecret: jwtSecret,
		},
		Database: DatabaseConfig{
			Driver: "sqlite3",
			Path:   filepath.Join(dataDir, DBFile),
		},
		Logging: LoggingConfig{
			Level: "info",
			Dir:   filepath.Join(dataDir, "logs"),
		},
	}
}

// DataDir returns the root data directory managed by this instance.
func (m *Manager) DataDir() string {
	return m.dataDir
}

// InitOrLoad initializes the data directory layout, auto-creates default config and database placeholder
// if missing, or loads pre-existing configuration on subsequent boots.
func (m *Manager) InitOrLoad() (*Config, error) {
	fmt.Fprintf(m.out, "[INFO] Initializing NodePhone data directory: %s\n", m.dataDir)

	// 1. Create root data directory
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %q: %w", m.dataDir, err)
	}

	// 2. Create subdirectories
	for _, sub := range DefaultSubdirectories {
		subPath := filepath.Join(m.dataDir, sub)
		if err := os.MkdirAll(subPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create subdirectory %q: %w", subPath, err)
		}
	}
	fmt.Fprintf(m.out, "[OK] Data directory structure verified\n")

	// 3. Process nodephone-data/secrets/jwt.key
	jwtSecret, err := LoadOrGenerateJWTSecret(m.dataDir, m.out)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize JWT secret key: %w", err)
	}

	// 4. Process config.json
	configPath := filepath.Join(m.dataDir, ConfigFile)
	var cfg *Config

	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(m.out, "[INFO] Configuration file not found. Generating default configuration...\n")

		cfg = DefaultConfig(m.dataDir, jwtSecret)
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal default configuration: %w", err)
		}

		if err := os.WriteFile(configPath, data, 0600); err != nil {
			return nil, fmt.Errorf("failed to write configuration file %q: %w", configPath, err)
		}
		fmt.Fprintf(m.out, "[OK] Default configuration saved to %s\n", configPath)
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat configuration file %q: %w", configPath, err)
	} else {
		fmt.Fprintf(m.out, "[INFO] Loading existing configuration from %s...\n", configPath)

		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read configuration file %q: %w", configPath, err)
		}

		cfg = &Config{}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse configuration file %q: %w", configPath, err)
		}

		// Ensure loaded config has latest JWT secret from jwt.key
		cfg.Auth.JWTSecret = jwtSecret
		fmt.Fprintf(m.out, "[OK] Existing configuration loaded successfully\n")
	}

	// 5. Process main.db placeholder file
	dbPath := filepath.Join(m.dataDir, DBFile)
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(m.out, "[INFO] Creating database placeholder file: %s...\n", dbPath)

		if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
			return nil, fmt.Errorf("failed to create database placeholder file %q: %w", dbPath, err)
		}
		fmt.Fprintf(m.out, "[OK] Database placeholder file created: %s\n", dbPath)
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat database file %q: %w", dbPath, err)
	} else {
		fmt.Fprintf(m.out, "[OK] Existing database file verified: %s\n", dbPath)
	}

	fmt.Fprintf(m.out, "[OK] Configuration Engine initialization complete\n")
	return cfg, nil
}
