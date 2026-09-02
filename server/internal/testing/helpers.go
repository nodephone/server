// Package testing provides reusable test utilities, isolated kernel bootstrappers,
// test HTTP servers, and helper token generators for the NodePhone Server test suites.
package testing

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/kernel"
)

// TestEnvironment holds references to an isolated test kernel runtime and teardown functions.
type TestEnvironment struct {
	Kernel   *kernel.Kernel
	DataDir  string
	Server   *httptest.Server
	Cleanup  func()
}

// SetupTestEnv initializes a clean, isolated Kernel instance in a temporary data directory with clean teardown.
func SetupTestEnv(t *testing.T) *TestEnvironment {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "nodephone_test_*")
	if err != nil {
		t.Fatalf("failed to create temp test data directory: %v", err)
	}

	stopCh := make(chan os.Signal, 1)
	k := kernel.New(nil, kernel.WithDataDir(tempDir), kernel.WithStopChannel(stopCh), kernel.WithNonBlocking(true))

	if err := k.Boot(); err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("failed to boot test kernel: %v", err)
	}

	ts := httptest.NewServer(k.APIServer().Handler())

	cleanup := func() {
		ts.Close()
		_ = k.Close()
		_ = os.RemoveAll(tempDir)
	}

	return &TestEnvironment{
		Kernel:  k,
		DataDir: tempDir,
		Server:  ts,
		Cleanup: cleanup,
	}
}

// CreateTestUser signs up a test user and returns the user object and valid JWT access token.
func CreateTestUser(t *testing.T, k *kernel.Kernel, username, email, role string) (*auth.User, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, err := k.AuthService().SignUp(ctx, auth.SignUpRequest{
		Username: username,
		Email:    email,
		Password: "TestPassword123!",
	})
	if err != nil {
		t.Fatalf("CreateTestUser SignUp failed for %s: %v", username, err)
	}

	if role != "user" && role != "" {
		_, err := k.DB().ExecContext(ctx, "UPDATE users SET role = ? WHERE id = ?", role, user.ID)
		if err != nil {
			t.Fatalf("failed to set role %s for user %s: %v", role, username, err)
		}
		user.Role = role
	}

	loginResp, err := k.AuthService().LogIn(ctx, auth.LoginRequest{
		Login:    username,
		Password: "TestPassword123!",
	})
	if err != nil {
		t.Fatalf("CreateTestUser LogIn failed for %s: %v", username, err)
	}

	return user, loginResp.AccessToken
}

// CreateTestAdmin creates an admin user and returns the user object and valid JWT access token.
func CreateTestAdmin(t *testing.T, k *kernel.Kernel) (*auth.User, string) {
	t.Helper()
	return CreateTestUser(t, k, "adminuser_"+randomSuffix(), "admin_"+randomSuffix()+"@example.com", "admin")
}

func randomSuffix() string {
	return filepath.Base(os.Args[0])
}
