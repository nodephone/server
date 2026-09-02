// Package backup implements atomic, compressed, and encrypted .npb server backups,
// full server restoration, scheduled retention pruning, archive verification,
// and REST API endpoints for NodePhone disaster recovery.
package backup

import "time"

// BackupMetadata holds details about a created .npb backup snapshot archive.
type BackupMetadata struct {
	ID           string    `json:"id"`            // e.g. "2026-09-02_22-30-15"
	Filename     string    `json:"filename"`      // e.g. "2026-09-02_22-30-15.npb"
	SizeBytes    int64     `json:"size_bytes"`
	SHA256Hash   string    `json:"sha256_hash"`
	Encrypted    bool      `json:"encrypted"`
	IncludesDB   bool      `json:"includes_db"`
	IncludesFiles bool     `json:"includes_files"`
	IncludesFuncs bool     `json:"includes_funcs"`
	IncludesConfig bool    `json:"includes_config"`
	CreatedAt    time.Time `json:"created_at"`
}

// BackupStatus represents runtime backup subsystem metrics and status.
type BackupStatus struct {
	Status           string          `json:"status"`            // "idle", "backing_up", "restoring"
	TotalBackups     int             `json:"total_backups"`
	TotalSizeBytes   int64           `json:"total_size_bytes"`
	LastBackupTime   *time.Time      `json:"last_backup_time,omitempty"`
	LastRestoreTime  *time.Time      `json:"last_restore_time,omitempty"`
	LastBackupStatus string          `json:"last_backup_status"` // "success", "failed", "none"
	ScheduledCount   int             `json:"scheduled_count"`
	RetentionLimit   int             `json:"retention_limit"`
	LastBackup       *BackupMetadata `json:"last_backup,omitempty"`
}

// CreateBackupRequest defines parameters for generating a new backup archive.
type CreateBackupRequest struct {
	Name            string `json:"name,omitempty"`
	EncryptPassword string `json:"encrypt_password,omitempty"`
	IncludeLogs     bool   `json:"include_logs,omitempty"`
}

// RestoreBackupRequest defines parameters for restoring server state from a backup archive.
type RestoreBackupRequest struct {
	BackupID        string `json:"backup_id"`
	DecryptPassword string `json:"decrypt_password,omitempty"`
}

// BackupItem represents an individual file entry inside the .npb archive manifest.
type BackupItem struct {
	Path     string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Hash     string `json:"hash"`
}
