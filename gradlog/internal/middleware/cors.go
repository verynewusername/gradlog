// Package middleware provides HTTP middleware for the Gradlog API.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/gradlog/gradlog/internal/config"
)

// CORS configures Cross-Origin Resource Sharing headers.
// This allows the frontend (hosted on a different domain) to make requests.
func CORS(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Allow the configured frontend URL and localhost for development.
		allowedOrigins := []string{
			cfg.FrontendURL,
			"http://localhost:3000",
			"http://127.0.0.1:3000",
		}

		isAllowed := false
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				isAllowed = true
				break
			}
		}

		if isAllowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight requests.
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
