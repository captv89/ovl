// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// VesselIdentity is this vessel's own copy of the office-side vessel
// profile, captured once at enrollment redemption.
type VesselIdentity struct {
	Name string
	IMO  string
}

// SaveVesselIdentity replaces this vessel's stored identity — there is at
// most one, matching SaveSyncCredential's singleton-row pattern.
func (s *Store) SaveVesselIdentity(ctx context.Context, v *VesselIdentity) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vessel_identity (id, name, imo)
		VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, imo = excluded.imo
	`, v.Name, v.IMO)
	if err != nil {
		return fmt.Errorf("save vessel identity: %w", err)
	}
	return nil
}

// GetVesselIdentity returns this vessel's stored identity. Returns
// ErrNotFound if the vessel has never redeemed an enrollment code (the
// offline/deferred-enrollment path never populates this).
func (s *Store) GetVesselIdentity(ctx context.Context) (*VesselIdentity, error) {
	row := s.db.QueryRowContext(ctx, `SELECT name, imo FROM vessel_identity WHERE id = 1`)
	var v VesselIdentity
	err := row.Scan(&v.Name, &v.IMO)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan vessel identity: %w", err)
	}
	return &v, nil
}
