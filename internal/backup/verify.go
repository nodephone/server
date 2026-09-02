package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Verifier validates .npb backup archive integrity, SHA-256 checksum match against embedded manifest,
// and corruption detection prior to restoration.
type Verifier struct{}

// NewVerifier creates a new Verifier instance.
func NewVerifier() *Verifier {
	return &Verifier{}
}

// VerifyArchive validates that an .npb backup file is uncorrupted and readable.
func (v *Verifier) VerifyArchive(archivePath string, password string) (*BackupMetadata, error) {
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup file %s: %w", archivePath, err)
	}

	rawZipBytes, err := DecryptData(archiveBytes, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt archive (incorrect password or corrupted payload): %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(rawZipBytes), int64(len(rawZipBytes)))
	if err != nil {
		return nil, fmt.Errorf("corrupted archive: invalid zip structure: %w", err)
	}

	var manifest map[string]string
	manifestFound := false

	for _, file := range zipReader.File {
		if file.Name == "manifest.json" {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open manifest.json: %w", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read manifest.json: %w", err)
			}
			if err := json.Unmarshal(content, &manifest); err != nil {
				return nil, fmt.Errorf("invalid manifest.json structure: %w", err)
			}
			manifestFound = true
			break
		}
	}

	if !manifestFound {
		return nil, fmt.Errorf("corrupted archive: missing manifest.json")
	}

	// Validate files against manifest checksums
	for _, file := range zipReader.File {
		if file.Name == "manifest.json" {
			continue
		}
		expectedHash, ok := manifest[file.Name]
		if !ok {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open entry %s: %w", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read entry %s: %w", file.Name, err)
		}

		actualHash := sha256.Sum256(content)
		actualHashStr := hex.EncodeToString(actualHash[:])
		if actualHashStr != expectedHash {
			return nil, fmt.Errorf("integrity check failed for file %s: expected SHA256 %s, got %s", file.Name, expectedHash, actualHashStr)
		}
	}

	archiveHash := sha256.Sum256(archiveBytes)
	baseName := filepath.Base(archivePath)

	return &BackupMetadata{
		ID:            strings.TrimSuffix(baseName, ".npb"),
		Filename:      baseName,
		SizeBytes:     int64(len(archiveBytes)),
		SHA256Hash:    hex.EncodeToString(archiveHash[:]),
		Encrypted:     password != "",
		IncludesDB:    manifest["main.db"] != "",
		IncludesFiles: len(manifest) > 1,
		IncludesFuncs: true,
		IncludesConfig: manifest["config.json"] != "",
		CreatedAt:     time.Now().UTC(),
	}, nil
}
