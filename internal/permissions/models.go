// Package permissions implements SQLite-backed row-level security (RLS) policies,
// boolean rule evaluation against user context and row data, default-deny safety enforcement,
// and 403 Forbidden structured responses for NodePhone Server.
package permissions

import (
	"errors"
	"time"

	"github.com/nodephone/server/internal/auth"
)

var (
	ErrPermissionDenied = errors.New("403 Forbidden: permission denied by row-level policy")
	ErrPolicyNotFound   = errors.New("policy not found")
	ErrPolicyExists     = errors.New("policy rule already exists for table, action, and role")
	ErrInvalidPolicy    = errors.New("invalid policy parameters or expression")
)

// Policy represents a row-level access control rule stored in SQLite.
type Policy struct {
	ID        string    `json:"id"`
	TableName string    `json:"table_name"`
	Action    string    `json:"action"` // SELECT, INSERT, UPDATE, DELETE, or *
	Role      string    `json:"role"`   // admin, user, guest, or *
	Expression string   `json:"expression"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreatePolicyRequest defines the payload required to create a row-level policy.
type CreatePolicyRequest struct {
	TableName  string `json:"table_name"`
	Action     string `json:"action"`
	Role       string `json:"role"`
	Expression string `json:"expression"`
}

// EvaluationContext encapsulates the user identity, operation action, target table,
// and row attribute map evaluated during policy checks.
type EvaluationContext struct {
	User      *auth.User
	Action    string
	TableName string
	Row       map[string]interface{}
}
