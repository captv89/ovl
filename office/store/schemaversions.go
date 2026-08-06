// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/captv89/ovl/office/schemaversions"
)

// CreateSchemaVersion inserts a new, immutable schema version record.
// Returns an error if (schema_name, version) already exists (the
// schema_versions UNIQUE constraint) — architecture 5.2's "a new version
// is always a new record" means there is no update path here at all,
// unlike UpdateVessel/UpdateUser.
func (s *Store) CreateSchemaVersion(ctx context.Context, sv *schemaversions.SchemaVersion) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO schema_versions (id, schema_name, version, source, content, published_at, published_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, sv.ID, sv.SchemaName, sv.Version, string(sv.Source), sv.Content, sv.PublishedAt, sv.PublishedBy)
	if err != nil {
		return fmt.Errorf("create schema version %s@%s: %w", sv.SchemaName, sv.Version, err)
	}
	return nil
}

// GetSchemaVersion returns one schema's specific published version.
func (s *Store) GetSchemaVersion(ctx context.Context, schemaName, version string) (*schemaversions.SchemaVersion, error) {
	row := s.db.QueryRowContext(ctx, schemaVersionColumns+`
		FROM schema_versions WHERE schema_name = $1 AND version = $2
	`, schemaName, version)
	return scanSchemaVersion(row)
}

// LatestSchemaVersion returns the most recently published version of
// schemaName — the "current version" PrepareUpload diffs a new upload
// against (architecture 5.3 step 5). Returns ErrNotFound if schemaName has
// never had a version published, which callers should treat the same way
// PrepareUpload treats a nil current version: nothing to diff against.
func (s *Store) LatestSchemaVersion(ctx context.Context, schemaName string) (*schemaversions.SchemaVersion, error) {
	row := s.db.QueryRowContext(ctx, schemaVersionColumns+`
		FROM schema_versions WHERE schema_name = $1 ORDER BY published_at DESC LIMIT 1
	`, schemaName)
	return scanSchemaVersion(row)
}

// ListSchemaVersions returns every published version of schemaName,
// newest first — design handoff B5's version list screen.
func (s *Store) ListSchemaVersions(ctx context.Context, schemaName string) ([]*schemaversions.SchemaVersion, error) {
	rows, err := s.db.QueryContext(ctx, schemaVersionColumns+`
		FROM schema_versions WHERE schema_name = $1 ORDER BY published_at DESC
	`, schemaName)
	if err != nil {
		return nil, fmt.Errorf("list schema versions for %s: %w", schemaName, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*schemaversions.SchemaVersion
	for rows.Next() {
		sv, err := scanSchemaVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema versions for %s: %w", schemaName, err)
	}
	return out, nil
}

// ListSchemaNames returns every distinct schema name with at least one
// published version, alphabetically — the schema picker B5/B6 build
// their screens from.
func (s *Store) ListSchemaNames(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT schema_name FROM schema_versions ORDER BY schema_name`)
	if err != nil {
		return nil, fmt.Errorf("list schema names: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan schema name: %w", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema names: %w", err)
	}
	return out, nil
}

// SchemaVersionCursorItem pairs a published schema version with its
// cursor value — PullInbox's own pull position, distinct from
// PublishedAt (see migration 00016's comment on why a dedicated
// monotonic column exists at all).
type SchemaVersionCursorItem struct {
	Version *schemaversions.SchemaVersion
	Cursor  int64
}

// ListSchemaVersionsSince returns every schema version published after
// sinceCursor, cursor ascending — the global stream PullInbox sends
// (Phase 4 decision: schema versions are not scoped per vessel/bundle).
func (s *Store) ListSchemaVersionsSince(ctx context.Context, sinceCursor int64) ([]SchemaVersionCursorItem, error) {
	rows, err := s.db.QueryContext(ctx, schemaVersionColumns+`, cursor
		FROM schema_versions WHERE cursor > $1 ORDER BY cursor ASC
	`, sinceCursor)
	if err != nil {
		return nil, fmt.Errorf("list schema versions since cursor %d: %w", sinceCursor, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []SchemaVersionCursorItem
	for rows.Next() {
		var (
			sv     schemaversions.SchemaVersion
			source string
			cursor int64
		)
		if err := rows.Scan(&sv.ID, &sv.SchemaName, &sv.Version, &source, &sv.Content, &sv.PublishedAt, &sv.PublishedBy, &cursor); err != nil {
			return nil, fmt.Errorf("scan schema version: %w", err)
		}
		sv.Source = schemaversions.Source(source)
		out = append(out, SchemaVersionCursorItem{Version: &sv, Cursor: cursor})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema versions since cursor %d: %w", sinceCursor, err)
	}
	return out, nil
}

const schemaVersionColumns = `SELECT id, schema_name, version, source, content, published_at, published_by`

func scanSchemaVersion(row rowScanner) (*schemaversions.SchemaVersion, error) {
	var (
		sv     schemaversions.SchemaVersion
		source string
	)
	err := row.Scan(&sv.ID, &sv.SchemaName, &sv.Version, &source, &sv.Content, &sv.PublishedAt, &sv.PublishedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan schema version: %w", err)
	}
	sv.Source = schemaversions.Source(source)
	return &sv, nil
}
