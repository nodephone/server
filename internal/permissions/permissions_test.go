package permissions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/database"
	"github.com/nodephone/server/internal/permissions"
)

func TestExpressionEvaluator(t *testing.T) {
	u1 := &auth.User{ID: "usr-1", Username: "alice", Role: "user"}
	uAdmin := &auth.User{ID: "usr-2", Username: "admin", Role: "admin"}

	tests := []struct {
		name       string
		expression string
		user       *auth.User
		row        map[string]interface{}
		expected   bool
	}{
		{
			name:       "Literal true",
			expression: "true",
			user:       u1,
			row:        nil,
			expected:   true,
		},
		{
			name:       "Literal false",
			expression: "false",
			user:       u1,
			row:        nil,
			expected:   false,
		},
		{
			name:       "Role check admin",
			expression: "user.role == 'admin'",
			user:       uAdmin,
			row:        nil,
			expected:   true,
		},
		{
			name:       "Role check user fails admin rule",
			expression: "user.role == 'admin'",
			user:       u1,
			row:        nil,
			expected:   false,
		},
		{
			name:       "Row ownership match",
			expression: "user.id == row.user_id",
			user:       u1,
			row:        map[string]interface{}{"user_id": "usr-1"},
			expected:   true,
		},
		{
			name:       "Row ownership mismatch",
			expression: "user.id == row.user_id",
			user:       u1,
			row:        map[string]interface{}{"user_id": "usr-999"},
			expected:   false,
		},
		{
			name:       "Row price condition",
			expression: "row.amount > 100",
			user:       u1,
			row:        map[string]interface{}{"amount": 250},
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evalCtx := &permissions.EvaluationContext{
				User: tc.user,
				Row:  tc.row,
			}
			allowed, err := permissions.EvaluateExpression(tc.expression, evalCtx)
			if err != nil {
				t.Fatalf("EvaluateExpression failed: %v", err)
			}
			if allowed != tc.expected {
				t.Errorf("expected %v, got %v for expression %q", tc.expected, allowed, tc.expression)
			}
		})
	}
}

func TestDefaultDenyAndPolicyEvaluation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "policy_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	pm := permissions.NewPolicyManager(db, nil)
	u1 := &auth.User{ID: "usr-1", Username: "alice", Role: "user"}
	u2 := &auth.User{ID: "usr-2", Username: "bob", Role: "user"}
	uAdmin := &auth.User{ID: "usr-admin", Username: "superadmin", Role: "admin"}

	// 1. DEFAULT-DENY Rule check: No policy created for 'notes' table
	allowed, err := pm.Can(ctx, u1, "SELECT", "notes", nil)
	if err != nil {
		t.Fatalf("Can failed: %v", err)
	}
	if allowed {
		t.Error("expected default-deny behavior for unconfigured table, but allowed=true")
	}

	err = pm.Enforce(ctx, u1, "SELECT", "notes", nil)
	if !errors.Is(err, permissions.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}

	// 2. Admin Bypass check
	adminAllowed, err := pm.Can(ctx, uAdmin, "SELECT", "notes", nil)
	if err != nil || !adminAllowed {
		t.Errorf("expected admin bypass allowed=true, got allowed=%v, err=%v", adminAllowed, err)
	}

	// 3. Create Policy for 'documents' table
	policy, err := pm.CreatePolicy(ctx, permissions.CreatePolicyRequest{
		TableName:  "documents",
		Action:     "SELECT",
		Role:       "user",
		Expression: "user.id == row.owner_id",
	})
	if err != nil {
		t.Fatalf("CreatePolicy failed: %v", err)
	}
	if policy.TableName != "documents" {
		t.Errorf("unexpected policy: %+v", policy)
	}

	// 4. Test ownership match (Alice owns row)
	aliceDoc := map[string]interface{}{"id": "doc-1", "owner_id": "usr-1"}
	aliceAllowed, err := pm.Can(ctx, u1, "SELECT", "documents", aliceDoc)
	if err != nil || !aliceAllowed {
		t.Errorf("expected Alice to access own document, got allowed=%v, err=%v", aliceAllowed, err)
	}

	// 5. Test ownership mismatch (Bob attempts to read Alice's doc)
	bobAllowed, err := pm.Can(ctx, u2, "SELECT", "documents", aliceDoc)
	if err != nil {
		t.Fatalf("Can failed: %v", err)
	}
	if bobAllowed {
		t.Error("expected Bob to be denied access to Alice's document")
	}

	// 6. Delete Policy
	if err := pm.DeletePolicy(ctx, policy.ID); err != nil {
		t.Fatalf("DeletePolicy failed: %v", err)
	}

	// After policy deletion, default-deny resumes
	aliceAfter, _ := pm.Can(ctx, u1, "SELECT", "documents", aliceDoc)
	if aliceAfter {
		t.Error("expected default-deny after policy deletion")
	}
}

func TestPolicyAPIEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "policy_api_test.db")
	db, err := database.Open(dbPath, nil)
	if err != nil {
		t.Fatalf("database.Open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	authService := auth.NewAuthService(db, "testjwtsecret1234567890123456", nil)
	pm := permissions.NewPolicyManager(db, nil)
	policyHandler := permissions.NewPolicyHandler(pm)

	// Create Normal User & Admin User
	_, _ = authService.SignUp(ctx, auth.SignUpRequest{Username: "normaluser", Email: "normal@example.com", Password: "Password123!"})
	loginNormal, _ := authService.LogIn(ctx, auth.LoginRequest{Login: "normaluser", Password: "Password123!"})

	// Manually promote admin user in DB for test
	adminUser, _ := authService.SignUp(ctx, auth.SignUpRequest{Username: "adminuser", Email: "admin@example.com", Password: "Password123!"})
	_, _ = db.ExecContext(ctx, "UPDATE users SET role = 'admin' WHERE id = ?", adminUser.ID)
	loginAdmin, _ := authService.LogIn(ctx, auth.LoginRequest{Login: "adminuser", Password: "Password123!"})

	authMW := auth.AuthMiddleware(authService)
	mux := http.NewServeMux()
	mux.Handle("/api/permissions/policies", authMW(http.HandlerFunc(policyHandler.RoutePolicies)))
	mux.Handle("/api/permissions/policies/", authMW(http.HandlerFunc(policyHandler.RoutePolicies)))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// 1. Normal User attempts to Create Policy -> 403 Forbidden
	body, _ := json.Marshal(permissions.CreatePolicyRequest{TableName: "posts", Action: "SELECT", Role: "user", Expression: "true"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/permissions/policies", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+loginNormal.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Normal user POST /api/permissions/policies failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403 Forbidden for normal user, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Admin User creates Policy -> 201 Created
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/permissions/policies", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+loginAdmin.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Admin POST /api/permissions/policies failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created for admin user, got %d", resp.StatusCode)
	}
	var createdPolicy permissions.Policy
	_ = json.NewDecoder(resp.Body).Decode(&createdPolicy)
	resp.Body.Close()

	// 3. Admin User lists Policies -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/permissions/policies", nil)
	req.Header.Set("Authorization", "Bearer "+loginAdmin.AccessToken)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Admin GET /api/permissions/policies failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Admin User deletes Policy -> 200 OK
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/permissions/policies/"+createdPolicy.ID, nil)
	req.Header.Set("Authorization", "Bearer "+loginAdmin.AccessToken)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Admin DELETE /api/permissions/policies failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
