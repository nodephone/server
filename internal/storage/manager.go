package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nodephone/server/internal/database"
)

var (
	ErrBucketExists       = errors.New("bucket already exists")
	ErrBucketNotFound     = errors.New("bucket not found")
	ErrObjectNotFound     = errors.New("object not found")
	ErrInvalidPath        = errors.New("invalid storage path or path traversal detected")
	ErrInvalidSignedURL   = errors.New("invalid or expired signed URL token")
	ErrUnauthorizedAccess = errors.New("access denied to private object")
	ErrInvalidInput       = errors.New("invalid bucket or object parameters")
)

// StorageManager manages file system persistence, SQLite metadata, boundary security, and signed URLs.
type StorageManager struct {
	db        *database.DB
	baseDir   string
	jwtSecret []byte
	out       io.Writer
}

// NewStorageManager creates a new StorageManager instance.
func NewStorageManager(db *database.DB, baseDir string, jwtSecret string, out io.Writer) (*StorageManager, error) {
	if out == nil {
		out = io.Discard
	}
	if baseDir == "" {
		baseDir = "nodephone-data/storage"
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for storage base directory: %w", err)
	}

	if err := os.MkdirAll(absBaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root storage directory %q: %w", absBaseDir, err)
	}

	return &StorageManager{
		db:        db,
		baseDir:   absBaseDir,
		jwtSecret: []byte(jwtSecret),
		out:       out,
	}, nil
}

// BaseDir returns the root disk storage path.
func (sm *StorageManager) BaseDir() string {
	return sm.baseDir
}

// SanitizeBucketName ensures bucket names contain only alphanumeric characters, hyphens, and underscores.
func SanitizeBucketName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var sb strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// ResolveSafePath validates and constructs a disk filepath within the storage base directory.
// Returns ErrInvalidPath if path traversal or directory breakout is attempted.
func (sm *StorageManager) ResolveSafePath(bucketName, objectName string) (string, error) {
	cleanBucket := SanitizeBucketName(bucketName)
	if cleanBucket == "" {
		return "", fmt.Errorf("%w: invalid bucket name", ErrInvalidPath)
	}

	cleanObject := filepath.Clean(objectName)
	if cleanObject == "." || cleanObject == "" || strings.HasPrefix(cleanObject, "..") || strings.HasPrefix(cleanObject, "/") || strings.HasPrefix(cleanObject, "\\") {
		return "", fmt.Errorf("%w: path traversal attempt detected in object name %q", ErrInvalidPath, objectName)
	}

	targetPath := filepath.Join(sm.baseDir, cleanBucket, cleanObject)
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("%w: unable to resolve target path", ErrInvalidPath)
	}

	// Boundary check to ensure absTarget remains inside sm.baseDir
	if !strings.HasPrefix(absTarget, sm.baseDir+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: target path outside storage directory boundary", ErrInvalidPath)
	}

	return absTarget, nil
}

