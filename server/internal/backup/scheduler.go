package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/nodephone/server/internal/config"
)

// Scheduler manages retention policy enforcement (e.g. keeping last 30 backups),
// pruning outdated snapshots, and automated cron execution.
type Scheduler struct {
	backupDir      string
	retentionLimit int
	out            io.Writer
}

// NewScheduler initializes a new Scheduler instance.
func NewScheduler(backupDir string, retentionLimit int, out io.Writer) *Scheduler {
	if out == nil {
		out = io.Discard
	}
	if retentionLimit <= 0 {
		retentionLimit = config.DefaultBackupRetention
	}
	return &Scheduler{
		backupDir:      backupDir,
		retentionLimit: retentionLimit,
		out:            out,
	}
}

// PruneOldBackups removes backup archives exceeding the configured retention limit (default 30).
func (s *Scheduler) PruneOldBackups() (int, error) {
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var archives []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".npb" {
			archives = append(archives, entry)
		}
	}

	if len(archives) <= s.retentionLimit {
		return 0, nil
	}

	// Sort archives by mod time / name descending (newest first)
	sort.Slice(archives, func(i, j int) bool {
		iInfo, iErr := archives[i].Info()
		jInfo, jErr := archives[j].Info()
		if iErr == nil && jErr == nil {
			return iInfo.ModTime().After(jInfo.ModTime())
		}
		return archives[i].Name() > archives[j].Name()
	})

	prunedCount := 0
	for i := s.retentionLimit; i < len(archives); i++ {
		targetPath := filepath.Join(s.backupDir, archives[i].Name())
		if err := os.Remove(targetPath); err == nil {
			prunedCount++
			fmt.Fprintf(s.out, "[BACKUP] Pruned expired backup archive beyond retention limit: %s\n", archives[i].Name())
		}
	}

	return prunedCount, nil
}
