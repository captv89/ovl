// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// VesselSyncStatus is the office's record of a vessel's most recent
// successful SyncStatus call (architecture 11.2 step 4). AppliedBundleID/
// Version are the config bundle the vessel reported it is actually running
// on (audit 2026-07-22 §3.3) — empty/0 until it has applied any bundle.
type VesselSyncStatus struct {
	VesselID             string
	AppVersion           string
	LastSeenAt           time.Time
	AppliedBundleID      string
	AppliedBundleVersion int64
}

// UpsertVesselSyncStatus records vesselID's latest sync check-in,
// replacing any previous one — this is current state, not history.
func (s *Store) UpsertVesselSyncStatus(ctx context.Context, v *VesselSyncStatus) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vessel_sync_status (vessel_id, app_version, last_seen_at, applied_bundle_id, applied_bundle_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (vessel_id) DO UPDATE SET
			app_version = EXCLUDED.app_version,
			last_seen_at = EXCLUDED.last_seen_at,
			applied_bundle_id = EXCLUDED.applied_bundle_id,
			applied_bundle_version = EXCLUDED.applied_bundle_version
	`, v.VesselID, v.AppVersion, v.LastSeenAt, v.AppliedBundleID, v.AppliedBundleVersion)
	if err != nil {
		return fmt.Errorf("upsert vessel sync status for vessel %s: %w", v.VesselID, err)
	}
	return nil
}

// GetVesselSyncStatus returns vesselID's last-seen sync status. Returns
// ErrNotFound if the vessel has never completed a SyncStatus call.
func (s *Store) GetVesselSyncStatus(ctx context.Context, vesselID string) (*VesselSyncStatus, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT vessel_id, app_version, last_seen_at, applied_bundle_id, applied_bundle_version
		FROM vessel_sync_status WHERE vessel_id = $1
	`, vesselID)
	var v VesselSyncStatus
	err := row.Scan(&v.VesselID, &v.AppVersion, &v.LastSeenAt, &v.AppliedBundleID, &v.AppliedBundleVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan vessel sync status: %w", err)
	}
	return &v, nil
}
