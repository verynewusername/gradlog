package handlers

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/config"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
	"github.com/gradlog/gradlog/internal/storage"
)

// ArtifactHandler handles upload, download, and management of run artifacts.
type ArtifactHandler struct {
	db             *database.DB
	storage        *storage.LocalStorage
	cfg            *config.Config
	projectHandler *ProjectHandler
}

// NewArtifactHandler creates a new ArtifactHandler.
func NewArtifactHandler(db *database.DB, s *storage.LocalStorage, cfg *config.Config, ph *ProjectHandler) *ArtifactHandler {
	return &ArtifactHandler{db: db, storage: s, cfg: cfg, projectHandler: ph}
}

// getProjectIDForRun returns the project ID for a given run ID.
func (h *ArtifactHandler) getProjectIDForRun(c *gin.Context, runID uuid.UUID) (uuid.UUID, error) {
	var projectID uuid.UUID
	err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT e.project_id FROM runs r
		 JOIN experiments e ON e.id = r.experiment_id
		 WHERE r.id = $1`,
		runID,
	).Scan(&projectID)
	return projectID, err
}

// getProjectIDForArtifact returns the project ID for a given artifact ID.
func (h *ArtifactHandler) getProjectIDForArtifact(c *gin.Context, artifactID uuid.UUID) (uuid.UUID, error) {
	var projectID uuid.UUID
	err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT e.project_id FROM artifacts a
		 JOIN runs r ON r.id = a.run_id
		 JOIN experiments e ON e.id = r.experiment_id
		 WHERE a.id = $1`,
		artifactID,
	).Scan(&projectID)
	return projectID, err
}

