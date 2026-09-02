// Package kernel provides the core initialization and runtime boot logic for the NodePhone Server.
package kernel

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nodephone/server/internal/api"
	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/config"
	"github.com/nodephone/server/internal/database"
	"github.com/nodephone/server/internal/functions"
	"github.com/nodephone/server/internal/permissions"
	"github.com/nodephone/server/internal/realtime"
	"github.com/nodephone/server/internal/storage"
)

// Version specifies the current release version of the NodePhone Server kernel.
const Version = "v0.1.0-dev"

// Option configures a Kernel instance.
type Option func(*Kernel)

// WithDataDir configures a custom data directory for the kernel instance.
func WithDataDir(dataDir string) Option {
	return func(k *Kernel) {
		k.dataDir = dataDir
	}
}

// WithStopChannel configures a custom signal channel to control server shutdown during testing.
func WithStopChannel(stopCh chan os.Signal) Option {
	return func(k *Kernel) {
		k.stopCh = stopCh
	}
}

// WithNonBlocking runs the HTTP API server in background without blocking Boot(). Useful for tests.
func WithNonBlocking(nonBlock bool) Option {
	return func(k *Kernel) {
		k.nonBlock = nonBlock
	}
}

// Kernel represents the core NodePhone server kernel subsystem.
type Kernel struct {
	os              string
	arch            string
	out             io.Writer
	dataDir         string
	config          *config.Config
	db              *database.DB
	authService     *auth.AuthService
	storageManager  *storage.StorageManager
	realtimeHub     *realtime.Hub
	functionManager *functions.FunctionManager
	policyManager   *permissions.PolicyManager
	apiServer       *api.Server
	stopCh          chan os.Signal
	nonBlock        bool
}

// Info holds diagnostic information about the host system runtime environment.
type Info struct {
	Version string
	OS      string
	Arch    string
	NumCPU  int
	GoVer   string
}

