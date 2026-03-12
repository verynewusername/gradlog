// Package database provides database connection and migration functionality.
// Migrations are embedded in the binary for single-binary deployment.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a connection pool and provides database operations.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new database connection pool.
// The connection string should be in PostgreSQL format:
// postgres://user:password@host:port/database?sslmode=disable
func New(connString string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Configure connection pool settings.
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection works.
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close closes all database connections.
func (db *DB) Close() {
	db.Pool.Close()
}

// Migrate runs all pending database migrations.
// Migrations are embedded in the binary using Go's embed package.
func (db *DB) Migrate() error {
	// Create a source driver from the embedded filesystem.
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	// We need a standard *sql.DB for the migrate library.
	// Register the pgx ConnConfig and open via database/sql.
	connConfig := db.Pool.Config().ConnConfig
	key := stdlib.RegisterConnConfig(connConfig)
	defer stdlib.UnregisterConnConfig(key)
	sqlDB, err := sql.Open("pgx", key)
	if err != nil {
		return fmt.Errorf("failed to open stdlib connection: %w", err)
	}
	defer sqlDB.Close()

	// Create the postgres driver.
	dbDriver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}

	// Create the migrator.
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	// Run all up migrations.
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
