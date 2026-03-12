// Package handlers contains HTTP request handlers for the Gradlog API.
// Handlers implement the business logic for each API endpoint.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gradlog/gradlog/internal/config"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	cfg         *config.Config
	db          *database.DB
	oauthConfig *oauth2.Config
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(cfg *config.Config, db *database.DB) *AuthHandler {
	var oauthConfig *oauth2.Config
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		oauthConfig = &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
			Scopes:       []string{"email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}
	return &AuthHandler{
		cfg:         cfg,
		db:          db,
		oauthConfig: oauthConfig,
	}
}

// GoogleLogin initiates the Google OAuth login flow.
// GET /api/v1/auth/google/login
func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	if h.oauthConfig == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Google OAuth is not configured",
		})
		return
	}

	// Generate a state token to prevent CSRF.
	// In production, this should be stored in a session or cookie.
	state := fmt.Sprintf("%d", time.Now().UnixNano())

	url := h.oauthConfig.AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GoogleCallback handles the OAuth callback from Google.
// GET /api/v1/auth/google/callback
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	if h.oauthConfig == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Google OAuth is not configured",
		})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing authorization code",
		})
		return
	}

	// Exchange the code for a token.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	token, err := h.oauthConfig.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to exchange authorization code",
		})
		return
	}

	// Get user info from Google.
	client := h.oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get user info from Google",
		})
		return
	}
	defer resp.Body.Close()

	var googleUser struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to decode user info",
		})
		return
	}

	// Find or create the user.
	user, err := h.findOrCreateUser(ctx, googleUser.ID, googleUser.Email, googleUser.Name, googleUser.Picture)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create user",
		})
		return
	}

	// Generate an opaque session token stored in api_keys.
	rawToken, err := generateSecureAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate session token",
		})
		return
	}

	tokenHash := hashKey(rawToken)
	tokenPrefix := rawToken[:8]
	_, err = h.db.Pool.Exec(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, key_prefix)
		VALUES ($1, 'Login Session', $2, $3)
	`, user.ID, tokenHash, tokenPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create session",
		})
		return
	}

	// Redirect to frontend with token.
	// The frontend stores this token and sends it as: Authorization: Bearer <token>
	redirectURL := fmt.Sprintf("%s/auth/callback?token=%s", h.cfg.FrontendURL, rawToken)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// findOrCreateUser finds an existing user by Google ID or creates a new one.
func (h *AuthHandler) findOrCreateUser(ctx context.Context, googleID, email, name, picture string) (*models.User, error) {
	var user models.User

	// Try to find existing user by Google ID.
	err := h.db.Pool.QueryRow(ctx, `
		SELECT id, email, name, picture_url, created_at, updated_at
		FROM users WHERE google_id = $1
	`, googleID).Scan(&user.ID, &user.Email, &user.Name, &user.PictureURL, &user.CreatedAt, &user.UpdatedAt)

	if err == nil {
		// User exists, update their info.
		_, err = h.db.Pool.Exec(ctx, `
			UPDATE users SET email = $1, name = $2, picture_url = $3 WHERE id = $4
		`, email, name, picture, user.ID)
		user.Email = email
		user.Name = name
		user.PictureURL = &picture
		return &user, err
	}

	// User doesn't exist, create a new one.
	err = h.db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, name, picture_url, google_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, name, picture_url, created_at, updated_at
	`, email, name, picture, googleID).Scan(
		&user.ID, &user.Email, &user.Name, &user.PictureURL, &user.CreatedAt, &user.UpdatedAt,
	)

	return &user, err
}

// GetCurrentUser returns the currently authenticated user.
// GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	user := middleware.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Not authenticated",
		})
		return
	}

	c.JSON(http.StatusOK, user)
}
