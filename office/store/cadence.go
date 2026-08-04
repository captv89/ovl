// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/captv89/ovl/office/compliance"
)

// SaveCadenceRule upserts r for its scope (one row per fleet/group/
// vessel scope, same partial-unique-index shape as
// regulatory_profile_assignments — see migration 00007 and
// profileConflictTarget's sibling here, cadenceConflictTarget).
func (s *Store) SaveCadenceRule(ctx context.Context, r *compliance.CadenceRule) error {
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("save cadence rule: %w", err)
	}
	vesselID, groupTag := scopeColumns(r.Scope)
	conflictTarget, err := cadenceConflictTarget(r.Scope.Type)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO cadence_rules (scope_type, vessel_id, group_tag, min_report_interval_hours, max_gap_hours, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT %s DO UPDATE SET
			min_report_interval_hours = EXCLUDED.min_report_interval_hours,
			max_gap_hours = EXCLUDED.max_gap_hours,
			updated_at = EXCLUDED.updated_at
	`, conflictTarget), string(r.Scope.Type), vesselID, groupTag, r.MinReportIntervalHours, r.MaxGapHours, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save cadence rule for scope %s/%s: %w", r.Scope.Type, r.Scope.Key, err)
	}
	return nil
}

// ListCadenceRules returns every stored cadence rule, across all scopes
// — the shape EffectiveCadence needs to resolve one vessel's rule.
func (s *Store) ListCadenceRules(ctx context.Context) ([]*compliance.CadenceRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope_type, vessel_id, group_tag, min_report_interval_hours, max_gap_hours, updated_at FROM cadence_rules
	`)
	if err != nil {
		return nil, fmt.Errorf("list cadence rules: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*compliance.CadenceRule
	for rows.Next() {
		var (
			scopeType                           string
			vesselID, groupTag                  sql.NullString
			minReportIntervalHours, maxGapHours float64
			updatedAt                           sql.NullTime
		)
		if err := rows.Scan(&scopeType, &vesselID, &groupTag, &minReportIntervalHours, &maxGapHours, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan cadence rule: %w", err)
		}
		scope, err := scopeFromColumns(scopeType, vesselID, groupTag)
		if err != nil {
			return nil, err
		}
		r := &compliance.CadenceRule{Scope: scope, MinReportIntervalHours: minReportIntervalHours, MaxGapHours: maxGapHours}
		if updatedAt.Valid {
			r.UpdatedAt = updatedAt.Time
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cadence rules: %w", err)
	}
	return out, nil
}

func cadenceConflictTarget(t compliance.ScopeType) (string, error) {
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
