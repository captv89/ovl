// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/pkg/validation"
)

func deleteTestConfigBundle(t *testing.T, st *Store, id string) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, `DELETE FROM bundle_assignments WHERE bundle_id = $1`, id); err != nil {
		t.Errorf("cleanup: delete test bundle assignments for bundle %s: %v", id, err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM config_bundles WHERE id = $1`, id); err != nil {
		t.Errorf("cleanup: delete test config bundle %s: %v", id, err)
	}
}

func newTestBundle(t *testing.T, publishedBy string) *configbundle.ConfigBundle {
	t.Helper()
	fp, err := fieldpolicy.New(compliance.FleetScope(), "log-abstract", "3.13")
	if err != nil {
		t.Fatalf("fieldpolicy.New: %v", err)
	}
	if err := fp.SetPolicy("Some_Field", validation.FieldCompanyMandatory, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	profile, err := compliance.NewProfileAssignment(compliance.FleetScope(), []validation.RegulatoryProfile{validation.ProfileMRV})
	if err != nil {
		t.Fatalf("NewProfileAssignment: %v", err)
	}
	cadence, err := compliance.NewCadenceRule(compliance.FleetScope(), 24, 12)
	if err != nil {
		t.Fatalf("NewCadenceRule: %v", err)
	}
	ruleSev, err := compliance.NewRuleSeverityAssignment(compliance.FleetScope())
	if err != nil {
		t.Fatalf("NewRuleSeverityAssignment: %v", err)
	}
	if err := ruleSev.SetSeverity(validation.RuleImpliedSpeed, validation.SeverityWarning); err != nil {
		t.Fatalf("SetSeverity: %v", err)
	}

	composed := configbundle.Compose(
		[]configbundle.SchemaVersionRef{{SchemaName: "log-abstract", Version: "3.13", ID: "sv-1"}},
		[]*fieldpolicy.SchemaFieldPolicy{fp},
		[]*compliance.ProfileAssignment{profile},
		[]*compliance.CadenceRule{cadence},
		[]*compliance.RuleSeverityAssignment{ruleSev},
		[]string{"master", "chiefOfficer"},
	)
	b, err := configbundle.Publish(composed, "Test Bundle", publishedBy)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return b
}

func TestStore_CreateAndGetConfigBundle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	b := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, b); err != nil {
		t.Fatalf("CreateConfigBundle: %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b.ID) })

	got, err := st.GetConfigBundle(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetConfigBundle: %v", err)
	}
	if got.Label != b.Label || got.PublishedBy != b.PublishedBy {
		t.Errorf("Label/PublishedBy = %q/%q, want %q/%q", got.Label, got.PublishedBy, b.Label, b.PublishedBy)
	}
	if len(got.SchemaVersions) != 1 || got.SchemaVersions[0].SchemaName != "log-abstract" {
		t.Errorf("SchemaVersions = %+v, want one log-abstract ref", got.SchemaVersions)
	}
	if len(got.FieldPolicies) != 1 || got.FieldPolicies[0].Policy["Some_Field"] != validation.FieldCompanyMandatory {
		t.Errorf("FieldPolicies round-trip mismatch: %+v", got.FieldPolicies)
	}
	if len(got.RegulatoryProfiles) != 1 || got.RegulatoryProfiles[0].Profiles[0] != validation.ProfileMRV {
		t.Errorf("RegulatoryProfiles round-trip mismatch: %+v", got.RegulatoryProfiles)
	}
	if len(got.CadenceRules) != 1 || got.CadenceRules[0].MaxGapHours != 12 {
		t.Errorf("CadenceRules round-trip mismatch: %+v", got.CadenceRules)
	}
	if len(got.RuleSeverities) != 1 || got.RuleSeverities[0].Severities[validation.RuleImpliedSpeed] != validation.SeverityWarning {
		t.Errorf("RuleSeverities round-trip mismatch: %+v", got.RuleSeverities)
	}
	if len(got.DefaultRoleNames) != 2 {
		t.Errorf("DefaultRoleNames = %v, want 2 entries", got.DefaultRoleNames)
	}
}

func TestStore_GetConfigBundle_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetConfigBundle(context.Background(), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetConfigBundle(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_GetConfigBundleCursor(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	b1 := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, b1); err != nil {
		t.Fatalf("CreateConfigBundle (b1): %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b1.ID) })

	cursor1, err := st.GetConfigBundleCursor(ctx, b1.ID)
	if err != nil {
		t.Fatalf("GetConfigBundleCursor (b1): %v", err)
	}
	if cursor1 <= 0 {
		t.Errorf("cursor1 = %d, want > 0", cursor1)
	}

	b2 := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, b2); err != nil {
		t.Fatalf("CreateConfigBundle (b2): %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b2.ID) })

	cursor2, err := st.GetConfigBundleCursor(ctx, b2.ID)
	if err != nil {
		t.Fatalf("GetConfigBundleCursor (b2): %v", err)
	}
	if cursor2 <= cursor1 {
		t.Errorf("cursor2 = %d, want greater than cursor1 = %d", cursor2, cursor1)
	}
}

func TestStore_GetConfigBundleCursor_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetConfigBundleCursor(context.Background(), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetConfigBundleCursor(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListConfigBundles_NewestFirst(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	older := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, older); err != nil {
		t.Fatalf("CreateConfigBundle (older): %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, older.ID) })

	newer := newTestBundle(t, "user-2")
	newer.PublishedAt = older.PublishedAt.Add(1e9) // 1s, comfortably past Postgres's microsecond precision
	if err := st.CreateConfigBundle(ctx, newer); err != nil {
		t.Fatalf("CreateConfigBundle (newer): %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, newer.ID) })

	list, err := st.ListConfigBundles(ctx)
	if err != nil {
		t.Fatalf("ListConfigBundles: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("ListConfigBundles returned %d bundles, want at least 2", len(list))
	}
	var newerIdx, olderIdx = -1, -1
	for i, b := range list {
		if b.ID == newer.ID {
			newerIdx = i
		}
		if b.ID == older.ID {
			olderIdx = i
		}
	}
	if newerIdx == -1 || olderIdx == -1 {
		t.Fatalf("ListConfigBundles missing created bundles: newerIdx=%d olderIdx=%d", newerIdx, olderIdx)
	}
	if newerIdx > olderIdx {
		t.Errorf("ListConfigBundles order: newer bundle at index %d, older at %d, want newest-first", newerIdx, olderIdx)
	}
}

func TestStore_SaveAndListBundleAssignments(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 40, []string{"Fleet A"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })

	b := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, b); err != nil {
		t.Fatalf("CreateConfigBundle: %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b.ID) })

	vesselScope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	assignment, err := configbundle.NewBundleAssignment(vesselScope, b.ID)
	if err != nil {
		t.Fatalf("NewBundleAssignment: %v", err)
	}
	if err := st.SaveBundleAssignment(ctx, assignment); err != nil {
		t.Fatalf("SaveBundleAssignment: %v", err)
	}

	list, err := st.ListBundleAssignments(ctx)
	if err != nil {
		t.Fatalf("ListBundleAssignments: %v", err)
	}
	var found bool
	for _, a := range list {
		if a.Scope.Type == compliance.ScopeVessel && a.Scope.Key == v.ID {
			found = true
			if a.BundleID != b.ID {
				t.Errorf("BundleID = %q, want %q", a.BundleID, b.ID)
			}
		}
	}
	if !found {
		t.Error("ListBundleAssignments missing the saved vessel assignment")
	}

	// Re-saving the same scope updates in place (partial unique index).
	b2 := newTestBundle(t, "user-2")
	if err := st.CreateConfigBundle(ctx, b2); err != nil {
		t.Fatalf("CreateConfigBundle (b2): %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b2.ID) })
	reassignment, err := configbundle.NewBundleAssignment(vesselScope, b2.ID)
	if err != nil {
		t.Fatalf("NewBundleAssignment (reassignment): %v", err)
	}
	if err := st.SaveBundleAssignment(ctx, reassignment); err != nil {
		t.Fatalf("SaveBundleAssignment (reassignment): %v", err)
	}
	list2, err := st.ListBundleAssignments(ctx)
	if err != nil {
		t.Fatalf("ListBundleAssignments (after reassign): %v", err)
	}
	var vesselRows int
	for _, a := range list2 {
		if a.Scope.Type == compliance.ScopeVessel && a.Scope.Key == v.ID {
			vesselRows++
			if a.BundleID != b2.ID {
				t.Errorf("BundleID after reassign = %q, want %q", a.BundleID, b2.ID)
			}
		}
	}
	if vesselRows != 1 {
		t.Errorf("vessel-scope rows after re-save = %d, want 1", vesselRows)
	}
}

// TestStore_SaveAndListBundleAssignment_FleetScope covers the 2026-07-15
// addition (migration 00024): fleet-wide is now a real bundle-assignment
// scope, same partial-unique-index upsert-in-place shape the vessel/group
// scopes already have (TestStore_SaveAndListBundleAssignments).
func TestStore_SaveAndListBundleAssignment_FleetScope(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	b := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, b); err != nil {
		t.Fatalf("CreateConfigBundle: %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b.ID) })
	t.Cleanup(func() {
		if _, err := st.db.ExecContext(context.Background(), `DELETE FROM bundle_assignments WHERE scope_type = 'fleet'`); err != nil {
			t.Errorf("cleanup: delete fleet-scope bundle assignment: %v", err)
		}
	})

	assignment, err := configbundle.NewBundleAssignment(compliance.FleetScope(), b.ID)
	if err != nil {
		t.Fatalf("NewBundleAssignment: %v", err)
	}
	if err := st.SaveBundleAssignment(ctx, assignment); err != nil {
		t.Fatalf("SaveBundleAssignment(fleet scope): %v", err)
	}

	list, err := st.ListBundleAssignments(ctx)
	if err != nil {
		t.Fatalf("ListBundleAssignments: %v", err)
	}
	var found bool
	for _, a := range list {
		if a.Scope.Type == compliance.ScopeFleet {
			found = true
			if a.BundleID != b.ID {
				t.Errorf("BundleID = %q, want %q", a.BundleID, b.ID)
			}
		}
	}
	if !found {
		t.Error("ListBundleAssignments missing the saved fleet-scope assignment")
	}
}

func TestStore_BundleAssignment_VesselCascadeDelete(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 41, nil)
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}

	b := newTestBundle(t, "user-1")
	if err := st.CreateConfigBundle(ctx, b); err != nil {
		t.Fatalf("CreateConfigBundle: %v", err)
	}
	t.Cleanup(func() { deleteTestConfigBundle(t, st, b.ID) })

	scope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	assignment, err := configbundle.NewBundleAssignment(scope, b.ID)
	if err != nil {
		t.Fatalf("NewBundleAssignment: %v", err)
	}
	if err := st.SaveBundleAssignment(ctx, assignment); err != nil {
		t.Fatalf("SaveBundleAssignment: %v", err)
	}

	deleteTestVessel(t, st, v.ID) // deleting the vessel should cascade-delete its assignment

	list, err := st.ListBundleAssignments(ctx)
	if err != nil {
		t.Fatalf("ListBundleAssignments: %v", err)
	}
	for _, a := range list {
		if a.Scope.Type == compliance.ScopeVessel && a.Scope.Key == v.ID {
			t.Errorf("bundle assignment for deleted vessel %s still present", v.ID)
		}
	}
}
