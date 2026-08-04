// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// VMSSource is this vessel's configured VMS (voyage management system)
// reference-data REST service — see migration 00018_vms_source.sql's own
// doc comment on the storage/trust model. Structurally identical to
// SensorSource, kept as a separate type/table (not a shared "kind"
// discriminator) since the two sources are configured, enabled, and
// tested independently.
type VMSSource struct {
	BaseURL   string
	APIKey    string
	Enabled   bool
	UpdatedAt time.Time
}

// SaveVMSSource replaces this vessel's stored VMS-source config — at
// most one, same "supersedes in place" shape as SaveSensorSource.
func (s *Store) SaveVMSSource(ctx context.Context, src *VMSSource) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vms_source (id, base_url, api_key, enabled, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET base_url = excluded.base_url, api_key = excluded.api_key, enabled = excluded.enabled, updated_at = excluded.updated_at
	`, src.BaseURL, src.APIKey, boolToInt(src.Enabled), src.UpdatedAt.UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("save vms source: %w", err)
	}
	return nil
}

// GetVMSSource returns this vessel's stored VMS-source config. Returns
// ErrNotFound if never configured.
func (s *Store) GetVMSSource(ctx context.Context) (*VMSSource, error) {
	row := s.db.QueryRowContext(ctx, `SELECT base_url, api_key, enabled, updated_at FROM vms_source WHERE id = 1`)
	var (
		src       VMSSource
		enabled   int
		updatedAt string
	)
	err := row.Scan(&src.BaseURL, &src.APIKey, &enabled, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan vms source: %w", err)
	}
	src.Enabled = enabled != 0
	if src.UpdatedAt, err = time.Parse(timeLayout, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &src, nil
}