// CreateBucket creates a new public or private storage bucket.
func (sm *StorageManager) CreateBucket(ctx context.Context, name string, public bool, createdBy string) (*Bucket, error) {
	cleanName := SanitizeBucketName(name)
	if cleanName == "" {
		return nil, fmt.Errorf("%w: bucket name must be alphanumeric with hyphens/underscores", ErrInvalidInput)
	}

	var count int
	err := sm.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM buckets WHERE name = ?", cleanName).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing bucket: %w", err)
	}
	if count > 0 {
		return nil, ErrBucketExists
	}

	bucketID := uuid.New().String()
	now := time.Now().UTC()

	bucketDir := filepath.Join(sm.baseDir, cleanName)
	if err := os.MkdirAll(bucketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bucket directory on disk: %w", err)
	}

	query := `INSERT INTO buckets (id, name, public, created_by, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err = sm.db.ExecContext(ctx, query, bucketID, cleanName, public, createdBy, now)
	if err != nil {
		_ = os.RemoveAll(bucketDir)
		return nil, fmt.Errorf("failed to insert bucket metadata: %w", err)
	}

	fmt.Fprintf(sm.out, "[STORAGE] Created bucket %s (public=%v)\n", cleanName, public)

	return &Bucket{
		ID:        bucketID,
		Name:      cleanName,
		Public:    public,
		CreatedBy: createdBy,
		CreatedAt: now,
	}, nil
}

// GetBucket fetches bucket metadata by name.
func (sm *StorageManager) GetBucket(ctx context.Context, name string) (*Bucket, error) {
	cleanName := SanitizeBucketName(name)
	var b Bucket
	query := `SELECT id, name, public, created_by, created_at FROM buckets WHERE name = ?`
	err := sm.db.QueryRowContext(ctx, query, cleanName).Scan(
		&b.ID, &b.Name, &b.Public, &b.CreatedBy, &b.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBucketNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch bucket: %w", err)
	}
	return &b, nil
}

// ListBuckets returns all registered buckets.
func (sm *StorageManager) ListBuckets(ctx context.Context) ([]*Bucket, error) {
	query := `SELECT id, name, public, created_by, created_at FROM buckets ORDER BY created_at DESC`
	rows, err := sm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query buckets: %w", err)
	}
	defer rows.Close()

	var buckets []*Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.ID, &b.Name, &b.Public, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan bucket row: %w", err)
		}
		buckets = append(buckets, &b)
	}
	return buckets, nil
}

// DeleteBucket deletes a bucket and all its contained objects from disk and database.
func (sm *StorageManager) DeleteBucket(ctx context.Context, name string) error {
	bucket, err := sm.GetBucket(ctx, name)
	if err != nil {
		return err
	}

	// Delete bucket row from database (CASCADE deletes object records)
	_, err = sm.db.ExecContext(ctx, "DELETE FROM buckets WHERE id = ?", bucket.ID)
	if err != nil {
		return fmt.Errorf("failed to delete bucket record: %w", err)
	}

	bucketDir := filepath.Join(sm.baseDir, bucket.Name)
	_ = os.RemoveAll(bucketDir)

	fmt.Fprintf(sm.out, "[STORAGE] Deleted bucket %s\n", bucket.Name)
	return nil
}

// PutObject streams data directly from an io.Reader into a file on disk without buffering in memory,
// and saves object metadata in SQLite.
func (sm *StorageManager) PutObject(ctx context.Context, bucketName, objectName string, body io.Reader, size int64, mimeType string, uploadedBy string) (*Object, error) {
	bucket, err := sm.GetBucket(ctx, bucketName)
	if err != nil {
		return nil, err
	}

	targetPath, err := sm.ResolveSafePath(bucket.Name, objectName)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory structure for object: %w", err)
	}

	outFile, err := os.Create(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file on disk: %w", err)
	}

	written, err := io.Copy(outFile, body)
	_ = outFile.Close()
	if err != nil {
		_ = os.Remove(targetPath)
		return nil, fmt.Errorf("failed to stream file payload to disk: %w", err)
	}

	actualSize := written
	if size > 0 && size != actualSize {
		actualSize = written
	}

	// Auto-detect MIME type if empty
	if mimeType == "" || mimeType == "application/octet-stream" {
		f, openErr := os.Open(targetPath)
		if openErr == nil {
			buf := make([]byte, 512)
			n, _ := f.Read(buf)
			_ = f.Close()
			if n > 0 {
				mimeType = http.DetectContentType(buf[:n])
			}
		}
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	now := time.Now().UTC()

	// Check if object already exists in bucket for upsert logic
	var existingID string
	err = sm.db.QueryRowContext(ctx, "SELECT id FROM objects WHERE bucket_id = ? AND name = ?", bucket.ID, objectName).Scan(&existingID)

	var objID string
	if errors.Is(err, sql.ErrNoRows) {
		objID = uuid.New().String()
		query := `INSERT INTO objects (id, bucket_id, name, size, mime_type, storage_path, uploaded_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = sm.db.ExecContext(ctx, query, objID, bucket.ID, objectName, actualSize, mimeType, targetPath, uploadedBy, now, now)
		if err != nil {
			_ = os.Remove(targetPath)
			return nil, fmt.Errorf("failed to insert object metadata: %w", err)
		}
	} else if err != nil {
		_ = os.Remove(targetPath)
		return nil, fmt.Errorf("failed to query existing object: %w", err)
	} else {
		objID = existingID
		query := `UPDATE objects SET size = ?, mime_type = ?, storage_path = ?, updated_at = ? WHERE id = ?`
		_, err = sm.db.ExecContext(ctx, query, actualSize, mimeType, targetPath, now, objID)
		if err != nil {
			return nil, fmt.Errorf("failed to update object metadata: %w", err)
		}
	}

	fmt.Fprintf(sm.out, "[STORAGE] Uploaded object %s/%s (%d bytes, %s)\n", bucket.Name, objectName, actualSize, mimeType)

	return &Object{
		ID:          objID,
		BucketID:    bucket.ID,
		BucketName:  bucket.Name,
		Name:        objectName,
		Size:        actualSize,
		MimeType:    mimeType,
		StoragePath: targetPath,
		UploadedBy:  uploadedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetObjectStream returns object metadata and an open file stream.
func (sm *StorageManager) GetObjectStream(ctx context.Context, bucketName, objectName string) (*Object, io.ReadCloser, error) {
	query := `
SELECT o.id, o.bucket_id, b.name, o.name, o.size, o.mime_type, o.storage_path, o.uploaded_by, o.created_at, o.updated_at
FROM objects o
JOIN buckets b ON o.bucket_id = b.id
WHERE b.name = ? AND o.name = ?`

	cleanBucket := SanitizeBucketName(bucketName)
	var obj Object
	err := sm.db.QueryRowContext(ctx, query, cleanBucket, objectName).Scan(
		&obj.ID, &obj.BucketID, &obj.BucketName, &obj.Name, &obj.Size, &obj.MimeType, &obj.StoragePath, &obj.UploadedBy, &obj.CreatedAt, &obj.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrObjectNotFound
	} else if err != nil {
		return nil, nil, fmt.Errorf("failed to query object metadata: %w", err)
	}

	file, err := os.Open(obj.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open storage file: %w", err)
	}

	return &obj, file, nil
}

// DeleteObject removes an object record from the database and deletes the physical file.
func (sm *StorageManager) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	cleanBucket := SanitizeBucketName(bucketName)
	var objID, storagePath string

	query := `
SELECT o.id, o.storage_path
FROM objects o
JOIN buckets b ON o.bucket_id = b.id
WHERE b.name = ? AND o.name = ?`

	err := sm.db.QueryRowContext(ctx, query, cleanBucket, objectName).Scan(&objID, &storagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrObjectNotFound
	} else if err != nil {
		return fmt.Errorf("failed to find object for deletion: %w", err)
	}

	_, err = sm.db.ExecContext(ctx, "DELETE FROM objects WHERE id = ?", objID)
	if err != nil {
		return fmt.Errorf("failed to delete object metadata: %w", err)
	}

	_ = os.Remove(storagePath)
	fmt.Fprintf(sm.out, "[STORAGE] Deleted object %s/%s\n", cleanBucket, objectName)
	return nil
}

