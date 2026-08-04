// SPDX-License-Identifier: AGPL-3.0-only

// Package store is ovl-vessel's SQLite persistence layer (architecture
// 9.1): reports, the audit trail, and a content-addressed attachment
// store on the filesystem. Uses modernc.org/sqlite (pure Go, CGO off)
// and WAL mode, migrated with goose.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// dbFileName is the SQLite database file within a vessel's data
// directory (architecture 9.2's "choose a data directory" first-run
// step).
const dbFileName = "ovl.db"

// Store is ovl-vessel's SQLite-backed store. The zero value is not
// usable; construct with Open.
type Store struct {
	db *sql.DB
}

// Open creates dataDir if needed, opens (or creates) dataDir/ovl.db,
// enables WAL mode, and applies any pending migrations.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data directory %s: %w", dataDir, err)
	}
	dbPath := filepath.Join(dataDir, dbFileName)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set %q: %w", pragma, err)
		}
	}
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}
