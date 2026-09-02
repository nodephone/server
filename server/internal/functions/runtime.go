package functions

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// Runtime defines the interface for executing serverless function runtimes (JS, Go, etc.).
type Runtime interface {
	Name() string
	Execute(ctx context.Context, code string, req *FunctionRequest) (*FunctionResult, error)
}

// JSRuntime implements the Runtime interface using the pure-Go Goja ECMAScript engine.
type JSRuntime struct{}

// NewJSRuntime creates a new JSRuntime instance.
func NewJSRuntime() *JSRuntime {
	return &JSRuntime{}
}

// Name returns the runtime engine identifier.
func (r *JSRuntime) Name() string {
	return "js"
}

// Execute runs JavaScript code inside an isolated Goja VM with request/response bindings,
// console logging, and timeout interrupts.
func (r *JSRuntime) Execute(ctx context.Context, code string, req *FunctionRequest) (resResult *FunctionResult, execErr error) {
	start := time.Now()
	vm := goja.New()

	var logs []string
	var logsMu sync.Mutex

	appendLog := func(prefix string, args []goja.Value) {
		logsMu.Lock()
		defer logsMu.Unlock()
		var parts []string
		for _, arg := range args {
			if arg != nil {
				parts = append(parts, arg.String())
			} else {
				parts = append(parts, "null")
			}
		}
		logs = append(logs, fmt.Sprintf("%s %s", prefix, strings.Join(parts, " ")))
	}

	// Timeout interrupt watcher
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt("function execution timeout exceeded")
		case <-done:
		}
	}()

	// Panic isolation & exception recovery
	defer func() {
		if r := recover(); r != nil {
			errStr := fmt.Sprintf("runtime error: %v", r)
			resResult = &FunctionResult{
				StatusCode: http.StatusInternalServerError,
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       map[string]string{"error": errStr},
				Logs:       logs,
				Duration:   time.Since(start),
				Error:      errStr,
			}
			execErr = nil
		}
	}()

	// Bind console logging
	consoleObj := vm.NewObject()
	_ = consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
		appendLog("[LOG]", call.Arguments)
		return goja.Undefined()
	})
	_ = consoleObj.Set("error", func(call goja.FunctionCall) goja.Value {
		appendLog("[ERROR]", call.Arguments)
		return goja.Undefined()
	})
	_ = consoleObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		appendLog("[WARN]", call.Arguments)
		return goja.Undefined()
	})
	_ = vm.Set("console", consoleObj)

	// Response state variables
	statusCode := http.StatusOK
	resHeaders := make(map[string]string)
	resHeaders["Content-Type"] = "application/json"
	var resBody interface{}

	// Bind res object methods
	resObj := vm.NewObject()
	_ = resObj.Set("status", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			statusCode = int(call.Arguments[0].ToInteger())
		}
		return resObj
	})

	_ = resObj.Set("header", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			k := call.Arguments[0].String()
			v := call.Arguments[1].String()
			resHeaders[k] = v
		}
		return resObj
	})

	_ = resObj.Set("json", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			resBody = call.Arguments[0].Export()
		}
		resHeaders["Content-Type"] = "application/json"
		return resObj
	})

	_ = resObj.Set("send", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			resBody = call.Arguments[0].Export()
		}
		return resObj
	})

	_ = vm.Set("res", resObj)

	// Bind req object
	if req == nil {
		req = &FunctionRequest{}
	}
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	if req.QueryParams == nil {
		req.QueryParams = make(map[string]string)
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}

	reqObj := map[string]interface{}{
		"method":  req.Method,
		"path":    req.Path,
		"headers": req.Headers,
		"query":   req.QueryParams,
		"body":    req.Body,
		"env":     req.Env,
	}
	_ = vm.Set("req", reqObj)

	// Wrap user code in a function execution scope
	wrapper := fmt.Sprintf(`
(function() {
	var exports = {};
	var module = { exports: exports };
	%s
	if (typeof handler === 'function') {
		handler(req, res);
	} else if (typeof module.exports === 'function') {
		module.exports(req, res);
	} else if (typeof exports.default === 'function') {
		exports.default(req, res);
	}
})();
`, code)

	_, err := vm.RunString(wrapper)
	if err != nil {
		errStr := err.Error()
		return &FunctionResult{
			StatusCode: http.StatusInternalServerError,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       map[string]string{"error": errStr},
			Logs:       logs,
			Duration:   time.Since(start),
			Error:      errStr,
		}, nil
	}

	return &FunctionResult{
		StatusCode: statusCode,
		Headers:    resHeaders,
		Body:       resBody,
		Logs:       logs,
		Duration:   time.Since(start),
	}, nil
}
