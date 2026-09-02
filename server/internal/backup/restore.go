package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Restorer handles full server restoration from a verified .npb backup archive.
type Restorer struct {
	dataDir  string
	archiver *Archiver
	verifier *Verifier
	out      io.Writer
}

// NewRestorer initializes a new Restorer instance.
func NewRestorer(dataDir string, out io.Writer) *Restorer {
	if out == nil {
		out = io.Discard
	}
	return &Restorer{
		dataDir:  dataDir,
		archiver: NewArchiver(),
		verifier: NewVerifier(),
		out:      out,
	}
}

// Restore validates the target .npb archive and overwrites server files atomically.
func (r *Restorer) Restore(archivePath string, password string) error {
	fmt.Fprintf(r.out, "[BACKUP] Verifying archive %s before restoration...\n", filepath.Base(archivePath))

	if _, err := r.verifier.VerifyArchive(archivePath, password); err != nil {
		return fmt.Errorf("pre-restore verification failed: %w", err)
	}

	tempStageDir, err := os.MkdirTemp("", "npb_restore_*")
	if err != nil {
		return fmt.Errorf("failed to create temporary staging directory: %w", err)
	}
	defer os.RemoveAll(tempStageDir)

	manifest, err := r.archiver.ExtractArchive(archivePath, tempStageDir, password)
	if err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	fmt.Fprintf(r.out, "[BACKUP] Extracting %d verified items into server data directory...\n", len(manifest))

	// Copy extracted files to destination dataDir
	for relPath := range manifest {
		stagedPath := filepath.Join(tempStageDir, relPath)
		targetPath := filepath.Join(r.dataDir, relPath)

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", targetPath, err)
		}

		content, err := os.ReadFile(stagedPath)
		if err != nil {
			return fmt.Errorf("failed to read staged file %s: %w", stagedPath, err)
		}

		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("failed to overwrite target file %s: %w", targetPath, err)
		}
	}

	fmt.Fprintf(r.out, "[BACKUP] Full server restoration completed successfully from %s\n", filepath.Base(archivePath))
	return nil
}
