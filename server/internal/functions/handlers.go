package functions

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// FunctionHandler handles HTTP invocation requests for serverless functions.
type FunctionHandler struct {
	manager *FunctionManager
}

// NewFunctionHandler creates a new FunctionHandler instance.
func NewFunctionHandler(manager *FunctionManager) *FunctionHandler {
	return &FunctionHandler{
		manager: manager,
	}
}

// Manager returns the underlying FunctionManager.
func (h *FunctionHandler) Manager() *FunctionManager {
	return h.manager
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	writeJSONResponse(w, statusCode, map[string]string{"error": message})
}

// RouteFunctions routes requests for /api/functions and /api/functions/{name}/*
func (h *FunctionHandler) RouteFunctions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/functions")
	trimmed = strings.TrimPrefix(trimmed, "/")

	if trimmed == "" {
		if r.Method == http.MethodGet {
			h.ListFunctions(w, r)
			return
		}
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	parts := strings.SplitN(trimmed, "/", 2)
	funcName := parts[0]

	h.InvokeFunction(w, r, funcName)
}

// ListFunctions handles GET /api/functions.
func (h *FunctionHandler) ListFunctions(w http.ResponseWriter, r *http.Request) {
	funcs := h.manager.ListFunctions()
	if funcs == nil {
		funcs = []*FunctionMeta{}
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"functions": funcs,
	})
}

// InvokeFunction handles execution of a named function.
func (h *FunctionHandler) InvokeFunction(w http.ResponseWriter, r *http.Request, funcName string) {
	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	queryParams := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	var reqBody interface{}
	if r.Body != nil && r.ContentLength != 0 {
		rawBody, err := io.ReadAll(r.Body)
		if err == nil && len(rawBody) > 0 {
			var jsonVal interface{}
			if jsonErr := json.Unmarshal(rawBody, &jsonVal); jsonErr == nil {
				reqBody = jsonVal
			} else {
				reqBody = string(rawBody)
			}
		}
	}

	funcReq := &FunctionRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		Headers:     headers,
		QueryParams: queryParams,
		Body:        reqBody,
		Env:         make(map[string]string),
	}

	result, err := h.manager.Invoke(r.Context(), funcName, funcReq)
	if err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Set execution headers
	for k, v := range result.Headers {
		w.Header().Set(k, v)
	}

	statusCode := result.StatusCode
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}

	writeJSONResponse(w, statusCode, map[string]interface{}{
		"result":   result.Body,
		"logs":     result.Logs,
		"duration": result.Duration.String(),
	})
}
