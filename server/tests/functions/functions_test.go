package functions_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nodephone/server/internal/kernel"
)

func TestFunctionsSubsystemFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "functions_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test function file in functions directory
	funcDir := filepath.Join(tempDir, "functions")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		t.Fatalf("failed to create functions dir: %v", err)
	}

	jsCode := `
function handler(req, res) {
	console.log("Running echo function");
	res.status(200).json({
		status: "success",
		echo: req.body
	});
}
`
	if err := os.WriteFile(filepath.Join(funcDir, "echo.js"), []byte(jsCode), 0644); err != nil {
		t.Fatalf("failed to write test function file: %v", err)
	}

	stopCh := make(chan os.Signal, 1)
	k := kernel.New(nil, kernel.WithDataDir(tempDir), kernel.WithStopChannel(stopCh), kernel.WithNonBlocking(true))
	if err := k.Boot(); err != nil {
		t.Fatalf("kernel.Boot failed: %v", err)
	}
	defer k.Close()

	ts := httptest.NewServer(k.APIServer().Handler())
	defer ts.Close()

	client := ts.Client()

	// 1. GET /api/functions (List Discovered Functions)
	resp, err := client.Get(ts.URL + "/api/functions")
	if err != nil {
		t.Fatalf("GET /api/functions failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for list functions, got %d", resp.StatusCode)
	}
	var listResult map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&listResult)
	resp.Body.Close()

	funcsList, ok := listResult["functions"].([]interface{})
	if !ok || len(funcsList) == 0 {
		t.Errorf("expected discovered functions in response, got %v", listResult["functions"])
	}

	// 2. Invoke Serverless Function: POST /api/functions/echo
	reqBody, _ := json.Marshal(map[string]string{"message": "Hello NodePhone Functions Engine"})
	resp, err = client.Post(ts.URL+"/api/functions/echo", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("POST /api/functions/echo failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for function execution, got %d", resp.StatusCode)
	}
	var invokeResult struct {
		Result map[string]interface{} `json:"result"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&invokeResult)
	resp.Body.Close()

	if invokeResult.Result["status"] != "success" {
		t.Errorf("expected result.status 'success', got %v", invokeResult.Result["status"])
	}
}
