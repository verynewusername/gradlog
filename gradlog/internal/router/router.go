// Package router configures and returns the Gin HTTP router for Gradlog.
package router

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gradlog/gradlog/internal/config"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/handlers"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/storage"
	"github.com/gradlog/gradlog/internal/ui"
)

// Setup creates a configured Gin engine with all routes registered.
func Setup(cfg *config.Config, db *database.DB, store *storage.LocalStorage) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS(cfg))

	// In DEV_NOAUTH_EMAIL mode, upsert the synthetic dev user so that all
	// DB foreign-key constraints (project_members, etc.) are satisfied.
	if cfg.DevNoAuthEmail != "" && db != nil {
		_, err := db.Pool.Exec(context.Background(), `
			INSERT INTO users (id, email, name, picture_url, google_id, created_at, updated_at)
			VALUES ($1, $2, 'Dev User', NULL, NULL, NOW(), NOW())
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, updated_at = NOW()
		`, middleware.DevNoAuthUserID(), cfg.DevNoAuthEmail)
		if err != nil {
			log.Printf("warning: could not upsert dev noauth user: %v", err)
		} else {
			log.Printf("DEV_NOAUTH_EMAIL: auto-authenticated as %q (id=%s)", cfg.DevNoAuthEmail, middleware.DevNoAuthUserID())
		}
	}

	// Health check — no authentication required.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Initialise handlers.
	projectHandler := handlers.NewProjectHandler(db)
	experimentHandler := handlers.NewExperimentHandler(db, projectHandler)
	runHandler := handlers.NewRunHandler(db, projectHandler)
	metricHandler := handlers.NewMetricHandler(db, projectHandler)
	artifactHandler := handlers.NewArtifactHandler(db, store, cfg, projectHandler)
	authHandler := handlers.NewAuthHandler(cfg, db)
	apiKeyHandler := handlers.NewAPIKeyHandler(db)

	// ----------------------------------------------------------
	// Unauthenticated routes
	// ----------------------------------------------------------
	auth := r.Group("/api/v1/auth")
	{
		auth.GET("/google/login", authHandler.GoogleLogin)
		auth.GET("/google/callback", authHandler.GoogleCallback)
	}

	// ----------------------------------------------------------
	// Authenticated routes
	// ----------------------------------------------------------
	protected := r.Group("/api/v1")
	protected.Use(middleware.Auth(db, cfg))
	{
		// Current-user endpoints.
		protected.GET("/auth/me", authHandler.GetCurrentUser)

		// API key management.
		keys := protected.Group("/api-keys")
		{
			keys.POST("", apiKeyHandler.CreateAPIKey)
			keys.GET("", apiKeyHandler.ListAPIKeys)
			keys.DELETE("/:id", apiKeyHandler.DeleteAPIKey)
		}

		// Projects.
		projects := protected.Group("/projects")
		{
			projects.POST("", projectHandler.CreateProject)
			projects.GET("", projectHandler.ListProjects)
			projects.GET("/:id", projectHandler.GetProject)
			projects.PATCH("/:id", projectHandler.UpdateProject)
			projects.DELETE("/:id", projectHandler.DeleteProject)

			// Project membership.
			projects.GET("/:id/members", projectHandler.ListMembers)
			projects.POST("/:id/members", projectHandler.AddMember)
			projects.DELETE("/:id/members/:userId", projectHandler.RemoveMember)
		}

		// Experiments nested under a project (must be a separate group to avoid
		// wildcard name conflict between :id and :projectId in the same segment).
		protected.POST("/projects/:id/experiments", experimentHandler.CreateExperiment)
		protected.GET("/projects/:id/experiments", experimentHandler.ListExperiments)

		// Experiments (individual resource access).
		protected.GET("/experiments/:id", experimentHandler.GetExperiment)
		protected.PATCH("/experiments/:id", experimentHandler.UpdateExperiment)
		protected.DELETE("/experiments/:id", experimentHandler.DeleteExperiment)
		// Runs nested under an experiment (separate from /:id to avoid wildcard conflict).
		protected.POST("/experiments/:id/runs", runHandler.CreateRun)
		protected.GET("/experiments/:id/runs", runHandler.ListRuns)

		// Runs (individual resource access).
		protected.GET("/runs/:id", runHandler.GetRun)
		protected.PATCH("/runs/:id", runHandler.UpdateRun)
		protected.DELETE("/runs/:id", runHandler.DeleteRun)
		// Metrics nested under a run.
		protected.POST("/runs/:id/metrics", metricHandler.LogMetric)
		protected.POST("/runs/:id/metrics/batch", metricHandler.LogMetricsBatch)
		protected.GET("/runs/:id/metrics", metricHandler.GetMetrics)
		protected.GET("/runs/:id/metrics/latest", metricHandler.GetLatestMetrics)
		protected.GET("/runs/:id/metrics/:key/history", metricHandler.GetMetricHistory)
		// Artifacts nested under a run.
		protected.GET("/runs/:id/artifacts", artifactHandler.ListArtifacts)
		protected.POST("/runs/:id/artifacts/upload", artifactHandler.SimpleUpload)
		protected.POST("/runs/:id/artifacts/init", artifactHandler.InitUpload)

		// Artifacts (individual resource access).
		artifacts := protected.Group("/artifacts")
		{
			artifacts.POST("/:artifactId/chunks/:chunkNumber", artifactHandler.UploadChunk)
			artifacts.POST("/:artifactId/complete", artifactHandler.CompleteUpload)
			artifacts.GET("/:artifactId/download-info", artifactHandler.GetDownloadInfo)
			artifacts.GET("/:artifactId/chunks/:chunkNumber", artifactHandler.DownloadChunk)
			artifacts.GET("/:artifactId/download", artifactHandler.SimpleDownload)
			artifacts.DELETE("/:artifactId", artifactHandler.DeleteArtifact)
		}
	}

	// ----------------------------------------------------------
	// Frontend — serve embedded static files.
	// Any path not matched by /api/v1/* falls through to here.
	// Unknown paths return index.html so client-side routing works.
	// ----------------------------------------------------------
	distFS, err := ui.DistFS()
	if err != nil {
		log.Fatalf("failed to load embedded UI: %v", err)
	}
	fileServer := http.FileServer(distFS)

	r.NoRoute(func(c *gin.Context) {
		// Avoid stale SPA assets after deploys. A mismatched cached app.js can
		// crash startup if HTML and JS versions diverge.
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		// Try serving the exact file first.
		f, err := distFS.Open(c.Request.URL.Path)
		if err != nil {
			// File not found — serve index.html for client-side routing.
			index, err := distFS.Open("/index.html")
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			defer index.Close()
			c.Status(http.StatusOK)
			c.Header("Content-Type", "text/html; charset=utf-8")
			io.Copy(c.Writer, index)
			return
		}
		f.Close()
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	return r
}
