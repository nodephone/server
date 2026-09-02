// Package kernel provides the core initialization and runtime boot logic for the NodePhone Server.
package kernel

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/nodephone/server/internal/config"
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

// Kernel represents the core NodePhone server kernel subsystem.
type Kernel struct {
	os      string
	arch    string
	out     io.Writer
	dataDir string
	config  *config.Config
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

// Boot initializes the NodePhone server kernel sequence, executes the configuration engine,
// and outputs system initialization diagnostics.
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

	// Initialize Configuration Engine
	cfgMgr := config.NewManager(k.dataDir, k.out)
	cfg, err := cfgMgr.InitOrLoad()
	if err != nil {
		return fmt.Errorf("failed to initialize configuration engine: %w", err)
	}
	k.config = cfg

	if _, err := fmt.Fprintf(k.out, "[OK] NodePhone Kernel initialized successfully.\n"); err != nil {
		return fmt.Errorf("failed to write boot completion: %w", err)
	}

	return nil
}

// Boot executes the default kernel boot sequence using standard output.
// This package-level function serves as the standard entry point for booting the kernel.
func Boot() error {
	k := New(os.Stdout)
	return k.Boot()
}
