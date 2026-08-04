// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/fieldpolicy"
)

// CreateConfigBundle inserts a newly published, immutable config bundle.
// There is no UpdateConfigBundle — see migration 00010's comment.
func (s *Store) CreateConfigBundle(ctx context.Context, b *configbundle.ConfigBundle) error {
	schemaVersionsJSON, err := json.Marshal(b.SchemaVersions)
	if err != nil {
		return fmt.Errorf("marshal schema versions for bundle %s: %w", b.ID, err)
	}
	fieldPoliciesJSON, err := json.Marshal(b.FieldPolicies)
	if err != nil {
		return fmt.Errorf("marshal field policies for bundle %s: %w", b.ID, err)
	}
	profilesJSON, err := json.Marshal(b.RegulatoryProfiles)
	if err != nil {
		return fmt.Errorf("marshal regulatory profiles for bundle %s: %w", b.ID, err)
	}
	cadenceJSON, err := json.Marshal(b.CadenceRules)
	if err != nil {
		return fmt.Errorf("marshal cadence rules for bundle %s: %w", b.ID, err)
	}
	ruleSeveritiesJSON, err := json.Marshal(b.RuleSeverities)
	if err != nil {
		return fmt.Errorf("marshal rule severities for bundle %s: %w", b.ID, err)
	}
	roleNamesJSON, err := json.Marshal(b.DefaultRoleNames)
	if err != nil {
		return fmt.Errorf("marshal default role names for bundle %s: %w", b.ID, err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO config_bundles (id, label, schema_versions, field_policies, regulatory_profiles, cadence_rules, rule_severities, default_role_names, published_at, published_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, b.ID, b.Label, string(schemaVersionsJSON), string(fieldPoliciesJSON), string(profilesJSON),
		string(cadenceJSON), string(ruleSeveritiesJSON), string(roleNamesJSON), b.PublishedAt, b.PublishedBy)
	if err != nil {
		return fmt.Errorf("create config bundle %s: %w", b.ID, err)
	}
	return nil
}

// GetConfigBundle returns one published bundle by ID.
func (s *Store) GetConfigBundle(ctx context.Context, id string) (*configbundle.ConfigBundle, error) {
	row := s.db.QueryRowContext(ctx, configBundleColumns+`FROM config_bundles WHERE id = $1`, id)
	return scanConfigBundle(row)
}

// GetConfigBundleCursor returns id's pull cursor — PullInbox's "is the
// vessel's currently-assigned bundle newer than what it already has"
// comparison (architecture 6.5), distinct from PublishedAt (see
// migration 00016's comment on why a dedicated monotonic column exists
// at all). A separate lookup from GetConfigBundle rather than adding
// cursor to configBundleColumns: every other GetConfigBundle/
// ListConfigBundles caller (B7's UI, resolveBundleAssignment's preview)
// has no use for it, so this keeps their existing scan shape unchanged.
func (s *Store) GetConfigBundleCursor(ctx context.Context, id string) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx, `SELECT cursor FROM config_bundles WHERE id = $1`, id).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get config bundle cursor for %s: %w", id, err)
	}
	return cursor, nil
}

