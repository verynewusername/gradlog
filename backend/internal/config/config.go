// Package config handles loading and validating configuration from environment variables.
// All configuration is loaded at runtime to support containerized deployments.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server settings
	Port string
	Host string

	// Database connection string
	DatabaseURL string

	// Google OAuth settings
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	// Artifact storage settings
	ArtifactStoragePath string
	ArtifactChunkSize   int64 // Size of chunks for upload/download (bytes)
	ArtifactMaxFileSize int64 // Maximum file size (0 = unlimited)

	// Frontend URL for CORS and redirects
	FrontendURL string
}

// Load reads configuration from environment variables.
// It attempts to load a .env file if present but does not require it.
// This allows for both local development (.env file) and production (env vars).
func Load() (*Config, error) {
	// Attempt to load .env file. Ignore errors as env vars may be set directly.
	_ = godotenv.Load()

	cfg := &Config{
		Port:                getEnvOrDefault("PORT", "8080"),
		Host:                getEnvOrDefault("HOST", "0.0.0.0"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:   os.Getenv("GOOGLE_REDIRECT_URL"),
		ArtifactStoragePath: getEnvOrDefault("ARTIFACT_STORAGE_PATH", "/data/artifacts"),
		FrontendURL:         getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
	}

	// Parse chunk size (default 50MB).
	chunkSize, err := strconv.ParseInt(getEnvOrDefault("ARTIFACT_CHUNK_SIZE", "52428800"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ARTIFACT_CHUNK_SIZE: %w", err)
	}
	cfg.ArtifactChunkSize = chunkSize

	// Parse max file size (default 0 = unlimited).
	maxFileSize, err := strconv.ParseInt(getEnvOrDefault("ARTIFACT_MAX_FILE_SIZE", "0"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ARTIFACT_MAX_FILE_SIZE: %w", err)
	}
	cfg.ArtifactMaxFileSize = maxFileSize

	// Validate required configuration.
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required configuration values are present.
func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	// Google OAuth is optional for API-key-only deployments,
	// but warn if not configured.
	if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
		fmt.Println("Warning: Google OAuth not configured. Only API key authentication will be available.")
	}
	return nil
}

// getEnvOrDefault returns the environment variable value or a default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
