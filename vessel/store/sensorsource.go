// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SensorSource is this vessel's configured onboard sensor-data REST
// service (18.07.26 manual-test items 4/9). See migration
// 00014_sensor_source.sql's own doc comment on the storage/trust model.
type SensorSource struct {
	BaseURL   string
	APIKey    string
	Enabled   bool
	UpdatedAt time.Time
}

// SaveSensorSource replaces this vessel's stored sensor-source config —
// at most one, same "supersedes in place" shape as SaveDRIdentity.
func (s *Store) SaveSensorSource(ctx context.Context, src *SensorSource) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sensor_source (id, base_url, api_key, enabled, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET base_url = excluded.base_url, api_key = excluded.api_key, enabled = excluded.enabled, updated_at = excluded.updated_at
	`, src.BaseURL, src.APIKey, boolToInt(src.Enabled), src.UpdatedAt.UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("save sensor source: %w", err)
	}
	return nil
}

// GetSensorSource returns this vessel's stored sensor-source config.
// Returns ErrNotFound if never configured.
func (s *Store) GetSensorSource(ctx context.Context) (*SensorSource, error) {
	row := s.db.QueryRowContext(ctx, `SELECT base_url, api_key, enabled, updated_at FROM sensor_source WHERE id = 1`)
	var (
		src       SensorSource
		enabled   int
		updatedAt string
	)
	err := row.Scan(&src.BaseURL, &src.APIKey, &enabled, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan sensor source: %w", err)
	}
	src.Enabled = enabled != 0
	if src.UpdatedAt, err = time.Parse(timeLayout, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &src, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
