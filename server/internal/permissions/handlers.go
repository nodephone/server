package permissions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nodephone/server/internal/auth"
)

// PolicyHandler exposes REST API endpoints for policy management.
type PolicyHandler struct {
	manager *PolicyManager
}

// NewPolicyHandler creates a new PolicyHandler instance.
func NewPolicyHandler(manager *PolicyManager) *PolicyHandler {
	return &PolicyHandler{
		manager: manager,
	}
}

// Manager returns the underlying PolicyManager.
func (h *PolicyHandler) Manager() *PolicyManager {
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

// RoutePolicies routes requests for /api/permissions/policies and subpaths.
func (h *PolicyHandler) RoutePolicies(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Policy administration requires admin privileges
	if user.Role != "admin" {
		writeErrorResponse(w, http.StatusForbidden, "Forbidden: policy administration requires admin role")
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/api/permissions/policies")
	trimmed = strings.TrimPrefix(trimmed, "/")

	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			h.ListPolicies(w, r)
		case http.MethodPost:
			h.CreatePolicy(w, r)
		default:
			writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	policyID := trimmed
	if r.Method == http.MethodDelete {
		h.DeletePolicy(w, r, policyID)
		return
	}

	writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
}

// CreatePolicy handles POST /api/permissions/policies.
func (h *PolicyHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var req CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	policy, err := h.manager.CreatePolicy(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrPolicyExists) {
			writeErrorResponse(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidPolicy) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create policy")
		return
	}

	writeJSONResponse(w, http.StatusCreated, policy)
}

// ListPolicies handles GET /api/permissions/policies.
func (h *PolicyHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.manager.ListPolicies(r.Context())
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list policies")
		return
	}

	if policies == nil {
		policies = []*Policy{}
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"policies": policies,
	})
}

// DeletePolicy handles DELETE /api/permissions/policies/{id}.
func (h *PolicyHandler) DeletePolicy(w http.ResponseWriter, r *http.Request, policyID string) {
	if err := h.manager.DeletePolicy(r.Context(), policyID); err != nil {
		if errors.Is(err, ErrPolicyNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete policy")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Policy %q deleted successfully", policyID),
	})
}