// ListArtifacts handles GET /runs/:runId/artifacts.
// Returns all artifacts associated with a run.
func (h *ArtifactHandler) ListArtifacts(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT id, run_id, path, file_name, file_size, content_type, checksum, storage_path, created_at
		 FROM artifacts WHERE run_id = $1 ORDER BY created_at DESC`,
		runID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list artifacts"})
		return
	}
	defer rows.Close()

	artifacts := make([]models.Artifact, 0)
	for rows.Next() {
		var a models.Artifact
		if err := rows.Scan(&a.ID, &a.RunID, &a.Path, &a.FileName, &a.FileSize,
			&a.ContentType, &a.Checksum, &a.StoragePath, &a.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan artifact"})
			return
		}
		artifacts = append(artifacts, a)
	}

	c.JSON(http.StatusOK, artifacts)
}

// SimpleUpload handles POST /runs/:runId/artifacts/upload.
// Accepts a multipart form upload with fields: path (string), file (binary).
// Maximum file size is enforced via config.MaxArtifactSize.
func (h *ArtifactHandler) SimpleUpload(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Enforce maximum upload size (0 means unlimited, use a safe 10GB cap).
	maxSize := h.cfg.ArtifactMaxFileSize
	if maxSize <= 0 {
		maxSize = 10 << 30 // 10 GiB
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required in form"})
		return
	}
	defer file.Close()

	artifactPath := c.PostForm("path")
	if artifactPath == "" {
		artifactPath = header.Filename
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	artifactID := uuid.New()
	storagePath := fmt.Sprintf("runs/%s/artifacts/%s/%s", runID, artifactID, header.Filename)

	checksum, err := h.storage.Store(storagePath, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store artifact"})
		return
	}

	fileSize, _ := h.storage.Size(storagePath)

	artifact := &models.Artifact{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`INSERT INTO artifacts (id, run_id, path, file_name, file_size, content_type, checksum, storage_path, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		 ON CONFLICT (run_id, path) DO UPDATE
		   SET file_name    = EXCLUDED.file_name,
		       file_size    = EXCLUDED.file_size,
		       content_type = EXCLUDED.content_type,
		       checksum     = EXCLUDED.checksum,
		       storage_path = EXCLUDED.storage_path
		 RETURNING id, run_id, path, file_name, file_size, content_type, checksum, storage_path, created_at`,
		artifactID, runID, artifactPath, header.Filename, fileSize,
		contentType, checksum, storagePath,
	).Scan(&artifact.ID, &artifact.RunID, &artifact.Path, &artifact.FileName,
		&artifact.FileSize, &artifact.ContentType, &artifact.Checksum,
		&artifact.StoragePath, &artifact.CreatedAt)
	if err != nil {
		h.storage.Delete(storagePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save artifact record"})
		return
	}

	c.JSON(http.StatusCreated, artifact)
}

// ArtifactUploadInit is the response body for InitUpload.
type ArtifactUploadInit struct {
	ArtifactID  uuid.UUID `json:"artifact_id"`
	TotalChunks int       `json:"total_chunks"`
	ChunkSize   int64     `json:"chunk_size"`
}

// InitUpload handles POST /runs/:runId/artifacts/init.
// Initiates a chunked upload by creating the artifact record and computing chunk info.
// Accepts JSON body: { path, file_name, file_size, content_type }.
func (h *ArtifactHandler) InitUpload(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Path        string `json:"path" binding:"required"`
		FileName    string `json:"file_name" binding:"required"`
		FileSize    int64  `json:"file_size" binding:"required,min=1"`
		ContentType string `json:"content_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ContentType == "" {
		req.ContentType = "application/octet-stream"
	}

	artifactID := uuid.New()
	storagePath := fmt.Sprintf("runs/%s/artifacts/%s/%s", runID, artifactID, req.FileName)

	// Create the artifact record without a checksum (to be filled after assembly).
	if _, err := h.db.Pool.Exec(
		c.Request.Context(),
		`INSERT INTO artifacts (id, run_id, path, file_name, file_size, content_type, storage_path, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		artifactID, runID, req.Path, req.FileName, req.FileSize, req.ContentType, storagePath,
	); err != nil {
		if isDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "artifact path already exists for this run"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create artifact record"})
		return
	}

	chunkSize := h.cfg.ArtifactChunkSize
	totalChunks := int(math.Ceil(float64(req.FileSize) / float64(chunkSize)))

	c.JSON(http.StatusCreated, ArtifactUploadInit{
		ArtifactID:  artifactID,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
	})
}

// UploadChunk handles POST /artifacts/:artifactId/chunks/:chunkNumber.
// Accepts the raw chunk data in the request body.
func (h *ArtifactHandler) UploadChunk(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact id"})
		return
	}

	projectID, err := h.getProjectIDForArtifact(c, artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	chunkNumber, err := strconv.Atoi(c.Param("chunkNumber"))
	if err != nil || chunkNumber < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk number"})
		return
	}

	chunkStoragePath := fmt.Sprintf("chunks/%s/%d", artifactID, chunkNumber)
	checksum, err := h.storage.StoreChunk(chunkStoragePath, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store chunk"})
		return
	}

	chunkSize, _ := h.storage.Size(chunkStoragePath)

	var chunkID uuid.UUID
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`INSERT INTO artifact_chunks (id, artifact_id, chunk_number, chunk_size, checksum, storage_path, uploaded_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (artifact_id, chunk_number) DO UPDATE
		   SET chunk_size   = EXCLUDED.chunk_size,
		       checksum     = EXCLUDED.checksum,
		       storage_path = EXCLUDED.storage_path,
		       uploaded_at  = EXCLUDED.uploaded_at
		 RETURNING id`,
		artifactID, chunkNumber, chunkSize, checksum, chunkStoragePath,
	).Scan(&chunkID)
	if err != nil {
		h.storage.Delete(chunkStoragePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save chunk record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"chunk_id":     chunkID,
		"chunk_number": chunkNumber,
		"checksum":     checksum,
	})
}

// CompleteUpload handles POST /artifacts/:artifactId/complete.
// Verifies all chunks are present, assembles them into the final file, and updates the artifact.
func (h *ArtifactHandler) CompleteUpload(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact id"})
		return
	}

	projectID, err := h.getProjectIDForArtifact(c, artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Fetch the artifact's expected storage path.
	var storagePath string
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT storage_path FROM artifacts WHERE id = $1`, artifactID,
	).Scan(&storagePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	// Retrieve all chunks ordered by chunk_number.
	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT chunk_number, storage_path FROM artifact_chunks
		 WHERE artifact_id = $1 ORDER BY chunk_number ASC`,
		artifactID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch chunks"})
		return
	}

	type chunkInfo struct {
		number      int
		storagePath string
	}
	chunks := make([]chunkInfo, 0)
	for rows.Next() {
		var ci chunkInfo
		if err := rows.Scan(&ci.number, &ci.storagePath); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan chunk"})
			return
		}
		chunks = append(chunks, ci)
	}
	rows.Close()

	// Verify chunk sequence has no gaps.
	for i, chunk := range chunks {
		if chunk.number != i {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("missing chunk %d, found chunk %d", i, chunk.number),
			})
			return
		}
	}

	chunkPaths := make([]string, len(chunks))
	for i, chunk := range chunks {
		chunkPaths[i] = chunk.storagePath
	}

	checksum, totalSize, err := h.storage.AssembleChunks(chunkPaths, storagePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assemble chunks"})
		return
	}

	// Update artifact with final checksum and size; delete chunk records.
	artifact := &models.Artifact{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`UPDATE artifacts SET checksum = $2, file_size = $3
		 WHERE id = $1
		 RETURNING id, run_id, path, file_name, file_size, content_type, checksum, storage_path, created_at`,
		artifactID, checksum, totalSize,
	).Scan(&artifact.ID, &artifact.RunID, &artifact.Path, &artifact.FileName,
		&artifact.FileSize, &artifact.ContentType, &artifact.Checksum,
		&artifact.StoragePath, &artifact.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update artifact"})
		return
	}

	// Remove chunk DB records (storage files already deleted by AssembleChunks).
	h.db.Pool.Exec(c.Request.Context(),
		`DELETE FROM artifact_chunks WHERE artifact_id = $1`, artifactID)

	c.JSON(http.StatusOK, artifact)
}

// ArtifactDownloadInfo is the response body for GetDownloadInfo.
type ArtifactDownloadInfo struct {
	models.Artifact
	TotalChunks int   `json:"total_chunks"`
	ChunkSize   int64 `json:"chunk_size"`
}

// GetDownloadInfo handles GET /artifacts/:artifactId/download-info.
// Returns metadata needed to perform a chunked download.
func (h *ArtifactHandler) GetDownloadInfo(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact id"})
		return
	}

	projectID, err := h.getProjectIDForArtifact(c, artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	artifact := &models.Artifact{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT id, run_id, path, file_name, file_size, content_type, checksum, storage_path, created_at
		 FROM artifacts WHERE id = $1`,
		artifactID,
	).Scan(&artifact.ID, &artifact.RunID, &artifact.Path, &artifact.FileName,
		&artifact.FileSize, &artifact.ContentType, &artifact.Checksum,
		&artifact.StoragePath, &artifact.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	chunkSize := h.cfg.ArtifactChunkSize
	totalChunks := int(math.Ceil(float64(artifact.FileSize) / float64(chunkSize)))

	c.JSON(http.StatusOK, ArtifactDownloadInfo{
		Artifact:    *artifact,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
	})
}

// DownloadChunk handles GET /artifacts/:artifactId/chunks/:chunkNumber.
// Streams a specific byte range of the artifact file to the client.
func (h *ArtifactHandler) DownloadChunk(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact id"})
		return
	}

	projectID, err := h.getProjectIDForArtifact(c, artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	chunkNumber, err := strconv.ParseInt(c.Param("chunkNumber"), 10, 64)
	if err != nil || chunkNumber < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chunk number"})
		return
	}

	var storagePath string
	var fileSize int64
	var fileName string
	var contentType string
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT storage_path, file_size, file_name, content_type FROM artifacts WHERE id = $1`,
		artifactID,
	).Scan(&storagePath, &fileSize, &fileName, &contentType); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	chunkSize := h.cfg.ArtifactChunkSize
	start := chunkNumber * chunkSize
	if start >= fileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chunk number out of range"})
		return
	}

	length := chunkSize
	if start+length > fileSize {
		length = fileSize - start
	}

	reader, err := h.storage.RetrieveRange(storagePath, start, length)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read artifact"})
		return
	}
	defer reader.Close()

	c.Header("Content-Length", strconv.FormatInt(length, 10))
	c.Header("Content-Type", contentType)
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}

// SimpleDownload handles GET /artifacts/:artifactId/download.
// Streams the full artifact file to the client with Content-Disposition header.
func (h *ArtifactHandler) SimpleDownload(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact id"})
		return
	}

	projectID, err := h.getProjectIDForArtifact(c, artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	artifact := &models.Artifact{}
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT id, run_id, path, file_name, file_size, content_type, checksum, storage_path, created_at
		 FROM artifacts WHERE id = $1`,
		artifactID,
	).Scan(&artifact.ID, &artifact.RunID, &artifact.Path, &artifact.FileName,
		&artifact.FileSize, &artifact.ContentType, &artifact.Checksum,
		&artifact.StoragePath, &artifact.CreatedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	reader, err := h.storage.Retrieve(artifact.StoragePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read artifact"})
		return
	}
	defer reader.Close()

	contentType := "application/octet-stream"
	if artifact.ContentType != nil && *artifact.ContentType != "" {
		contentType = *artifact.ContentType
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, artifact.FileName))
	c.Header("Content-Length", strconv.FormatInt(artifact.FileSize, 10))
	c.Header("Content-Type", contentType)
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}

// DeleteArtifact handles DELETE /artifacts/:artifactId.
// Removes the artifact record and its stored file.
func (h *ArtifactHandler) DeleteArtifact(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact id"})
		return
	}

	projectID, err := h.getProjectIDForArtifact(c, artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var storagePath string
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`DELETE FROM artifacts WHERE id = $1 RETURNING storage_path`, artifactID,
	).Scan(&storagePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}

	// Best-effort file deletion; don't fail the request if the file is missing.
	h.storage.Delete(storagePath)

	c.JSON(http.StatusNoContent, nil)
}