// New creates a new Kernel instance configured with the specified output writer and options.
// If out is nil, os.Stdout is used by default.
func New(out io.Writer, opts ...Option) *Kernel {
	if out == nil {
		out = os.Stdout
	}
	k := &Kernel{
		os:      runtime.GOOS,
		arch:    runtime.GOARCH,
		out:     out,
		dataDir: config.DefaultDataDir,
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// GetInfo returns runtime metadata about the current kernel environment.
func (k *Kernel) GetInfo() Info {
	return Info{
		Version: Version,
		OS:      k.os,
		Arch:    k.arch,
		NumCPU:  runtime.NumCPU(),
		GoVer:   runtime.Version(),
	}
}

// Config returns the loaded configuration after kernel boot.
func (k *Kernel) Config() *config.Config {
	return k.config
}

// DB returns the initialized database engine instance.
func (k *Kernel) DB() *database.DB {
	return k.db
}

// AuthService returns the initialized authentication service instance.
func (k *Kernel) AuthService() *auth.AuthService {
	return k.authService
}

// StorageManager returns the initialized storage manager instance.
func (k *Kernel) StorageManager() *storage.StorageManager {
	return k.storageManager
}

// RealtimeHub returns the initialized realtime hub instance.
func (k *Kernel) RealtimeHub() *realtime.Hub {
	return k.realtimeHub
}

// FunctionManager returns the initialized functions manager instance.
func (k *Kernel) FunctionManager() *functions.FunctionManager {
	return k.functionManager
}

// PolicyManager returns the initialized policy manager instance.
func (k *Kernel) PolicyManager() *permissions.PolicyManager {
	return k.policyManager
}

// APIServer returns the initialized API engine server instance.
func (k *Kernel) APIServer() *api.Server {
	return k.apiServer
}

// Boot initializes the NodePhone server kernel sequence, executes the configuration engine,
// boots the SQLite database engine, executes schema migrations, boots the Authentication engine,
// boots the Storage engine, boots the Realtime engine, boots the Functions engine, boots the Permissions engine,
// boots the HTTP API Engine, and handles server lifecycle operations.
func (k *Kernel) Boot() error {
	if k.out == nil {
		return fmt.Errorf("kernel output writer is uninitialized")
	}

	info := k.GetInfo()

	if _, err := fmt.Fprintf(k.out, "[INFO] Starting NodePhone Server Kernel (%s)...\n", info.Version); err != nil {
		return fmt.Errorf("failed to write boot header: %w", err)
	}

	if _, err := fmt.Fprintf(k.out, "[INFO] Operating System: %s\n", info.OS); err != nil {
		return fmt.Errorf("failed to write OS info: %w", err)
	}

	if _, err := fmt.Fprintf(k.out, "[INFO] CPU Architecture: %s\n", info.Arch); err != nil {
		return fmt.Errorf("failed to write CPU arch info: %w", err)
	}

	// 1. Initialize Configuration Engine
	cfgMgr := config.NewManager(k.dataDir, k.out)
	cfg, err := cfgMgr.InitOrLoad()
	if err != nil {
		return fmt.Errorf("failed to initialize configuration engine: %w", err)
	}
	k.config = cfg

	// 2. Initialize Database Engine
	db, err := database.Open(k.config.Database.Path, k.out)
	if err != nil {
		return fmt.Errorf("failed to initialize database engine: %w", err)
	}
	k.db = db
	defer func() {
		_ = k.db.Close()
	}()

	// 3. Execute Database Schema Auto-Migrations
	migCtx, migCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer migCancel()

	if err := k.db.AutoMigrate(migCtx); err != nil {
		return fmt.Errorf("failed to execute database auto-migrations: %w", err)
	}

	// 4. Initialize Authentication Engine
	k.authService = auth.NewAuthService(k.db, k.config.Auth.JWTSecret, k.out)
	authHandler := auth.NewAuthHandler(k.authService)

	// 5. Initialize Storage Engine
	storageDir := filepath.Join(k.dataDir, "storage")
	sm, err := storage.NewStorageManager(k.db, storageDir, k.config.Auth.JWTSecret, k.out)
	if err != nil {
		return fmt.Errorf("failed to initialize storage engine: %w", err)
	}
	k.storageManager = sm
	storageHandler := storage.NewStorageHandler(k.storageManager, k.authService)

	// 6. Initialize Realtime Engine
	k.realtimeHub = realtime.NewHub(k.out)
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	go k.realtimeHub.Run(hubCtx)
	realtimeHandler := realtime.NewRealtimeHandler(k.realtimeHub, k.authService)

	// 7. Initialize Functions Engine
	funcDir := filepath.Join(k.dataDir, "functions")
	fm, err := functions.NewFunctionManager(funcDir, k.out)
	if err != nil {
		return fmt.Errorf("failed to initialize functions engine: %w", err)
	}
	if err := fm.Discover(); err != nil {
		return fmt.Errorf("failed to discover functions: %w", err)
	}
	k.functionManager = fm
	functionHandler := functions.NewFunctionHandler(k.functionManager)

	// 8. Initialize Permissions Engine
	k.policyManager = permissions.NewPolicyManager(k.db, k.out)
	policyHandler := permissions.NewPolicyHandler(k.policyManager)

	if _, err := fmt.Fprintf(k.out, "[OK] NodePhone Kernel initialized successfully.\n"); err != nil {
		return fmt.Errorf("failed to write boot completion: %w", err)
	}

	// 9. Initialize HTTP API Engine
	k.apiServer = api.NewServer(k.config, Version, k.out, authHandler, storageHandler, realtimeHandler, functionHandler, policyHandler)

	if k.nonBlock {
		go func() {
			_ = k.apiServer.Start()
		}()
		return nil
	}

	// 10. Start API Server with Graceful Shutdown listener
	return k.apiServer.ListenAndServeWithGracefulShutdown(k.stopCh)
}

// Boot executes the default kernel boot sequence using standard output.
// This package-level function serves as the standard entry point for booting the kernel.
func Boot() error {
	k := New(os.Stdout)
	return k.Boot()
}
