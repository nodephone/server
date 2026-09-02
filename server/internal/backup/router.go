package backup

import (
	"net/http"

	"github.com/nodephone/server/internal/auth"
)

// RegisterRoutes attaches backup REST endpoints onto the HTTP ServeMux protected by AuthMiddleware.
func RegisterRoutes(mux *http.ServeMux, handler *BackupHandler, authService *auth.AuthService) {
	if mux == nil || handler == nil {
		return
	}

	authMW := auth.AuthMiddleware(authService)

	mux.Handle("/api/backup/create", authMW(http.HandlerFunc(handler.CreateBackup)))
	mux.Handle("/api/backup/restore", authMW(http.HandlerFunc(handler.RestoreBackup)))
	mux.Handle("/api/backup/list", authMW(http.HandlerFunc(handler.ListBackups)))
	mux.Handle("/api/backup/status", authMW(http.HandlerFunc(handler.GetStatus)))
	mux.Handle("/api/backup/", authMW(http.HandlerFunc(handler.RouteDeleteBackup)))
}
