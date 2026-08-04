// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SyncCredential is this vessel's redeemed long-lived office sync bearer
// token (architecture 11.1/11.2).
type SyncCredential struct {
	Token    string
	IssuedAt time.Time
}

// SaveSyncCredential replaces this vessel's stored sync credential —
// there is at most one, a vessel syncs with exactly one office, matching
// office/enrollment's own "revoke and re-issue in place" pattern rather
// than accumulating credential history.
func (s *Store) SaveSyncCredential(ctx context.Context, c *SyncCredential) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_credential (id, token, issued_at)
		VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET token = excluded.token, issued_at = excluded.issued_at
	`, c.Token, c.IssuedAt.UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("save sync credential: %w", err)
	}
	return nil
}

// GetSyncCredential returns this vessel's stored sync credential.
// Returns ErrNotFound if the vessel has never redeemed an enrollment
// code.
func (s *Store) GetSyncCredential(ctx context.Context) (*SyncCredential, error) {
	row := s.db.QueryRowContext(ctx, `SELECT token, issued_at FROM sync_credential WHERE id = 1`)
	var (
		c        SyncCredential
		issuedAt string
	)
	err := row.Scan(&c.Token, &issuedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan sync credential: %w", err)
	}
	if c.IssuedAt, err = time.Parse(timeLayout, issuedAt); err != nil {
		return nil, fmt.Errorf("parse issued_at: %w", err)
	}
	return &c, nil
}
