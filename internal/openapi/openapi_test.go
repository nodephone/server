package openapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nodephone/server/internal/config"
	"github.com/nodephone/server/internal/openapi"
)

func TestEngineSpecCompilation(t *testing.T) {
	cfg := config.DefaultConfig("testdata", "testsecret")
	engine := openapi.NewEngine(cfg, nil)

	routes := engine.GetRoutes()
	if len(routes) == 0 {
		t.Fatal("expected discovered routes, got 0")
	}

	specBytes := engine.GetSpecBytes()
	if len(specBytes) == 0 {
		t.Fatal("expected cached OpenAPI spec bytes, got 0")
	}

	var spec openapi.OpenAPI
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("failed to unmarshal OpenAPI spec JSON: %v", err)
	}

	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI version 3.1.0, got %q", spec.OpenAPI)
	}

	if spec.Info.Title != "NodePhone API" {
		t.Errorf("expected Info.Title 'NodePhone API', got %q", spec.Info.Title)
	}

	if len(spec.Paths) == 0 {
		t.Error("expected populated paths map in specification")
	}

	if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Error("expected bearerAuth securityScheme in components")
	}
}

func TestDocumentationEndpoints(t *testing.T) {
	cfg := config.DefaultConfig("testdata", "testsecret")
	engine := openapi.NewEngine(cfg, nil)
	handler := openapi.NewOpenAPIHandler(engine)

	mux := http.NewServeMux()
	openapi.RegisterRoutes(mux, handler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// 1. GET /docs/openapi.json
	resp, err := client.Get(ts.URL + "/docs/openapi.json")
	if err != nil {
		t.Fatalf("GET /docs/openapi.json failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var specMap map[string]interface{}
	if err := json.Unmarshal(body, &specMap); err != nil {
		t.Fatalf("failed to parse /docs/openapi.json body: %v", err)
	}

	// 2. GET /docs/routes
	resp, err = client.Get(ts.URL + "/docs/routes")
	if err != nil {
		t.Fatalf("GET /docs/routes failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. GET /docs (Interactive Explorer UI)
	resp, err = client.Get(ts.URL + "/docs")
	if err != nil {
		t.Fatalf("GET /docs failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", resp.Header.Get("Content-Type"))
	}
	htmlBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(htmlBody), "spec-url=\"/docs/openapi.json\"") {
		t.Errorf("expected RapiDoc UI html to contain spec-url, got:\n%s", string(htmlBody))
	}
}
