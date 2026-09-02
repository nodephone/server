package deploy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nodephone/server/internal/deploy"
)

func TestDeploymentEngineAndStatus(t *testing.T) {
	cfg := deploy.DefaultDeployConfig()
	engine := deploy.NewDeploymentEngine(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx, 8081); err != nil {
		t.Fatalf("engine.Start failed: %v", err)
	}
	defer engine.Stop()

	status := engine.GetStatus()
	if status.Status != "LIVE" {
		t.Errorf("expected status 'LIVE', got %q", status.Status)
	}
	if status.PublicURL == "" {
		t.Error("expected non-empty PublicURL")
	}
	if !status.SSLActive {
		t.Error("expected SSLActive == true")
	}

	health := engine.GetHealth()
	if health.Status != "healthy" {
		t.Errorf("expected health status 'healthy', got %q", health.Status)
	}

	// Test Bind Custom Domain
	domainInfo, err := engine.BindDomain("api.mycompany.dev")
	if err != nil {
		t.Fatalf("BindDomain failed: %v", err)
	}
	if domainInfo.Domain != "api.mycompany.dev" || !domainInfo.Verified {
		t.Errorf("unexpected domainInfo: %+v", domainInfo)
	}

	updatedStatus := engine.GetStatus()
	if updatedStatus.CustomDomain != "api.mycompany.dev" {
		t.Errorf("expected custom domain 'api.mycompany.dev', got %q", updatedStatus.CustomDomain)
	}
}

func TestDeploymentRESTEndpoints(t *testing.T) {
	cfg := deploy.DefaultDeployConfig()
	engine := deploy.NewDeploymentEngine(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = engine.Start(ctx, 8081)
	defer engine.Stop()

	handler := deploy.NewDeployHandler(engine)
	mux := http.NewServeMux()
	deploy.RegisterRoutes(mux, handler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// 1. GET /deploy/status
	resp, err := client.Get(ts.URL + "/deploy/status")
	if err != nil {
		t.Fatalf("GET /deploy/status failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	var statusRes deploy.DeployStatus
	_ = json.NewDecoder(resp.Body).Decode(&statusRes)
	resp.Body.Close()

	if statusRes.Status != "LIVE" {
		t.Errorf("expected status 'LIVE', got %q", statusRes.Status)
	}

	// 2. GET /deploy/health
	resp, err = client.Get(ts.URL + "/deploy/health")
	if err != nil {
		t.Fatalf("GET /deploy/health failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for health, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. GET /deploy/domain (Initially unbound)
	resp, err = client.Get(ts.URL + "/deploy/domain")
	if err != nil {
		t.Fatalf("GET /deploy/domain failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for domain, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. POST /deploy/domain
	bindBody, _ := json.Marshal(map[string]string{"domain": "api.myapp.com"})
	resp, err = client.Post(ts.URL+"/deploy/domain", "application/json", bytes.NewBuffer(bindBody))
	if err != nil {
		t.Fatalf("POST /deploy/domain failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK for binding domain, got %d", resp.StatusCode)
	}
	var boundRes deploy.CustomDomainInfo
	_ = json.NewDecoder(resp.Body).Decode(&boundRes)
	resp.Body.Close()

	if boundRes.Domain != "api.myapp.com" {
		t.Errorf("expected bound domain 'api.myapp.com', got %q", boundRes.Domain)
	}
}
