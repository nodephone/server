package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/nodephone/server/internal/auth"
)

// StorageHandler exposes HTTP REST API handlers for storage buckets and objects.
type StorageHandler struct {
	manager     *StorageManager
	authService *auth.AuthService
}

// NewStorageHandler creates a new StorageHandler instance.
func NewStorageHandler(manager *StorageManager, authService *auth.AuthService) *StorageHandler {
	return &StorageHandler{
		manager:     manager,
		authService: authService,
	}
}

// Manager returns the underlying StorageManager.
func (h *StorageHandler) Manager() *StorageManager {
	return h.manager
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	writeJSONResponse(w, statusCode, map[string]string{"error": message})
}

// CreateBucket handles POST /api/storage/buckets requests.
func (h *StorageHandler) CreateBucket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req CreateBucketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	bucket, err := h.manager.CreateBucket(r.Context(), req.Name, req.Public, user.ID)
	if err != nil {
		if errors.Is(err, ErrBucketExists) {
			writeErrorResponse(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidInput) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create bucket")
		return
	}

	writeJSONResponse(w, http.StatusCreated, bucket)
}

// ListBuckets handles GET /api/storage/buckets requests.
func (h *StorageHandler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	buckets, err := h.manager.ListBuckets(r.Context())
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list buckets")
		return
	}

	if buckets == nil {
		buckets = []*Bucket{}
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"buckets": buckets,
	})
}

// RouteBucketObject handles routing for /api/storage/buckets/{bucket}/objects and subpaths.
func (h *StorageHandler) RouteBucketObject(w http.ResponseWriter, r *http.Request) {
	// Path pattern expected: /api/storage/buckets/{bucket_name}/objects/...
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/storage/buckets/")
	parts := strings.Split(trimmed, "/")

	if len(parts) == 0 || parts[0] == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Missing bucket name in path")
		return
	}

	bucketName := parts[0]

	// Handle /api/storage/buckets/{bucket_name} (Delete bucket)
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			h.DeleteBucket(w, r, bucketName)
			return
		}
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if parts[1] != "objects" {
		writeErrorResponse(w, http.StatusNotFound, "Endpoint not found")
		return
	}

	// Handle /api/storage/buckets/{bucket_name}/objects (List / Upload)
	if len(parts) == 2 {
		if r.Method == http.MethodGet {
			h.ListObjects(w, r, bucketName)
			return
		}
		if r.Method == http.MethodPost {
			h.UploadObject(w, r, bucketName)
			return
		}
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Sub-path after /objects/
	objectName := strings.Join(parts[2:], "/")

	// Check if this is a signing request: /api/storage/buckets/{bucket}/objects/{name}/sign
	if strings.HasSuffix(objectName, "/sign") && r.Method == http.MethodPost {
		cleanObjName := strings.TrimSuffix(objectName, "/sign")
		h.SignURL(w, r, bucketName, cleanObjName)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.DownloadObject(w, r, bucketName, objectName)
	case http.MethodDelete:
		h.DeleteObject(w, r, bucketName, objectName)
	default:
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// DeleteBucket handles DELETE /api/storage/buckets/{name}.
func (h *StorageHandler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucketName string) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.manager.DeleteBucket(r.Context(), bucketName); err != nil {
		if errors.Is(err, ErrBucketNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete bucket")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Bucket %q deleted successfully", bucketName),
	})
}

// UploadObject handles POST /api/storage/buckets/{bucket}/objects.
func (h *StorageHandler) UploadObject(w http.ResponseWriter, r *http.Request, bucketName string) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var reader io.Reader
	var objectName string
	var size int64
	var mimeType string

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Multipart file upload
		err := r.ParseMultipartForm(32 << 20) // 32MB max memory for form parsing
		if err != nil {
			writeErrorResponse(w, http.StatusBadRequest, "Failed to parse multipart form data")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeErrorResponse(w, http.StatusBadRequest, "Missing 'file' field in multipart form")
			return
		}
		defer file.Close()

		reader = file
		size = header.Size
		mimeType = header.Header.Get("Content-Type")

		// Determine object name (query param override or form header filename)
		objectName = r.URL.Query().Get("name")
		if objectName == "" {
			objectName = header.Filename
		}
	} else {
		// Raw payload upload
		objectName = r.URL.Query().Get("name")
		if objectName == "" {
			writeErrorResponse(w, http.StatusBadRequest, "Query parameter 'name' is required for binary uploads")
			return
		}
		reader = r.Body
		size = r.ContentLength
		mimeType = contentType
	}

	objectName = path.Clean(objectName)
	obj, err := h.manager.PutObject(r.Context(), bucketName, objectName, reader, size, mimeType, user.ID)
	if err != nil {
		if errors.Is(err, ErrBucketNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrInvalidPath) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to upload object")
		return
	}

	writeJSONResponse(w, http.StatusCreated, obj)
}

// DownloadObject handles GET /api/storage/buckets/{bucket}/objects/{name}.
func (h *StorageHandler) DownloadObject(w http.ResponseWriter, r *http.Request, bucketName, objectName string) {
	bucket, err := h.manager.GetBucket(r.Context(), bucketName)
	if err != nil {
		if errors.Is(err, ErrBucketNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to fetch bucket metadata")
		return
	}

	// Access control: Public bucket OR valid Signed URL token OR valid Auth user
	isAuthorized := false

	if bucket.Public {
		isAuthorized = true
	} else {
		// Check for signed URL token parameter
		token := r.URL.Query().Get("token")
		if token != "" && h.manager.ValidateSignedURL(token, bucketName, objectName) {
			isAuthorized = true
		} else {
			// Check for authenticated user in request context
			user, ok := auth.UserFromContext(r.Context())
			if ok && user != nil {
				isAuthorized = true
			}
		}
	}

	if !isAuthorized {
		writeErrorResponse(w, http.StatusUnauthorized, "Access denied to private object")
		return
	}

	obj, stream, err := h.manager.GetObjectStream(r.Context(), bucketName, objectName)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve object stream")
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", obj.MimeType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
	w.WriteHeader(http.StatusOK)

	// Stream object payload efficiently to client
	_, _ = io.Copy(w, stream)
}

// DeleteObject handles DELETE /api/storage/buckets/{bucket}/objects/{name}.
func (h *StorageHandler) DeleteObject(w http.ResponseWriter, r *http.Request, bucketName, objectName string) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.manager.DeleteObject(r.Context(), bucketName, objectName); err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete object")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Object %q deleted successfully", objectName),
	})
}

// ListObjects handles GET /api/storage/buckets/{bucket}/objects.
func (h *StorageHandler) ListObjects(w http.ResponseWriter, r *http.Request, bucketName string) {
	bucket, err := h.manager.GetBucket(r.Context(), bucketName)
	if err != nil {
		if errors.Is(err, ErrBucketNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to fetch bucket")
		return
	}

	if !bucket.Public {
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user == nil {
			writeErrorResponse(w, http.StatusUnauthorized, "Access denied to private bucket listing")
			return
		}
	}

	objects, err := h.manager.ListObjects(r.Context(), bucketName)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list objects")
		return
	}

	if objects == nil {
		objects = []*Object{}
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"objects": objects,
	})
}

// SignURL handles POST /api/storage/buckets/{bucket}/objects/{name}/sign.
func (h *StorageHandler) SignURL(w http.ResponseWriter, r *http.Request, bucketName, objectName string) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeErrorResponse(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req SignURLRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 1 * time.Hour
	}

	resp, err := h.manager.GenerateSignedURL(bucketName, objectName, expiresIn)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate signed URL")
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}
