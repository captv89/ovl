// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/pkg/validation"
)

// SaveFieldPolicy upserts p's whole policy/prefill map for its
// (Scope, SchemaName, SchemaVersion) — one row per scope, same partial-
// unique-index upsert-in-place shape as SaveProfileAssignment/
// SaveCadenceRule/SaveRuleSeverityAssignment (migration 00031 gave
// field_policy_assignments the identical scope shape those tables
// already had). A whole-map replace, not a diff-and-mutate, matching
// this type's pre-scoping behavior (see the old per-field-row
// implementation this migration replaced).
func (s *Store) SaveFieldPolicy(ctx context.Context, p *fieldpolicy.SchemaFieldPolicy) error {
	if err := p.Scope.Validate(); err != nil {
		return fmt.Errorf("save field policy for %s@%s: %w", p.SchemaName, p.SchemaVersion, err)
	}
	policyJSON, err := json.Marshal(stringifyFieldPolicyStates(p.Policy))
	if err != nil {
		return fmt.Errorf("marshal policy for %s@%s: %w", p.SchemaName, p.SchemaVersion, err)
	}
	prefillJSON, err := json.Marshal(stringifyPrefillClasses(p.Prefill))
	if err != nil {
		return fmt.Errorf("marshal prefill for %s@%s: %w", p.SchemaName, p.SchemaVersion, err)
	}
	eventsJSON, err := json.Marshal(map[string][]string(p.Events))
	if err != nil {
		return fmt.Errorf("marshal events for %s@%s: %w", p.SchemaName, p.SchemaVersion, err)
	}
	vesselID, groupTag := scopeColumns(p.Scope)

	conflictTarget, err := fieldPolicyConflictTarget(p.Scope.Type)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO field_policy_assignments (scope_type, vessel_id, group_tag, schema_name, schema_version, policy, prefill, events, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT %s DO UPDATE SET policy = EXCLUDED.policy, prefill = EXCLUDED.prefill, events = EXCLUDED.events, updated_at = EXCLUDED.updated_at
	`, conflictTarget), string(p.Scope.Type), vesselID, groupTag, p.SchemaName, p.SchemaVersion, string(policyJSON), string(prefillJSON), string(eventsJSON), p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save field policy for %s/%s@%s: %w", p.Scope.Type, p.SchemaName, p.SchemaVersion, err)
	}
	return nil
}

// LoadFieldPolicy returns scope's stored overrides for (schemaName,
// schemaVersion). No row found is not an error — it means every field
// is still at its default state for that scope — so this always
// returns a valid, possibly-empty SchemaFieldPolicy rather than
// ErrNotFound.
func (s *Store) LoadFieldPolicy(ctx context.Context, scope compliance.Scope, schemaName, schemaVersion string) (*fieldpolicy.SchemaFieldPolicy, error) {
	vesselID, groupTag := scopeColumns(scope)
	row := s.db.QueryRowContext(ctx, `
		SELECT policy, prefill, events, updated_at FROM field_policy_assignments
		WHERE scope_type = $1 AND vessel_id IS NOT DISTINCT FROM $2 AND group_tag IS NOT DISTINCT FROM $3
		  AND schema_name = $4 AND schema_version = $5
	`, string(scope.Type), vesselID, groupTag, schemaName, schemaVersion)
	p, err := scanFieldPolicyRow(row, scope, schemaName, schemaVersion)
	if err != nil {
		return nil, fmt.Errorf("load field policy for %s/%s@%s: %w", scope.Type, schemaName, schemaVersion, err)
	}
	return p, nil
}

// ListFieldPolicyAssignments returns every stored field policy
// assignment for (schemaName, schemaVersion), across every scope — the
// shape EffectiveFieldPolicy needs to resolve one vessel's effective
// policy, and what composeCurrentBundle embeds wholesale into a
// published bundle (architecture 6.5's "field policies per schema" is
// now a list across scopes, matching RegulatoryProfiles/CadenceRules/
// RuleSeverities' own already-scoped shape).
func (s *Store) ListFieldPolicyAssignments(ctx context.Context, schemaName, schemaVersion string) ([]*fieldpolicy.SchemaFieldPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope_type, vessel_id, group_tag, policy, prefill, events, updated_at
		FROM field_policy_assignments WHERE schema_name = $1 AND schema_version = $2
	`, schemaName, schemaVersion)
	if err != nil {
		return nil, fmt.Errorf("list field policy assignments for %s@%s: %w", schemaName, schemaVersion, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*fieldpolicy.SchemaFieldPolicy
	for rows.Next() {
		var (
			scopeType          string
			vesselID, groupTag sql.NullString
			policyJSON         []byte
			prefillJSON        []byte
			eventsJSON         []byte
			updatedAt          sql.NullTime
		)
		if err := rows.Scan(&scopeType, &vesselID, &groupTag, &policyJSON, &prefillJSON, &eventsJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan field policy assignment for %s@%s: %w", schemaName, schemaVersion, err)
		}
		scope, err := scopeFromColumns(scopeType, vesselID, groupTag)
		if err != nil {
			return nil, err
		}
		p, err := parseFieldPolicyRow(scope, schemaName, schemaVersion, policyJSON, prefillJSON, eventsJSON, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate field policy assignments for %s@%s: %w", schemaName, schemaVersion, err)
	}
	return out, nil
}

func fieldPolicyConflictTarget(t compliance.ScopeType) (string, error) {
	switch t {
	case compliance.ScopeFleet:
		return "(schema_name, schema_version) WHERE scope_type = 'fleet'", nil
	case compliance.ScopeVessel:
		return "(schema_name, schema_version, vessel_id) WHERE scope_type = 'vessel'", nil
	case compliance.ScopeGroup:
		return "(schema_name, schema_version, group_tag) WHERE scope_type = 'group'", nil
	default:
		return "", fmt.Errorf("store: unknown scope type %q", t)
	}
}

func scanFieldPolicyRow(row rowScanner, scope compliance.Scope, schemaName, schemaVersion string) (*fieldpolicy.SchemaFieldPolicy, error) {
	var (
		policyJSON, prefillJSON, eventsJSON []byte
		updatedAt                           sql.NullTime
	)
	err := row.Scan(&policyJSON, &prefillJSON, &eventsJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		p := fieldpolicy.SchemaFieldPolicy{
			Scope: scope, SchemaName: schemaName, SchemaVersion: schemaVersion,
			Policy: validation.FieldPolicy{}, Events: validation.FieldEvents{}, Prefill: map[string]fieldpolicy.PrefillClass{},
		}
		return &p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return parseFieldPolicyRow(scope, schemaName, schemaVersion, policyJSON, prefillJSON, eventsJSON, updatedAt)
}

func parseFieldPolicyRow(scope compliance.Scope, schemaName, schemaVersion string, policyJSON, prefillJSON, eventsJSON []byte, updatedAt sql.NullTime) (*fieldpolicy.SchemaFieldPolicy, error) {
	var policyStrs map[string]string
	if err := json.Unmarshal(policyJSON, &policyStrs); err != nil {
		return nil, fmt.Errorf("unmarshal policy for %s@%s: %w", schemaName, schemaVersion, err)
	}
	var prefillStrs map[string]string
	if err := json.Unmarshal(prefillJSON, &prefillStrs); err != nil {
		return nil, fmt.Errorf("unmarshal prefill for %s@%s: %w", schemaName, schemaVersion, err)
	}
	var eventLists map[string][]string
	if err := json.Unmarshal(eventsJSON, &eventLists); err != nil {
		return nil, fmt.Errorf("unmarshal events for %s@%s: %w", schemaName, schemaVersion, err)
	}
	p := &fieldpolicy.SchemaFieldPolicy{
		Scope: scope, SchemaName: schemaName, SchemaVersion: schemaVersion,
		Policy: validation.FieldPolicy{}, Events: validation.FieldEvents{}, Prefill: map[string]fieldpolicy.PrefillClass{},
	}
	for name, state := range policyStrs {
		p.Policy[name] = validation.FieldPolicyState(state)
	}
	maps.Copy(p.Events, eventLists)
	for name, class := range prefillStrs {
		p.Prefill[name] = fieldpolicy.PrefillClass(class)
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	}
	return p, nil
}

func stringifyFieldPolicyStates(p validation.FieldPolicy) map[string]string {
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = string(v)
	}
	return out
}

func stringifyPrefillClasses(p map[string]fieldpolicy.PrefillClass) map[string]string {
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = string(v)
	}
	return out
}