// ListConfigBundles returns every published bundle, newest first —
// design handoff B7's bundle history list.
func (s *Store) ListConfigBundles(ctx context.Context) ([]*configbundle.ConfigBundle, error) {
	rows, err := s.db.QueryContext(ctx, configBundleColumns+`FROM config_bundles ORDER BY published_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list config bundles: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*configbundle.ConfigBundle
	for rows.Next() {
		b, err := scanConfigBundle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate config bundles: %w", err)
	}
	return out, nil
}

const configBundleColumns = `SELECT id, label, schema_versions, field_policies, regulatory_profiles, cadence_rules, rule_severities, default_role_names, published_at, published_by `

func scanConfigBundle(row rowScanner) (*configbundle.ConfigBundle, error) {
	var (
		b                                                                                               configbundle.ConfigBundle
		schemaVersionsJSON, fieldPoliciesJSON, profilesJSON, cadenceJSON, ruleSeveritiesJSON, rolesJSON []byte
	)
	err := row.Scan(&b.ID, &b.Label, &schemaVersionsJSON, &fieldPoliciesJSON, &profilesJSON,
		&cadenceJSON, &ruleSeveritiesJSON, &rolesJSON, &b.PublishedAt, &b.PublishedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan config bundle: %w", err)
	}
	if err := json.Unmarshal(schemaVersionsJSON, &b.SchemaVersions); err != nil {
		return nil, fmt.Errorf("unmarshal schema versions for bundle %s: %w", b.ID, err)
	}
	var fieldPolicies []*fieldpolicy.SchemaFieldPolicy
	if err := json.Unmarshal(fieldPoliciesJSON, &fieldPolicies); err != nil {
		return nil, fmt.Errorf("unmarshal field policies for bundle %s: %w", b.ID, err)
	}
	b.FieldPolicies = fieldPolicies
	var profiles []*compliance.ProfileAssignment
	if err := json.Unmarshal(profilesJSON, &profiles); err != nil {
		return nil, fmt.Errorf("unmarshal regulatory profiles for bundle %s: %w", b.ID, err)
	}
	b.RegulatoryProfiles = profiles
	var cadence []*compliance.CadenceRule
	if err := json.Unmarshal(cadenceJSON, &cadence); err != nil {
		return nil, fmt.Errorf("unmarshal cadence rules for bundle %s: %w", b.ID, err)
	}
	b.CadenceRules = cadence
	var ruleSeverities []*compliance.RuleSeverityAssignment
	if err := json.Unmarshal(ruleSeveritiesJSON, &ruleSeverities); err != nil {
		return nil, fmt.Errorf("unmarshal rule severities for bundle %s: %w", b.ID, err)
	}
	b.RuleSeverities = ruleSeverities
	if err := json.Unmarshal(rolesJSON, &b.DefaultRoleNames); err != nil {
		return nil, fmt.Errorf("unmarshal default role names for bundle %s: %w", b.ID, err)
	}
	return &b, nil
}

// SaveBundleAssignment upserts which bundle applies to a's scope (one row
// per fleet/group/vessel scope — see migration 00024, which added the
// 'fleet' scope_type migration 00011 originally excluded).
func (s *Store) SaveBundleAssignment(ctx context.Context, a *configbundle.BundleAssignment) error {
	if err := a.Scope.Validate(); err != nil {
		return fmt.Errorf("save bundle assignment: %w", err)
	}
	vesselID, groupTag := scopeColumns(a.Scope)
	conflictTarget, err := bundleAssignmentConflictTarget(a.Scope.Type)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO bundle_assignments (scope_type, vessel_id, group_tag, bundle_id, assigned_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT %s DO UPDATE SET bundle_id = EXCLUDED.bundle_id, assigned_at = EXCLUDED.assigned_at
	`, conflictTarget), string(a.Scope.Type), vesselID, groupTag, a.BundleID, a.AssignedAt)
	if err != nil {
		return fmt.Errorf("save bundle assignment for scope %s/%s: %w", a.Scope.Type, a.Scope.Key, err)
	}
	return nil
}

// ListBundleAssignments returns every stored bundle assignment, across
// all vessel/group scopes.
func (s *Store) ListBundleAssignments(ctx context.Context) ([]*configbundle.BundleAssignment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope_type, vessel_id, group_tag, bundle_id, assigned_at FROM bundle_assignments
	`)
	if err != nil {
		return nil, fmt.Errorf("list bundle assignments: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []*configbundle.BundleAssignment
	for rows.Next() {
		var (
			scopeType          string
			vesselID, groupTag sql.NullString
			bundleID           string
			assignedAt         sql.NullTime
		)
		if err := rows.Scan(&scopeType, &vesselID, &groupTag, &bundleID, &assignedAt); err != nil {
			return nil, fmt.Errorf("scan bundle assignment: %w", err)
		}
		scope, err := scopeFromColumns(scopeType, vesselID, groupTag)
		if err != nil {
			return nil, err
		}
		a := &configbundle.BundleAssignment{Scope: scope, BundleID: bundleID}
		if assignedAt.Valid {
			a.AssignedAt = assignedAt.Time
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bundle assignments: %w", err)
	}
	return out, nil
}

func bundleAssignmentConflictTarget(t compliance.ScopeType) (string, error) {
	switch t {
	case compliance.ScopeVessel:
		return "(vessel_id) WHERE scope_type = 'vessel'", nil
	case compliance.ScopeGroup:
		return "(group_tag) WHERE scope_type = 'group'", nil
	case compliance.ScopeFleet:
		return "(scope_type) WHERE scope_type = 'fleet'", nil
	default:
		return "", fmt.Errorf("store: unknown or unsupported bundle assignment scope type %q", t)
	}
}
