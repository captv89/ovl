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

// SaveRuleSeverityAssignment upserts a's severity overrides for its scope
// (one row per fleet/group/vessel scope, same partial-unique-index shape
// as regulatory_profile_assignments/cadence_rules — see migration 00009).
func (s *Store) SaveRuleSeverityAssignment(ctx context.Context, a *compliance.RuleSeverityAssignment) error {
	if err := a.Scope.Validate(); err != nil {
		return fmt.Errorf("save rule severity assignment: %w", err)
	}
	severitiesJSON, err := json.Marshal(a.Severities)
	if err != nil {
		return fmt.Errorf("marshal severities for scope %s/%s: %w", a.Scope.Type, a.Scope.Key, err)
	}
	vesselID, groupTag := scopeColumns(a.Scope)

	conflictTarget, err := ruleSeverityConflictTarget(a.Scope.Type)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO rule_severity_assignments (scope_type, vessel_id, group_tag, severities, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT %s DO UPDATE SET severities = EXCLUDED.severities, updated_at = EXCLUDED.updated_at
	`, conflictTarget), string(a.Scope.Type), vesselID, groupTag, string(severitiesJSON), a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save rule severity assignment for scope %s/%s: %w", a.Scope.Type, a.Scope.Key, err)
	}
	return nil
}

// ListRuleSeverityAssignments returns every stored rule severity
// assignment, across all scopes — the shape EffectiveSeverities needs to
// resolve one vessel's overrides.
func (s *Store) ListRuleSeverityAssignments(ctx context.Context) ([]*compliance.RuleSeverityAssignment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope_type, vessel_id, group_tag, severities, updated_at FROM rule_severity_assignments
	`)
	if err != nil {
		return nil, fmt.Errorf("list rule severity assignments: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*compliance.RuleSeverityAssignment
	for rows.Next() {
		var (
			scopeType      string
			vesselID       sql.NullString
			groupTag       sql.NullString
			severitiesJSON []byte
			updatedAt      sql.NullTime
		)
		if err := rows.Scan(&scopeType, &vesselID, &groupTag, &severitiesJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan rule severity assignment: %w", err)
		}
		var severities map[string]validation.Severity
		if err := json.Unmarshal(severitiesJSON, &severities); err != nil {
			return nil, fmt.Errorf("unmarshal severities for scope %s: %w", scopeType, err)
		}
		scope, err := scopeFromColumns(scopeType, vesselID, groupTag)
		if err != nil {
			return nil, err
		}
		a := &compliance.RuleSeverityAssignment{Scope: scope, Severities: severities}
		if updatedAt.Valid {
			a.UpdatedAt = updatedAt.Time
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rule severity assignments: %w", err)
	}
	return out, nil
}

func ruleSeverityConflictTarget(t compliance.ScopeType) (string, error) {
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
