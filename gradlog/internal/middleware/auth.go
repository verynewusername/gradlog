// Package middleware provides Gin middleware for authentication and authorization.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/config"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/models"
)

// Context keys used to store authenticated user data inside a Gin context.
const (
	contextKeyUser   = "gradlog_user"
	contextKeyUserID = "gradlog_user_id"
)

// devNoAuthUserID is a fixed synthetic user injected in DEV_NOAUTH_EMAIL mode.
// The UUID is deterministic so project memberships survive restarts.
var devNoAuthUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// DevNoAuthUserID returns the fixed UUID used for the synthetic dev user.
func DevNoAuthUserID() uuid.UUID { return devNoAuthUserID }

// Auth returns a Gin middleware that authenticates requests using an opaque token
// looked up in the api_keys table. Tokens are accepted via:
//   - Authorization: Bearer <token>
//   - Authorization: ApiKey <token>
//
// When cfg.DevNoAuthEmail is non-empty every request is automatically
// authenticated as a synthetic dev user — no token required.
// Unauthenticated requests receive a 401 response.
func Auth(db *database.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── DEV BYPASS ──────────────────────────────────────────────────
		if cfg.DevNoAuthEmail != "" {
			devUser := &models.User{
				ID:    devNoAuthUserID,
				Email: cfg.DevNoAuthEmail,
				Name:  "Dev User",
			}
			c.Set(contextKeyUser, devUser)
			c.Set(contextKeyUserID, devNoAuthUserID)
			c.Next()
			return
		}
		// ── NORMAL AUTH ─────────────────────────────────────────────────
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		scheme := strings.ToLower(parts[0])
		token := parts[1]

		switch scheme {
		case "bearer", "apikey":
			handleAPIKeyAuth(c, db, token)
		default:
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unsupported authorization scheme"})
		}
	}
}

// handleAPIKeyAuth validates an API key and populates the request context with the user.
// The last_used_at timestamp is updated asynchronously to avoid blocking the request.
func handleAPIKeyAuth(c *gin.Context, db *database.DB, key string) {
	if len(key) < 8 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return
	}

	keyHash := hashAPIKey(key)
	var userID uuid.UUID
	var apiKeyID uuid.UUID

	row := db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT ak.id, ak.user_id FROM api_keys ak
		 WHERE ak.key_hash = $1
		   AND (ak.expires_at IS NULL OR ak.expires_at > NOW())`,
		keyHash,
	)
	if err := row.Scan(&apiKeyID, &userID); err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired api key"})
		return
	}

	user, err := getUserByID(c.Request.Context(), db, userID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	// Update last_used_at without blocking the request.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		db.Pool.Exec(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, apiKeyID)
	}()

	c.Set(contextKeyUser, user)
	c.Set(contextKeyUserID, userID)
	c.Next()
}

// getUserByID fetches a user record by their UUID primary key.
func getUserByID(ctx context.Context, db *database.DB, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := db.Pool.QueryRow(
		ctx,
		`SELECT id, email, name, picture_url, google_id, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.PictureURL, &user.GoogleID,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserFromContext retrieves the authenticated user from the Gin context.
// Returns nil if no user is set (request is unauthenticated).
func GetUserFromContext(c *gin.Context) *models.User {
	val, exists := c.Get(contextKeyUser)
	if !exists {
		return nil
	}
	user, _ := val.(*models.User)
	return user
}

// GetUserIDFromContext retrieves the authenticated user's UUID from the Gin context.
// Returns uuid.Nil if no user is authenticated.
func GetUserIDFromContext(c *gin.Context) uuid.UUID {
	val, exists := c.Get(contextKeyUserID)
	if !exists {
		return uuid.Nil
	}
	id, _ := val.(uuid.UUID)
	return id
}

// hashAPIKey returns the hex-encoded SHA-256 of the raw token string.
func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
