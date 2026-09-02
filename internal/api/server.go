package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nodephone/server/internal/config"
)

// Server represents the NodePhone HTTP API engine server.
type Server struct {
	cfg        *config.Config
	version    string
	out        io.Writer
	httpServer *http.Server
	host       string
	port       int
}

// NewServer initializes a new Server instance configured with port and host settings from config.
func NewServer(cfg *config.Config, version string, out io.Writer) *Server {
	if out == nil {
		out = os.Stdout
	}

	host := "0.0.0.0"
	port := 8080

	if cfg != nil {
		if cfg.Server.Host != "" {
			host = cfg.Server.Host
		}
		if cfg.Server.Port >= 0 {
			port = cfg.Server.Port
		}
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	handler := NewHandler(cfg, version)
	router := NewRouter(handler, out, DefaultTimeout)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		cfg:        cfg,
		version:    version,
		out:        out,
		httpServer: httpServer,
		host:       host,
		port:       port,
	}
}

// Addr returns the network address the server is bound to.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// Host returns the host address.
func (s *Server) Host() string {
	return s.host
}

// Port returns the listening port.
func (s *Server) Port() int {
	return s.port
}

// PrintBanner outputs the formatted startup banner to the configured log writer.
func (s *Server) PrintBanner() {
	banner := fmt.Sprintf(`
=======================================================
  NodePhone HTTP API Engine (%s)
=======================================================
  Host        : %s
  Port        : %d
  Status      : RUNNING
  Endpoints   :
    - GET  /        (Server Metadata)
    - GET  /health  (Health Probe)
    - GET  /version (System Version Info)
    - GET  /ready   (Readiness Probe)
=======================================================
`, s.version, s.host, s.port)

	fmt.Fprint(s.out, banner)
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.PrintBanner()
	fmt.Fprintf(s.out, "[INFO] HTTP API Engine listening on http://%s:%d\n", s.host, s.port)

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP API Engine server error: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	fmt.Fprintf(s.out, "[INFO] Shutting down HTTP API Engine gracefully...\n")
	err := s.httpServer.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("HTTP API Engine shutdown error: %w", err)
	}
	fmt.Fprintf(s.out, "[OK] HTTP API Engine stopped cleanly\n")
	return nil
}

// ListenAndServeWithGracefulShutdown starts the HTTP API server in a goroutine and blocks until an OS signal is received.
func (s *Server) ListenAndServeWithGracefulShutdown(stopCh <-chan os.Signal) error {
	if stopCh == nil {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		stopCh = ch
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case sig := <-stopCh:
		fmt.Fprintf(s.out, "[INFO] Received termination signal (%v). Initiating shutdown...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.Shutdown(ctx)
}
