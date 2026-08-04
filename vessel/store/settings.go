// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
)

// defaultSyncIntervalSeconds mirrors migration 00015_vessel_settings.sql's
// own seeded default — used only as a fallback if GetSyncIntervalSeconds
// is ever called against a database that predates the migration seed row
// somehow being missing (shouldn't happen; the migration inserts it).
const defaultSyncIntervalSeconds = 300

// GetSyncIntervalSeconds returns this vessel's currently configured sync
// interval (architecture 11.2's "configurable interval"). Migration
// 00015 seeds the single row with the previous hardcoded default (300s),
// so this never needs an ErrNotFound path in practice.
func (s *Store) GetSyncIntervalSeconds(ctx context.Context) (int, error) {
	var seconds int
	row := s.db.QueryRowContext(ctx, `SELECT sync_interval_seconds FROM vessel_settings WHERE id = 1`)
	if err := row.Scan(&seconds); err != nil {
		return defaultSyncIntervalSeconds, fmt.Errorf("get sync interval: %w", err)
	}
	return seconds, nil
}

// SetSyncIntervalSeconds updates the configured sync interval. Range
// validation (Master-facing min/max) is httpapi's job, same split as
// SensorSource's own enabled/URL validation.
func (s *Store) SetSyncIntervalSeconds(ctx context.Context, seconds int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE vessel_settings SET sync_interval_seconds = ? WHERE id = 1`, seconds)
	if err != nil {
		return fmt.Errorf("set sync interval: %w", err)
	}
	return nil
}
