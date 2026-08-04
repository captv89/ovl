// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/captv89/ovl/office/vessels"
)

// CreateVessel inserts a new vessel profile. Returns an error if imo is
// already taken (the vessels.imo UNIQUE constraint) — an IMO number
// identifies one real hull, so it can never be shared by two rows.
func (s *Store) CreateVessel(ctx context.Context, v *vessels.Vessel) error {
	groupsJSON, err := json.Marshal(v.Groups)
	if err != nil {
		return fmt.Errorf("marshal groups for vessel %s: %w", v.IMO, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vessels (id, imo, name, type, groups, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, v.ID, v.IMO, v.Name, v.Type, string(groupsJSON), v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create vessel %s: %w", v.IMO, err)
	}
	return nil
}

// UpdateVessel persists changes to an existing vessel's profile (name,
// type, groups). IMO is never updated — vessels.Vessel has no setter for
// it either.
func (s *Store) UpdateVessel(ctx context.Context, v *vessels.Vessel) error {
	groupsJSON, err := json.Marshal(v.Groups)
	if err != nil {
		return fmt.Errorf("marshal groups for vessel %s: %w", v.ID, err)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE vessels SET
			name = $1, type = $2, groups = $3, updated_at = $4
		WHERE id = $5
	`, v.Name, v.Type, string(groupsJSON), v.UpdatedAt, v.ID)
	if err != nil {
		return fmt.Errorf("update vessel %s: %w", v.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update vessel %s: %w", v.ID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetVessel returns the vessel with the given ID.
func (s *Store) GetVessel(ctx context.Context, id string) (*vessels.Vessel, error) {
	row := s.db.QueryRowContext(ctx, vesselColumns+` FROM vessels WHERE id = $1`, id)
	return scanVessel(row)
}

// GetVesselByIMO returns the vessel with the given IMO number.
func (s *Store) GetVesselByIMO(ctx context.Context, imo string) (*vessels.Vessel, error) {
	row := s.db.QueryRowContext(ctx, vesselColumns+` FROM vessels WHERE imo = $1`, imo)
	return scanVessel(row)
}

// ListVessels returns every vessel, ordered by name — the shape design
// handoff B2's fleet list needs.
func (s *Store) ListVessels(ctx context.Context) ([]*vessels.Vessel, error) {
	rows, err := s.db.QueryContext(ctx, vesselColumns+` FROM vessels ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list vessels: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*vessels.Vessel
	for rows.Next() {
		v, err := scanVessel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vessels: %w", err)
	}
	return out, nil
}

// ListVesselsByGroup returns every vessel tagged with group, ordered by
// name — design handoff B2/B3's "filter by group" (config assignment
// and dashboard filtering, architecture 12.4).
func (s *Store) ListVesselsByGroup(ctx context.Context, group string) ([]*vessels.Vessel, error) {
	// groups @> '["Fleet A"]' checks whether the groups array contains
	// "Fleet A" as an element — simpler than a jsonb_build_array(...)
	// call, since json.Marshal on a one-element slice already produces
	// exactly the array literal the @> containment operator expects.
	needleJSON, err := json.Marshal([]string{group})
	if err != nil {
		return nil, fmt.Errorf("marshal group %q: %w", group, err)
	}
	rows, err := s.db.QueryContext(ctx, vesselColumns+`
		FROM vessels WHERE groups @> $1::jsonb ORDER BY name ASC
	`, string(needleJSON))
	if err != nil {
		return nil, fmt.Errorf("list vessels by group %q: %w", group, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*vessels.Vessel
	for rows.Next() {
		v, err := scanVessel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vessels: %w", err)
	}
	return out, nil
}

const vesselColumns = `SELECT id, imo, name, type, groups, created_at, updated_at`

func scanVessel(row rowScanner) (*vessels.Vessel, error) {
	var (
		v          vessels.Vessel
		groupsJSON []byte
	)
	err := row.Scan(&v.ID, &v.IMO, &v.Name, &v.Type, &groupsJSON, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan vessel: %w", err)
	}
	if err := json.Unmarshal(groupsJSON, &v.Groups); err != nil {
		return nil, fmt.Errorf("unmarshal groups for vessel %s: %w", v.ID, err)
	}
	return &v, nil
}
