// Package main is the entry point for the Gradlog server.
// Gradlog is an open-source ML experiment tracking platform similar to MLflow.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gradlog/gradlog/internal/config"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/router"
	"github.com/gradlog/gradlog/internal/storage"
)

func main() {
	// Load configuration from environment variables.
	// Configuration is loaded at runtime, not baked into the binary.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database connection and run embedded migrations.
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run database migrations embedded in the binary.
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize artifact storage backend.
	artifactStorage, err := storage.NewLocalStorage(cfg.ArtifactStoragePath)
	if err != nil {
		log.Fatalf("Failed to initialize artifact storage: %v", err)
	}

	// Setup the Gin router with all routes and middleware.
	r := router.Setup(cfg, db, artifactStorage)

	// Create HTTP server with timeouts.
	srv := &http.Server{
		Addr:         cfg.Host + ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine for graceful shutdown support.
	go func() {
		log.Printf("Starting Gradlog server on %s:%s", cfg.Host, cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
