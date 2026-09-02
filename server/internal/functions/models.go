// Package functions provides serverless function discovery, Goja JavaScript runtime execution,
// request/response context abstractions, timeout enforcement, panic safety, and cron execution for NodePhone.
package functions

import "time"

// FunctionMeta holds metadata and configuration for a discovered function.
type FunctionMeta struct {
	Name         string        `json:"name"`
	Runtime      string        `json:"runtime"` // "js"
	FilePath     string        `json:"file_path"`
	Timeout      time.Duration `json:"timeout"`
	CronSchedule string        `json:"cron_schedule,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
}

// FunctionRequest represents the request object passed into function handlers.
type FunctionRequest struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Headers     map[string]string `json:"headers"`
	QueryParams map[string]string `json:"query_params"`
	Body        interface{}       `json:"body"`
	Env         map[string]string `json:"env"`
}

// FunctionResult holds the response output, status code, headers, and log outputs from an execution.
type FunctionResult struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       interface{}       `json:"body"`
	Logs       []string          `json:"logs"`
	Duration   time.Duration     `json:"duration_ms"`
	Error      string            `json:"error,omitempty"`
}
