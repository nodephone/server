package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Snapshotter discovers and collects system files (SQLite database, config, secrets,
// storage buckets, serverless functions, logs) for backup archiving.
type Snapshotter struct {
	dataDir string
	out     io.Writer
}

// NewSnapshotter initializes a new Snapshotter instance.
func NewSnapshotter(dataDir string, out io.Writer) *Snapshotter {
	if out == nil {
		out = io.Discard
	}
	return &Snapshotter{
		dataDir: dataDir,
		out:     out,
	}
}

// CollectFiles scans the nodephone-data directory and constructs a mapping of archive paths to physical files.
func (s *Snapshotter) CollectFiles(includeLogs bool) (map[string]string, error) {
	files := make(map[string]string)

	err := filepath.Walk(s.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable paths
		}

		if info.IsDir() {
			rel, _ := filepath.Rel(s.dataDir, path)
			if rel == "backups" || rel == "temp" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(s.dataDir, path)
		if err != nil {
			return nil
		}

		relSlash := filepath.ToSlash(rel)

		// Exclude backups directory contents
		if strings.HasPrefix(relSlash, "backups/") || relSlash == "backups" {
			return nil
		}

		// Exclude logs if requested
		if !includeLogs && strings.HasPrefix(relSlash, "logs/") {
			return nil
		}

		files[relSlash] = path
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to collect snapshot files: %w", err)
	}

	return files, nil
}
