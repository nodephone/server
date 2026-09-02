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

// Archiver handles bundling files into .npb (NodePhone Backup) Zip archives,
// SHA-256 checksum generation, manifest embedding, and archive extraction.
type Archiver struct{}

// NewArchiver creates a new Archiver instance.
func NewArchiver() *Archiver {
	return &Archiver{}
}

// CreateArchive packs source files into a compressed .npb archive with optional AES-256 encryption.
func (a *Archiver) CreateArchive(destFilePath string, sourceFiles map[string]string, password string) (*BackupMetadata, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	manifest := make(map[string]string)

	for relPath, absPath := range sourceFiles {
		cleanRel := strings.TrimPrefix(filepath.ToSlash(relPath), "/")
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to stat file %s: %w", absPath, err)
		}

		if info.IsDir() {
			continue
		}

		fileData, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", absPath, err)
		}

		hash := sha256.Sum256(fileData)
		hashStr := hex.EncodeToString(hash[:])
		manifest[cleanRel] = hashStr

		fWriter, err := zipWriter.Create(cleanRel)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry %s: %w", cleanRel, err)
		}

		if _, err := fWriter.Write(fileData); err != nil {
			return nil, fmt.Errorf("failed to write zip entry content %s: %w", cleanRel, err)
		}
	}

	// Write manifest.json inside archive
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	mfWriter, err := zipWriter.Create("manifest.json")
	if err == nil {
		_, _ = mfWriter.Write(manifestBytes)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip archive: %w", err)
	}

	rawZipBytes := buf.Bytes()
	finalBytes, err := EncryptData(rawZipBytes, password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt backup payload: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination backup directory: %w", err)
	}

	if err := os.WriteFile(destFilePath, finalBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write backup archive file: %w", err)
	}

	archiveHash := sha256.Sum256(finalBytes)
	baseName := filepath.Base(destFilePath)

	return &BackupMetadata{
		ID:            strings.TrimSuffix(baseName, ".npb"),
		Filename:      baseName,
		SizeBytes:     int64(len(finalBytes)),
		SHA256Hash:    hex.EncodeToString(archiveHash[:]),
		Encrypted:     password != "",
		IncludesDB:    manifest["main.db"] != "",
		IncludesFiles: len(manifest) > 1,
		IncludesFuncs: true,
		IncludesConfig: manifest["config.json"] != "",
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// ExtractArchive decrypts and unpacks a .npb backup archive into destDir.
func (a *Archiver) ExtractArchive(archiveFilePath string, destDir string, password string) (map[string]string, error) {
	archiveBytes, err := os.ReadFile(archiveFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive file: %w", err)
	}

	rawZipBytes, err := DecryptData(archiveBytes, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt archive: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(rawZipBytes), int64(len(rawZipBytes)))
	if err != nil {
		return nil, fmt.Errorf("invalid or corrupted zip archive: %w", err)
	}

	manifest := make(map[string]string)

	for _, file := range zipReader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open zip entry %s: %w", file.Name, err)
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read zip entry %s: %w", file.Name, err)
		}

		if file.Name == "manifest.json" {
			_ = json.Unmarshal(content, &manifest)
			continue
		}

		outPath := filepath.Join(destDir, file.Name)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create dir for %s: %w", outPath, err)
		}

		if err := os.WriteFile(outPath, content, 0644); err != nil {
			return nil, fmt.Errorf("failed to write extracted file %s: %w", outPath, err)
		}
	}

	return manifest, nil
}
