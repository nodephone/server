package permissions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nodephone/server/internal/auth"
	"github.com/nodephone/server/internal/database"
)

// PolicyManager manages row-level security policy persistence in SQLite and evaluates rules before database operations.
type PolicyManager struct {
	db  *database.DB
	out io.Writer
	mu  sync.RWMutex
}

// NewPolicyManager creates a new PolicyManager instance.
func NewPolicyManager(db *database.DB, out io.Writer) *PolicyManager {
	if out == nil {
		out = io.Discard
	}
	return &PolicyManager{
		db:  db,
		out: out,
	}
}

// CreatePolicy creates and persists a new row-level security policy.
func (pm *PolicyManager) CreatePolicy(ctx context.Context, req CreatePolicyRequest) (*Policy, error) {
	tableName := strings.TrimSpace(strings.ToLower(req.TableName))
	action := strings.TrimSpace(strings.ToUpper(req.Action))
	role := strings.TrimSpace(strings.ToLower(req.Role))
	expression := strings.TrimSpace(req.Expression)

	if tableName == "" || action == "" || role == "" || expression == "" {
		return nil, ErrInvalidPolicy
	}

	// Validate expression syntax
	dummyCtx := &EvaluationContext{
		User:      &auth.User{ID: "dummy", Username: "test", Role: "user"},
		Action:    action,
		TableName: tableName,
		Row:       map[string]interface{}{"id": "1"},
	}
	if _, err := EvaluateExpression(expression, dummyCtx); err != nil {
		return nil, fmt.Errorf("%w: invalid expression syntax: %v", ErrInvalidPolicy, err)
	}

	// Check duplicate policy
	var count int
	err := pm.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM policies WHERE table_name = ? AND action = ? AND role = ?", tableName, action, role).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing policy: %w", err)
	}
	if count > 0 {
		return nil, ErrPolicyExists
	}

	policyID := uuid.New().String()
	now := time.Now().UTC()

	query := `INSERT INTO policies (id, table_name, action, role, expression, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = pm.db.ExecContext(ctx, query, policyID, tableName, action, role, expression, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert policy record: %w", err)
	}

	fmt.Fprintf(pm.out, "[PERMISSIONS] Created policy %s (table=%s, action=%s, role=%s)\n", policyID, tableName, action, role)

	return &Policy{
		ID:         policyID,
		TableName:  tableName,
		Action:     action,
		Role:       role,
		Expression: expression,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// GetPolicy fetches a policy record by ID.
func (pm *PolicyManager) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	var p Policy
	query := `SELECT id, table_name, action, role, expression, created_at, updated_at FROM policies WHERE id = ?`
	err := pm.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.TableName, &p.Action, &p.Role, &p.Expression, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPolicyNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch policy: %w", err)
	}
	return &p, nil
}

// ListPolicies returns all active row-level policies.
func (pm *PolicyManager) ListPolicies(ctx context.Context) ([]*Policy, error) {
	query := `SELECT id, table_name, action, role, expression, created_at, updated_at FROM policies ORDER BY created_at DESC`
	rows, err := pm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list policies: %w", err)
	}
	defer rows.Close()

	var policies []*Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.TableName, &p.Action, &p.Role, &p.Expression, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan policy row: %w", err)
		}
		policies = append(policies, &p)
	}
	return policies, nil
}

// DeletePolicy removes a policy record by ID.
func (pm *PolicyManager) DeletePolicy(ctx context.Context, id string) error {
	p, err := pm.GetPolicy(ctx, id)
	if err != nil {
		return err
	}

	_, err = pm.db.ExecContext(ctx, "DELETE FROM policies WHERE id = ?", p.ID)
	if err != nil {
		return fmt.Errorf("failed to delete policy record: %w", err)
	}

	fmt.Fprintf(pm.out, "[PERMISSIONS] Deleted policy %s (table=%s)\n", id, p.TableName)
	return nil
}

// Can evaluates whether an operation is permitted under defined row-level policies.
// Enforces DEFAULT-DENY behavior if no matching policy allows the operation.
func (pm *PolicyManager) Can(ctx context.Context, user *auth.User, action string, tableName string, row map[string]interface{}) (bool, error) {
	tableName = strings.TrimSpace(strings.ToLower(tableName))
	action = strings.TrimSpace(strings.ToUpper(action))

	// 1. Admin Role Bypass
	if user != nil && user.Role == "admin" {
		return true, nil
	}

	userRole := "guest"
	if user != nil && user.Role != "" {
		userRole = strings.ToLower(user.Role)
	}

	// 2. Fetch matching policies from database
	query := `
SELECT id, table_name, action, role, expression, created_at, updated_at
FROM policies
WHERE table_name = ?
  AND (action = ? OR action = '*')
  AND (role = ? OR role = '*')`

	rows, err := pm.db.QueryContext(ctx, query, tableName, action, userRole)
	if err != nil {
		return false, fmt.Errorf("failed to query policies for evaluation: %w", err)
	}
	defer rows.Close()

	var matchingPolicies []*Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.TableName, &p.Action, &p.Role, &p.Expression, &p.CreatedAt, &p.UpdatedAt); err == nil {
			matchingPolicies = append(matchingPolicies, &p)
		}
	}

	// 3. DEFAULT-DENY Rule: If no policy rule exists for this table & action -> Deny!
	if len(matchingPolicies) == 0 {
		return false, nil
	}

	evalCtx := &EvaluationContext{
		User:      user,
		Action:    action,
		TableName: tableName,
		Row:       row,
	}

	// 4. Evaluate matching policies. If any policy evaluates to true -> Allow!
	for _, p := range matchingPolicies {
		allowed, evalErr := EvaluateExpression(p.Expression, evalCtx)
		if evalErr == nil && allowed {
			return true, nil
		}
	}

	return false, nil
}

// Enforce evaluates policy rules and returns ErrPermissionDenied if the operation is rejected.
func (pm *PolicyManager) Enforce(ctx context.Context, user *auth.User, action string, tableName string, row map[string]interface{}) error {
	allowed, err := pm.Can(ctx, user, action, tableName, row)
	if err != nil {
		return err
	}
	if !allowed {
		username := "guest"
		if user != nil {
			username = user.Username
		}
		fmt.Fprintf(pm.out, "[PERMISSIONS] Denied %s operation on %s for user %s\n", action, tableName, username)
		return ErrPermissionDenied
	}
	return nil
}
