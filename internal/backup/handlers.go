package backup

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nodephone/server/internal/auth"
)

// BackupHandler exposes REST API endpoints for server backup creation, full restoration, listing, deletion, and metrics status.
type BackupHandler struct {
	engine *BackupEngine
}

// NewBackupHandler initializes a new BackupHandler instance.
func NewBackupHandler(engine *BackupEngine) *BackupHandler {
	return &BackupHandler{
		engine: engine,
	}
}

// Engine returns the underlying BackupEngine.
func (h *BackupHandler) Engine() *BackupEngine {
	return h.engine
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

// ensureAdmin verifies that the requesting user possesses administrative privileges.
func (h *BackupHandler) ensureAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil || user.Role != "admin" {
		writeErrorResponse(w, http.StatusForbidden, "Forbidden: Administrative role required for disaster recovery operations")
		return false
	}
	return true
}

// CreateBackup handles POST /api/backup/create.
func (h *BackupHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !h.ensureAdmin(w, r) {
		return
	}

	var req CreateBackupRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	meta, err := h.engine.CreateBackup(req)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSONResponse(w, http.StatusCreated, meta)
}

// RestoreBackup handles POST /api/backup/restore.
func (h *BackupHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !h.ensureAdmin(w, r) {
		return
	}

	var req RestoreBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := h.engine.RestoreBackup(req); err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{"message": "Server restoration completed successfully"})
}

// ListBackups handles GET /api/backup/list.
func (h *BackupHandler) ListBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !h.ensureAdmin(w, r) {
		return
	}

	list, err := h.engine.ListBackups()
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	if list == nil {
		list = []*BackupMetadata{}
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"backups": list,
	})
}

// GetStatus handles GET /api/backup/status.
func (h *BackupHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !h.ensureAdmin(w, r) {
		return
	}

	status := h.engine.GetStatus()
	writeJSONResponse(w, http.StatusOK, status)
}

// RouteDeleteBackup handles DELETE /api/backup/{id}.
func (h *BackupHandler) RouteDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if !h.ensureAdmin(w, r) {
		return
	}

	if r.Method != http.MethodDelete {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeErrorResponse(w, http.StatusBadRequest, "Missing backup ID in path")
		return
	}

	backupID := parts[2]
	if err := h.engine.DeleteBackup(backupID); err != nil {
		writeErrorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{"message": "Backup snapshot deleted successfully"})
}