// ListObjects returns all objects inside a bucket.
func (sm *StorageManager) ListObjects(ctx context.Context, bucketName string) ([]*Object, error) {
	bucket, err := sm.GetBucket(ctx, bucketName)
	if err != nil {
		return nil, err
	}

	query := `
SELECT id, bucket_id, name, size, mime_type, storage_path, uploaded_by, created_at, updated_at
FROM objects WHERE bucket_id = ? ORDER BY created_at DESC`

	rows, err := sm.db.QueryContext(ctx, query, bucket.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	defer rows.Close()

	var objects []*Object
	for rows.Next() {
		var o Object
		o.BucketName = bucket.Name
		if err := rows.Scan(&o.ID, &o.BucketID, &o.Name, &o.Size, &o.MimeType, &o.StoragePath, &o.UploadedBy, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan object row: %w", err)
		}
		objects = append(objects, &o)
	}
	return objects, nil
}

// GenerateSignedURL generates an HMAC-SHA256 signed access token for temporary private file access.
func (sm *StorageManager) GenerateSignedURL(bucketName, objectName string, ttl time.Duration) (*SignedURLResponse, error) {
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)
	expiresUnix := expiresAt.Unix()

	cleanBucket := SanitizeBucketName(bucketName)
	payload := fmt.Sprintf("%s:%s:%d", cleanBucket, objectName, expiresUnix)

	h := hmac.New(sha256.New, sm.jwtSecret)
	h.Write([]byte(payload))
	sig := hex.EncodeToString(h.Sum(nil))

	token := fmt.Sprintf("%d.%s", expiresUnix, sig)
	url := fmt.Sprintf("/api/storage/buckets/%s/objects/%s?token=%s", cleanBucket, objectName, token)

	return &SignedURLResponse{
		Token:     token,
		URL:       url,
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateSignedURL verifies the HMAC signature and expiration timestamp of a signed URL token.
func (sm *StorageManager) ValidateSignedURL(token, bucketName, objectName string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	expiresUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}

	if time.Now().UTC().Unix() > expiresUnix {
		return false // Token expired
	}

	cleanBucket := SanitizeBucketName(bucketName)
	payload := fmt.Sprintf("%s:%s:%d", cleanBucket, objectName, expiresUnix)

	h := hmac.New(sha256.New, sm.jwtSecret)
	h.Write([]byte(payload))
	expectedSig := hex.EncodeToString(h.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expectedSig)) == 1
}
