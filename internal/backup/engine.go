package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nodephone/server/internal/config"
)

// BackupEngine coordinates server snapshot creation, full server restoration, retention pruning,
// verification probes, and disaster recovery operations.
type BackupEngine struct {
	dataDir          string
	backupDir        string
	archiver         *Archiver
	snapshotter      *Snapshotter
	verifier         *Verifier
	restorer         *Restorer
	scheduler        *Scheduler
	status           string
	lastBackup       *BackupMetadata
	lastBackupTime   *time.Time
	lastRestoreTime  *time.Time
	lastBackupStatus string
	retentionLimit   int
	out              io.Writer
	mu               sync.RWMutex
}

// NewBackupEngine initializes a new BackupEngine instance.
func NewBackupEngine(dataDir string, out io.Writer) *BackupEngine {
	if out == nil {
		out = io.Discard
	}

	backupDir := filepath.Join(dataDir, "backups")
	retention := config.DefaultBackupRetention
	if retention <= 0 {
		retention = 30
	}

	return &BackupEngine{
		dataDir:          dataDir,
		backupDir:        backupDir,
		archiver:         NewArchiver(),
		snapshotter:      NewSnapshotter(dataDir, out),
		verifier:         NewVerifier(),
		restorer:         NewRestorer(dataDir, out),
		scheduler:        NewScheduler(backupDir, retention, out),
		status:           "idle",
		lastBackupStatus: "none",
		retentionLimit:   retention,
		out:              out,
	}
}

// CreateBackup generates an atomic .npb backup snapshot archive of server state.
func (be *BackupEngine) CreateBackup(req CreateBackupRequest) (*BackupMetadata, error) {
	be.mu.Lock()
	defer be.mu.Unlock()

	be.status = "backing_up"
	defer func() { be.status = "idle" }()

	now := time.Now().UTC()
	var filename string
	if req.Name != "" {
		clean := strings.TrimSuffix(req.Name, ".npb")
		filename = fmt.Sprintf("%s.npb", clean)
	} else {
		filename = fmt.Sprintf("%s.npb", now.Format("2006-01-02_15-04-05"))
	}

	destPath := filepath.Join(be.backupDir, filename)

	fmt.Fprintf(be.out, "[BACKUP] Initiating server backup snapshot creation: %s\n", filename)

	files, err := be.snapshotter.CollectFiles(req.IncludeLogs)
	if err != nil {
		be.lastBackupStatus = "failed"
		return nil, fmt.Errorf("failed to collect snapshot files: %w", err)
	}

	meta, err := be.archiver.CreateArchive(destPath, files, req.EncryptPassword)
	if err != nil {
		be.lastBackupStatus = "failed"
		return nil, fmt.Errorf("failed to create backup archive: %w", err)
	}

	// Prune backups exceeding retention limit
	_, _ = be.scheduler.PruneOldBackups()

	be.lastBackup = meta
	be.lastBackupTime = &now
	be.lastBackupStatus = "success"

	fmt.Fprintf(be.out, "[BACKUP] Backup snapshot created successfully (%d bytes): %s\n", meta.SizeBytes, meta.Filename)
	return meta, nil
}

// RestoreBackup validates and restores server state from an existing backup snapshot.
func (be *BackupEngine) RestoreBackup(req RestoreBackupRequest) error {
	be.mu.Lock()
	defer be.mu.Unlock()

	be.status = "restoring"
	defer func() { be.status = "idle" }()

	if req.BackupID == "" {
		return fmt.Errorf("backup_id is required")
	}

	filename := req.BackupID
	if !strings.HasSuffix(filename, ".npb") {
		filename = filename + ".npb"
	}

	archivePath := filepath.Join(be.backupDir, filename)
	if _, err := os.Stat(archivePath); err != nil {
		return fmt.Errorf("backup archive %s not found", filename)
	}

	if err := be.restorer.Restore(archivePath, req.DecryptPassword); err != nil {
		return fmt.Errorf("restore operation failed: %w", err)
	}

	now := time.Now().UTC()
	be.lastRestoreTime = &now
	return nil
}

// ListBackups returns all available .npb backup snapshot metadata in the backup directory.
func (be *BackupEngine) ListBackups() ([]*BackupMetadata, error) {
	be.mu.RLock()
	defer be.mu.RUnlock()

	entries, err := os.ReadDir(be.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*BackupMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var list []*BackupMetadata

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".npb" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		filePath := filepath.Join(be.backupDir, entry.Name())
		fileBytes, err := os.ReadFile(filePath)
		hashStr := ""
		if err == nil {
			hash := sha256.Sum256(fileBytes)
			hashStr = hex.EncodeToString(hash[:])
		}

		baseName := entry.Name()
		id := strings.TrimSuffix(baseName, ".npb")

		list = append(list, &BackupMetadata{
			ID:            id,
			Filename:      baseName,
			SizeBytes:     info.Size(),
			SHA256Hash:    hashStr,
			Encrypted:     false,
			IncludesDB:    true,
			IncludesFiles: true,
			IncludesFuncs: true,
			IncludesConfig: true,
			CreatedAt:     info.ModTime().UTC(),
		})
	}

	// Sort newest first
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list, nil
}

// DeleteBackup deletes a specific backup archive by ID.
func (be *BackupEngine) DeleteBackup(backupID string) error {
	be.mu.Lock()
	defer be.mu.Unlock()

	filename := backupID
	if !strings.HasSuffix(filename, ".npb") {
		filename = filename + ".npb"
	}

	targetPath := filepath.Join(be.backupDir, filename)
	if err := os.Remove(targetPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup snapshot %s not found", filename)
		}
		return fmt.Errorf("failed to delete backup %s: %w", filename, err)
	}

	fmt.Fprintf(be.out, "[BACKUP] Deleted backup snapshot %s\n", filename)
	return nil
}

// GetStatus returns current runtime backup subsystem status and metrics.
func (be *BackupEngine) GetStatus() BackupStatus {
	be.mu.RLock()
	defer be.mu.RUnlock()

	list, _ := be.ListBackups()
	var totalSize int64
	for _, item := range list {
		totalSize += item.SizeBytes
	}

	return BackupStatus{
		Status:           be.status,
		TotalBackups:     len(list),
		TotalSizeBytes:   totalSize,
		LastBackupTime:   be.lastBackupTime,
		LastRestoreTime:  be.lastRestoreTime,
		LastBackupStatus: be.lastBackupStatus,
		ScheduledCount:   0,
		RetentionLimit:   be.retentionLimit,
		LastBackup:       be.lastBackup,
	}
}
