// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

// SaveProfileAssignment upserts a's profile set for its scope (one row
// per fleet/group/vessel scope, enforced by the partial unique indexes
// in migration 00006 — see that file's comment for why scope is three
// nullable columns plus a CHECK rather than three tables).
func (s *Store) SaveProfileAssignment(ctx context.Context, a *compliance.ProfileAssignment) error {
	if err := a.Scope.Validate(); err != nil {
		return fmt.Errorf("save profile assignment: %w", err)
	}
	profilesJSON, err := json.Marshal(a.Profiles)
	if err != nil {
		return fmt.Errorf("marshal profiles for scope %s/%s: %w", a.Scope.Type, a.Scope.Key, err)
	}
	vesselID, groupTag := scopeColumns(a.Scope)

	conflictTarget, err := profileConflictTarget(a.Scope.Type)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO regulatory_profile_assignments (scope_type, vessel_id, group_tag, profiles, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT %s DO UPDATE SET profiles = EXCLUDED.profiles, updated_at = EXCLUDED.updated_at
	`, conflictTarget), string(a.Scope.Type), vesselID, groupTag, string(profilesJSON), a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save profile assignment for scope %s/%s: %w", a.Scope.Type, a.Scope.Key, err)
	}
	return nil
}

// ListProfileAssignments returns every stored regulatory profile
// assignment, across all scopes — the shape EffectiveProfiles needs to
// resolve one vessel's enabled set.
func (s *Store) ListProfileAssignments(ctx context.Context) ([]*compliance.ProfileAssignment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope_type, vessel_id, group_tag, profiles, updated_at FROM regulatory_profile_assignments
	`)
	if err != nil {
		return nil, fmt.Errorf("list profile assignments: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*compliance.ProfileAssignment
	for rows.Next() {
		var (
			scopeType    string
			vesselID     sql.NullString
			groupTag     sql.NullString
			profilesJSON []byte
			updatedAt    sql.NullTime
		)
		if err := rows.Scan(&scopeType, &vesselID, &groupTag, &profilesJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan profile assignment: %w", err)
		}
		var profiles []validation.RegulatoryProfile
		if err := json.Unmarshal(profilesJSON, &profiles); err != nil {
			return nil, fmt.Errorf("unmarshal profiles for scope %s: %w", scopeType, err)
		}
		scope, err := scopeFromColumns(scopeType, vesselID, groupTag)
		if err != nil {
			return nil, err
		}
		a := &compliance.ProfileAssignment{Scope: scope, Profiles: profiles}
		if updatedAt.Valid {
			a.UpdatedAt = updatedAt.Time
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile assignments: %w", err)
	}
	return out, nil
}

func profileConflictTarget(t compliance.ScopeType) (string, error) {
	switch t {
	case compliance.ScopeFleet:
		return "(scope_type) WHERE scope_type = 'fleet'", nil
	case compliance.ScopeVessel:
		return "(vessel_id) WHERE scope_type = 'vessel'", nil
	case compliance.ScopeGroup:
		return "(group_tag) WHERE scope_type = 'group'", nil
	default:
		return "", fmt.Errorf("store: unknown scope type %q", t)
	}
}

// scopeColumns splits a Scope into the (vessel_id, group_tag) column
// values regulatory_profile_assignments/cadence_rules share.
func scopeColumns(scope compliance.Scope) (vesselID, groupTag sql.NullString) {
	switch scope.Type {
	case compliance.ScopeVessel:
		return sql.NullString{String: scope.Key, Valid: true}, sql.NullString{}
	case compliance.ScopeGroup:
		return sql.NullString{}, sql.NullString{String: scope.Key, Valid: true}
	default: // ScopeFleet
		return sql.NullString{}, sql.NullString{}
	}
}

func scopeFromColumns(scopeType string, vesselID, groupTag sql.NullString) (compliance.Scope, error) {
	switch compliance.ScopeType(scopeType) {
	case compliance.ScopeFleet:
		return compliance.FleetScope(), nil
	case compliance.ScopeVessel:
		return compliance.VesselScope(vesselID.String)
	case compliance.ScopeGroup:
		return compliance.GroupScope(groupTag.String)
	default:
		return compliance.Scope{}, fmt.Errorf("store: unknown scope type %q", scopeType)
	}
}
