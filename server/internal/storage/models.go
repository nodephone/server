// Package storage provides disk-backed file storage, metadata tracking in SQLite,
// boundary safety against path traversal attacks, streaming upload/download handling, and signed URL generation.
package storage

import "time"

// Bucket represents a logical storage container in NodePhone.
type Bucket struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Public    bool      `json:"public"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Object represents a file stored inside a bucket with associated metadata.
type Object struct {
	ID          string    `json:"id"`
	BucketID    string    `json:"bucket_id"`
	BucketName  string    `json:"bucket_name,omitempty"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	MimeType    string    `json:"mime_type"`
	StoragePath string    `json:"-"`
	UploadedBy  string    `json:"uploaded_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateBucketRequest holds the payload to create a storage bucket.
type CreateBucketRequest struct {
	Name   string `json:"name"`
	Public bool   `json:"public"`
}

// SignURLRequest holds the payload to generate a signed URL token.
type SignURLRequest struct {
	ExpiresIn int64 `json:"expires_in"` // Expiration time in seconds (default 3600)
}

// SignedURLResponse returns the signed URL token details.
type SignedURLResponse struct {
	Token     string    `json:"token"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
