package permissions

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// EvaluateExpression evaluates a policy rule expression (e.g., "user.id == row.user_id", "user.role == 'admin'", "true")
// against the provided EvaluationContext using an isolated Goja JS runtime.
func EvaluateExpression(expression string, evalCtx *EvaluationContext) (allowed bool, evalErr error) {
	expr := strings.TrimSpace(expression)
	if expr == "" || expr == "false" {
		return false, nil
	}
	if expr == "true" {
		return true, nil
	}

	vm := goja.New()

	// Panic isolation and safe evaluation error handling
	defer func() {
		if r := recover(); r != nil {
			evalErr = fmt.Errorf("expression evaluation panic: %v", r)
			allowed = false
		}
	}()

	// Bind user object
	if evalCtx != nil && evalCtx.User != nil {
		userObj := map[string]interface{}{
			"id":       evalCtx.User.ID,
			"username": evalCtx.User.Username,
			"email":    evalCtx.User.Email,
			"role":     evalCtx.User.Role,
		}
		_ = vm.Set("user", userObj)
	} else {
		_ = vm.Set("user", nil)
	}

	// Bind row object
	if evalCtx != nil && evalCtx.Row != nil {
		_ = vm.Set("row", evalCtx.Row)
	} else {
		_ = vm.Set("row", map[string]interface{}{})
	}

	script := fmt.Sprintf("(function() { return !!(%s); })();", expr)
	val, err := vm.RunString(script)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate policy expression %q: %w", expr, err)
	}

	return val.ToBoolean(), nil
}
