// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/office/apikey"
)

// CreateAPIKey inserts a new API key row.
func (s *Store) CreateAPIKey(ctx context.Context, k *apikey.APIKey) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, label, token_hash, token_lookup_hash, group_id, created_by, created_at, revoked_at, last_used_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, k.ID, k.Label, k.TokenHash, k.TokenLookupHash, k.GroupID, k.CreatedBy, k.CreatedAt, k.RevokedAt, k.LastUsedAt)
	if err != nil {
		return fmt.Errorf("create api key %s: %w", k.ID, err)
	}
	return nil
}

// GetAPIKeyByLookupHash returns the API key whose TokenLookupHash
// matches lookupHash — the O(1) candidate lookup authenticatedAPIKey
// uses before argon2id-verifying a presented bearer token against it.
// Returns ErrNotFound if no row matches.
func (s *Store) GetAPIKeyByLookupHash(ctx context.Context, lookupHash string) (*apikey.APIKey, error) {
	row := s.db.QueryRowContext(ctx, apiKeyColumns+` FROM api_keys WHERE token_lookup_hash = $1`, lookupHash)
	return scanAPIKey(row)
}

// ListAPIKeys returns every API key, newest first (design handoff's
// Administration > API Access tab).
func (s *Store) ListAPIKeys(ctx context.Context) ([]*apikey.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, apiKeyColumns+` FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*apikey.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return out, nil
}

// RevokeAPIKey revokes id's key, if it exists and isn't already revoked
// — a no-op otherwise (RowsAffected deliberately ignored), matching
// RevokeVesselCredential's own idempotent shape.
func (s *Store) RevokeAPIKey(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE api_keys SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL
	`, id, now)
	if err != nil {
		return fmt.Errorf("revoke api key %s: %w", id, err)
	}
	return nil
}

// TouchAPIKeyLastUsed updates id's last_used_at to now — called after
// every successful authenticatedAPIKey check, so an Admin reviewing
// Administration > API Access can tell an active key from a stale one.
func (s *Store) TouchAPIKeyLastUsed(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE api_keys SET last_used_at = $2 WHERE id = $1
	`, id, now)
	if err != nil {
		return fmt.Errorf("touch last-used for api key %s: %w", id, err)
	}
	return nil
}

// GetAPIKeyByID returns the API key with the given id, or ErrNotFound.
// Unlike GetAPIKeyByLookupHash (the bearer-auth candidate lookup), this
// is an admin-facing lookup by surrogate id — handleDeleteAPIKey uses it
// to check the "must already be revoked" rule before calling
// DeleteAPIKey.
func (s *Store) GetAPIKeyByID(ctx context.Context, id string) (*apikey.APIKey, error) {
	row := s.db.QueryRowContext(ctx, apiKeyColumns+` FROM api_keys WHERE id = $1`, id)
	return scanAPIKey(row)
}

// DeleteAPIKey permanently removes id's key and its event history (the
// api_key_events FK is ON DELETE CASCADE). Callers must enforce "already
// revoked" themselves (handleDeleteAPIKey does, via GetAPIKeyByID) —
// this method has no such guard, the same "caller decides, store just
// executes" split RevokeAPIKey's own idempotent no-op already follows.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete api key %s: %w", id, err)
	}
	return nil
}

// APIKeyEvent is one row of an API key's activity log (Administration >
// API Access's per-key log panel) — append-only, recorded at creation,
// revocation, and every successful data-API use (GraphQL and CSV export
// kept distinct — see migration 00033's own doc comment).
type APIKeyEvent struct {
	APIKeyID string
	Kind     string
	At       time.Time
}

// RecordAPIKeyEvent appends one activity-log row for apiKeyID.
func (s *Store) RecordAPIKeyEvent(ctx context.Context, apiKeyID, kind string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_key_events (api_key_id, kind, at) VALUES ($1, $2, $3)
	`, apiKeyID, kind, at)
	if err != nil {
		return fmt.Errorf("record api key event (%s) for %s: %w", kind, apiKeyID, err)
	}
	return nil
}

// ListAPIKeyEvents returns apiKeyID's full activity log, newest first.
func (s *Store) ListAPIKeyEvents(ctx context.Context, apiKeyID string) ([]APIKeyEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT api_key_id, kind, at FROM api_key_events WHERE api_key_id = $1 ORDER BY at DESC
	`, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("list api key events for %s: %w", apiKeyID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []APIKeyEvent
	for rows.Next() {
		var e APIKeyEvent
		if err := rows.Scan(&e.APIKeyID, &e.Kind, &e.At); err != nil {
			return nil, fmt.Errorf("scan api key event: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key events: %w", err)
	}
	return out, nil
}

const apiKeyColumns = `SELECT id, label, token_hash, token_lookup_hash, group_id, created_by, created_at, revoked_at, last_used_at` // #nosec G101 -- a SQL column list, not a credential; gosec's keyword match on "token" false-positives here

func scanAPIKey(row rowScanner) (*apikey.APIKey, error) {
	var (
		k          apikey.APIKey
		groupID    sql.NullString
		revokedAt  sql.NullTime
		lastUsedAt sql.NullTime
	)
	err := row.Scan(&k.ID, &k.Label, &k.TokenHash, &k.TokenLookupHash, &groupID, &k.CreatedBy, &k.CreatedAt, &revokedAt, &lastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	if groupID.Valid {
		k.GroupID = &groupID.String
	}
	if revokedAt.Valid {
		k.RevokedAt = &revokedAt.Time
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.Time
	}
	return &k, nil
}
