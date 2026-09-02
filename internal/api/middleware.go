package api

import (
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"
)

// Middleware defines a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain constructs a new http.Handler by wrapping the provided handler with a slice of middleware.
// Middleware is applied in the order passed, so the first middleware in the slice executes first.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// responseWriterInterceptor wraps http.ResponseWriter to capture the HTTP status code.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func newResponseWriterInterceptor(w http.ResponseWriter) *responseWriterInterceptor {
	return &responseWriterInterceptor{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriterInterceptor) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriterInterceptor) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// LoggingMiddleware creates a middleware that logs incoming HTTP requests and execution duration.
func LoggingMiddleware(out io.Writer) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rwi := newResponseWriterInterceptor(w)

			next.ServeHTTP(rwi, r)

			duration := time.Since(start)
			remoteAddr := r.RemoteAddr
			if remoteAddr == "" {
				remoteAddr = "-"
			}

			if out != nil {
				fmt.Fprintf(out, "[HTTP] %s %s %d %v - %s\n", r.Method, r.URL.Path, rwi.statusCode, duration, remoteAddr)
			}
		})
	}
}

// RecoveryMiddleware creates a middleware that recovers from panics and returns an HTTP 500 Internal Server Error.
func RecoveryMiddleware(out io.Writer) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					if out != nil {
						fmt.Fprintf(out, "[ERROR] Panic recovered in HTTP handler: %v\nStack trace:\n%s\n", err, string(debug.Stack()))
					}
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware creates a middleware that cancels the request context if processing exceeds duration.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		if timeout <= 0 {
			return next
		}
		msg := `{"error":"Request Timeout"}`
		return http.TimeoutHandler(next, timeout, msg)
	}
}
