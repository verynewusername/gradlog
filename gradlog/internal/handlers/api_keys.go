// Package handlers contains HTTP request handlers for the Gradlog API.
package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
)

// APIKeyHandler handles API key management requests.
type APIKeyHandler struct {
	db *database.DB
}

// NewAPIKeyHandler creates a new API key handler.
func NewAPIKeyHandler(db *database.DB) *APIKeyHandler {
	return &APIKeyHandler{db: db}
}

// CreateAPIKeyRequest is the request body for creating a new API key.
type CreateAPIKeyRequest struct {
	Name      string `json:"name" binding:"required"`
	ExpiresIn *int   `json:"expires_in"` // Days until expiration, nil for no expiration.
}

// CreateAPIKey generates a new API key for the authenticated user.
// POST /api/v1/api-keys
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Not authenticated",
		})
		return
	}

	// Generate a secure random API key.
	key, err := generateSecureAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate API key",
		})
		return
	}

	// Hash the key for storage.
	keyHash := hashKey(key)
	keyPrefix := key[:8]

	// Calculate expiration if specified.
	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, *req.ExpiresIn)
		expiresAt = &exp
	}

	// Insert the API key.
	var apiKey models.APIKey
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO api_keys (user_id, name, key_hash, key_prefix, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, name, key_prefix, expires_at, created_at
	`, userID, req.Name, keyHash, keyPrefix, expiresAt).Scan(
		&apiKey.ID, &apiKey.UserID, &apiKey.Name, &apiKey.KeyPrefix, &apiKey.ExpiresAt, &apiKey.CreatedAt,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create API key",
		})
		return
	}

	// Return the full key only this once.
	response := models.APIKeyWithSecret{
		APIKey: apiKey,
		Key:    key,
	}

	c.JSON(http.StatusCreated, response)
}

// ListAPIKeys returns all API keys for the authenticated user.
// GET /api/v1/api-keys
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Not authenticated",
		})
		return
	}

	rows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT id, user_id, name, key_prefix, last_used_at, expires_at, created_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch API keys",
		})
		return
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var key models.APIKey
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt); err != nil {
			continue
		}
		keys = append(keys, key)
	}

	if keys == nil {
		keys = []models.APIKey{}
	}

	c.JSON(http.StatusOK, keys)
}

// DeleteAPIKey deletes an API key.
// DELETE /api/v1/api-keys/:id
func (h *APIKeyHandler) DeleteAPIKey(c *gin.Context) {
	keyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid API key ID",
		})
		return
	}

	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Not authenticated",
		})
		return
	}

	// Delete the key only if it belongs to the user.
	result, err := h.db.Pool.Exec(c.Request.Context(), `
		DELETE FROM api_keys WHERE id = $1 AND user_id = $2
	`, keyID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete API key",
		})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "API key not found",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// generateSecureAPIKey creates a cryptographically secure API key.
// Format: gl_<40 hex characters>
func generateSecureAPIKey() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "gl_" + hex.EncodeToString(bytes), nil
}

// hashKey creates a SHA-256 hash of an API key.
func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
