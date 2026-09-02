package functions

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrFunctionNotFound   = errors.New("function not found")
	ErrRuntimeNotSupported = errors.New("runtime not supported")
)

// FunctionManager manages auto-discovery of functions from disk, runtime engine registration,
// function invocation with timeout and panic controls, and scheduled cron triggers.
type FunctionManager struct {
	baseDir   string
	runtimes  map[string]Runtime
	functions map[string]*FunctionMeta
	out       io.Writer
	mu        sync.RWMutex
}

// NewFunctionManager creates a new FunctionManager instance.
func NewFunctionManager(baseDir string, out io.Writer) (*FunctionManager, error) {
	if out == nil {
		out = io.Discard
	}
	if baseDir == "" {
		baseDir = "nodephone-data/functions"
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for functions directory: %w", err)
	}

	if err := os.MkdirAll(absBase, 0755); err != nil {
		return nil, fmt.Errorf("failed to create functions directory %q: %w", absBase, err)
	}

	fm := &FunctionManager{
		baseDir:   absBase,
		runtimes:  make(map[string]Runtime),
		functions: make(map[string]*FunctionMeta),
		out:       out,
	}

	// Register default JavaScript runtime engine
	jsRt := NewJSRuntime()
	fm.runtimes[jsRt.Name()] = jsRt

	return fm, nil
}

// BaseDir returns the root disk functions directory path.
func (fm *FunctionManager) BaseDir() string {
	return fm.baseDir
}

// RegisterRuntime registers a custom runtime engine (e.g. Go, Python, Wasm).
func (fm *FunctionManager) RegisterRuntime(rt Runtime) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.runtimes[rt.Name()] = rt
}

// Discover scans the functions directory and registers all discovered functions.
func (fm *FunctionManager) Discover() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	entries, err := os.ReadDir(fm.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read functions directory: %w", err)
	}

	discovered := make(map[string]*FunctionMeta)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".js") {
			funcName := strings.TrimSuffix(name, ".js")
			filePath := filepath.Join(fm.baseDir, name)

			timeout := 5 * time.Second
			cronSchedule := ""

			// Read file annotations (// @cron, // @timeout)
			f, openErr := os.Open(filePath)
			if openErr == nil {
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if !strings.HasPrefix(line, "//") {
						break // Stop parsing annotations once executable code begins
					}

					if strings.Contains(line, "@cron") {
						parts := strings.SplitN(line, "@cron", 2)
						if len(parts) == 2 {
							cronSchedule = strings.TrimSpace(parts[1])
						}
					}

					if strings.Contains(line, "@timeout") {
						parts := strings.SplitN(line, "@timeout", 2)
						if len(parts) == 2 {
							secStr := strings.TrimSpace(parts[1])
							if sec, pErr := strconv.Atoi(secStr); pErr == nil && sec > 0 {
								timeout = time.Duration(sec) * time.Second
							}
						}
					}
				}
				_ = f.Close()
			}

			meta := &FunctionMeta{
				Name:         funcName,
				Runtime:      "js",
				FilePath:     filePath,
				Timeout:      timeout,
				CronSchedule: cronSchedule,
				CreatedAt:    time.Now().UTC(),
			}

			discovered[funcName] = meta
			fmt.Fprintf(fm.out, "[FUNCTIONS] Discovered function %q (runtime=js, timeout=%v, cron=%q)\n", funcName, timeout, cronSchedule)
		}
	}

	fm.functions = discovered
	return nil
}

// GetFunction returns metadata for a registered function.
func (fm *FunctionManager) GetFunction(name string) (*FunctionMeta, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	meta, ok := fm.functions[name]
	if !ok {
		return nil, ErrFunctionNotFound
	}
	return meta, nil
}

// ListFunctions returns metadata for all discovered functions.
func (fm *FunctionManager) ListFunctions() []*FunctionMeta {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	funcs := make([]*FunctionMeta, 0, len(fm.functions))
	for _, f := range fm.functions {
		funcs = append(funcs, f)
	}
	return funcs
}

// Invoke executes a function by name with request payload and timeout controls.
func (fm *FunctionManager) Invoke(ctx context.Context, name string, req *FunctionRequest) (*FunctionResult, error) {
	meta, err := fm.GetFunction(name)
	if err != nil {
		return nil, err
	}

	fm.mu.RLock()
	rt, ok := fm.runtimes[meta.Runtime]
	fm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRuntimeNotSupported, meta.Runtime)
	}

	code, err := os.ReadFile(meta.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read function source file: %w", err)
	}

	timeout := meta.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	invokeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := rt.Execute(invokeCtx, string(code), req)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(fm.out, "[FUNCTIONS] Executed function %q in %v (status=%d)\n", name, result.Duration, result.StatusCode)
	return result, nil
}

// StartCronScheduler starts a background ticker executing functions configured with cron schedules.
func (fm *FunctionManager) StartCronScheduler(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	fmt.Fprintln(fm.out, "[FUNCTIONS] Cron scheduler loop started")

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(fm.out, "[FUNCTIONS] Cron scheduler stopped")
			return
		case <-ticker.C:
			fm.mu.RLock()
			var scheduled []*FunctionMeta
			for _, f := range fm.functions {
				if f.CronSchedule != "" {
					scheduled = append(scheduled, f)
				}
			}
			fm.mu.RUnlock()

			for _, meta := range scheduled {
				go func(m *FunctionMeta) {
					req := &FunctionRequest{
						Method: "CRON",
						Path:   "/api/functions/" + m.Name,
					}
					_, _ = fm.Invoke(ctx, m.Name, req)
				}(meta)
			}
		}
	}
}
