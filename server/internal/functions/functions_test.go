package functions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodephone/server/internal/functions"
)

func TestJSExecutionAndContext(t *testing.T) {
	rt := functions.NewJSRuntime()
	ctx := context.Background()

	code := `
console.log("Starting calculation...");
console.warn("Test warning");
var num = req.body.x + req.body.y;
res.status(201).json({ result: num, env_val: req.env.SECRET });
`

	req := &functions.FunctionRequest{
		Method: "POST",
		Path:   "/api/functions/calc",
		Body:   map[string]interface{}{"x": 10, "y": 20},
		Env:    map[string]string{"SECRET": "abc123"},
	}

	result, err := rt.Execute(ctx, code, req)
	if err != nil {
		t.Fatalf("rt.Execute failed: %v", err)
	}

	if result.StatusCode != 201 {
		t.Errorf("expected status code 201, got %d", result.StatusCode)
	}

	bodyMap, ok := result.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body, got %T", result.Body)
	}

	if bodyMap["result"] != int64(30) && bodyMap["result"] != float64(30) {
		t.Errorf("expected result 30, got %v", bodyMap["result"])
	}

	if len(result.Logs) < 2 || !strings.Contains(result.Logs[0], "[LOG] Starting calculation...") {
		t.Errorf("unexpected logs captured: %+v", result.Logs)
	}
}

func TestTimeoutAndPanicIsolation(t *testing.T) {
	rt := functions.NewJSRuntime()

	// 1. Test timeout interrupt
	timeoutCode := `while(true) {}`
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := rt.Execute(ctx, timeoutCode, nil)
	if err != nil {
		t.Fatalf("Execute unexpected error: %v", err)
	}

	if result.Error == "" || !strings.Contains(result.Error, "timeout") {
		t.Errorf("expected timeout error in result, got result: %+v", result)
	}

	// 2. Test syntax error / runtime exception isolation
	invalidCode := `throw new Error("Something went BOOM!");`
	ctx2 := context.Background()
	result2, err2 := rt.Execute(ctx2, invalidCode, nil)
	if err2 != nil {
		t.Fatalf("Execute unexpected error: %v", err2)
	}

	if result2.StatusCode != 500 || !strings.Contains(result2.Error, "BOOM!") {
		t.Errorf("expected 500 error with BOOM!, got result: %+v", result2)
	}
}

func TestFunctionDiscoveryAndHTTPInvocation(t *testing.T) {
	tempDir := t.TempDir()
	functionsDir := filepath.Join(tempDir, "functions")
	if err := os.MkdirAll(functionsDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}

	// Create sample function file
	jsContent := `
// @timeout 2
function handler(req, res) {
	console.log("Greeting", req.body.name);
	res.status(200).json({ greeting: "Hello " + req.body.name });
}
`
	helloPath := filepath.Join(functionsDir, "greet.js")
	if err := os.WriteFile(helloPath, []byte(jsContent), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fm, err := functions.NewFunctionManager(functionsDir, nil)
	if err != nil {
		t.Fatalf("NewFunctionManager failed: %v", err)
	}

	if err := fm.Discover(); err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	funcs := fm.ListFunctions()
	if len(funcs) != 1 || funcs[0].Name != "greet" {
		t.Errorf("expected discovered function 'greet', got %+v", funcs)
	}

	// HTTP Invocation
	handler := functions.NewFunctionHandler(fm)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/functions", handler.RouteFunctions)
	mux.HandleFunc("/api/functions/", handler.RouteFunctions)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// GET /api/functions -> list functions
	resp, err := ts.Client().Get(ts.URL + "/api/functions")
	if err != nil {
		t.Fatalf("GET /api/functions failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// POST /api/functions/greet -> invoke function
	postBody, _ := json.Marshal(map[string]string{"name": "Sagar"})
	resp, err = ts.Client().Post(ts.URL+"/api/functions/greet", "application/json", bytes.NewBuffer(postBody))
	if err != nil {
		t.Fatalf("POST /api/functions/greet failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}

	var invokeResp struct {
		Result struct {
			Greeting string `json:"greeting"`
		} `json:"result"`
		Logs []string `json:"logs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&invokeResp); err != nil {
		t.Fatalf("failed to decode invoke response: %v", err)
	}
	resp.Body.Close()

	if invokeResp.Result.Greeting != "Hello Sagar" {
		t.Errorf("expected 'Hello Sagar', got %q", invokeResp.Result.Greeting)
	}

	if len(invokeResp.Logs) < 1 || !strings.Contains(invokeResp.Logs[0], "Greeting Sagar") {
		t.Errorf("expected captured log 'Greeting Sagar', got %+v", invokeResp.Logs)
	}
}
