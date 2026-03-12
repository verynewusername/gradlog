// Package models defines the data structures used throughout Gradlog.
// These structs map to database tables and are used for JSON serialization.
package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered user in the system.
// Users authenticate via Google OAuth and can create projects/experiments.
type User struct {
	ID         uuid.UUID `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	PictureURL *string   `json:"picture_url,omitempty"`
	GoogleID   *string   `json:"-"` // Never expose in API responses.
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// APIKey represents a machine-to-machine authentication token.
// The actual key is only returned once at creation time.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"` // First 8 chars for identification.
	KeyHash    string     `json:"-"`          // Never expose in API responses.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// APIKeyWithSecret is returned only once when creating a new API key.
type APIKeyWithSecret struct {
	APIKey
	Key string `json:"key"` // The full key, shown only at creation.
}

// Project is a top-level container for experiments.
// Projects support multi-user collaboration with different roles.
type Project struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	OwnerID     uuid.UUID `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MemberRole constants define the available project roles.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// ProjectMember represents a user's membership in a project.
type ProjectMember struct {
	ProjectID uuid.UUID `json:"project_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"` // owner, admin, member, viewer
	CreatedAt time.Time `json:"created_at"`
}

// ProjectMemberWithUser extends ProjectMember with the user's display info.
type ProjectMemberWithUser struct {
	ProjectMember
	UserEmail      string  `json:"user_email"`
	UserName       string  `json:"user_name"`
	UserPictureURL *string `json:"user_picture_url,omitempty"`
}

// Experiment groups related runs within a project.
type Experiment struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RunStatus constants define the available run statuses.
const (
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusKilled    = "killed"
)

// Run represents a single ML training or evaluation run.
type Run struct {
	ID           uuid.UUID              `json:"id"`
	ExperimentID uuid.UUID              `json:"experiment_id"`
	Name         *string                `json:"name,omitempty"`
	Status       string                 `json:"status"` // running, completed, failed, killed
	Params       map[string]interface{} `json:"params"`
	Tags         map[string]interface{} `json:"tags"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      *time.Time             `json:"end_time,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Metric represents a single metric value logged during a run.
// Multiple metrics can share the same key at different steps.
type Metric struct {
	ID        uuid.UUID `json:"id"`
	RunID     uuid.UUID `json:"run_id"`
	Key       string    `json:"key"`
	Value     float64   `json:"value"`
	Step      int64     `json:"step"`
	Timestamp time.Time `json:"timestamp"`
}

// MetricHistory contains all logged values for a single metric key.
type MetricHistory struct {
	Key    string   `json:"key"`
	Values []Metric `json:"values"`
}

// Artifact represents a file associated with a run.
type Artifact struct {
	ID          uuid.UUID `json:"id"`
	RunID       uuid.UUID `json:"run_id"`
	Path        string    `json:"path"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	ContentType *string   `json:"content_type,omitempty"`
	Checksum    *string   `json:"checksum,omitempty"`
	StoragePath string    `json:"-"` // Internal storage path, never exposed.
	CreatedAt   time.Time `json:"created_at"`
}

// ArtifactChunk represents one chunk of an in-progress chunked upload.
type ArtifactChunk struct {
	ID          uuid.UUID `json:"id"`
	ArtifactID  uuid.UUID `json:"artifact_id"`
	ChunkNumber int       `json:"chunk_number"`
	ChunkSize   int64     `json:"chunk_size"`
	Checksum    string    `json:"checksum"`
	StoragePath string    `json:"-"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// ArtifactUploadInit contains the chunking parameters returned when starting an upload.
type ArtifactUploadInit struct {
	ArtifactID  uuid.UUID `json:"artifact_id"`
	TotalChunks int       `json:"total_chunks"`
	ChunkSize   int64     `json:"chunk_size"`
	UploadURL   string    `json:"upload_url"`
}

// ArtifactDownloadInfo contains the chunking parameters for downloading a large artifact.
type ArtifactDownloadInfo struct {
	ArtifactID  uuid.UUID `json:"artifact_id"`
	TotalChunks int       `json:"total_chunks"`
	TotalSize   int64     `json:"total_size"`
	ChunkSize   int64     `json:"chunk_size"`
	DownloadURL string    `json:"download_url"`
}
