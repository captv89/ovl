// SPDX-License-Identifier: AGPL-3.0-only

// Package store is ovl-office's PostgreSQL persistence layer (architecture
// 12.1). Uses jackc/pgx/v5's database/sql driver and goose for migrations,
// mirroring vessel/store's shape on a different backing database.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned by lookups that find no matching row.
var ErrNotFound = errors.New("store: not found")

// Pool tuning: an office instance serves one company's Postgres, not a
// multi-tenant fleet, so a modest fixed pool is enough — revisit if a
// real deployment's connection metrics ever show contention.
const (
	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 5 * time.Minute
)

// Store is ovl-office's Postgres-backed store. The zero value is not
// usable; construct with Open.
type Store struct {
	db *sql.DB
}

// Open connects to the Postgres instance at dsn and applies any pending
// migrations. dsn is a standard libpq connection string or URL (e.g.
// "postgres://user:pass@host:5432/dbname?sslmode=disable").
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping verifies the database connection is alive, for the HTTP health
// endpoint.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
