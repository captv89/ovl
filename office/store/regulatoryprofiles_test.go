// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

func deleteTestProfileAssignment(t *testing.T, st *Store, scopeType string, key string) {
	t.Helper()
	ctx := context.Background()
	var err error
	switch scopeType {
	case "fleet":
		_, err = st.db.ExecContext(ctx, `DELETE FROM regulatory_profile_assignments WHERE scope_type = 'fleet'`)
	case "vessel":
		_, err = st.db.ExecContext(ctx, `DELETE FROM regulatory_profile_assignments WHERE scope_type = 'vessel' AND vessel_id = $1`, key)
	case "group":
		_, err = st.db.ExecContext(ctx, `DELETE FROM regulatory_profile_assignments WHERE scope_type = 'group' AND group_tag = $1`, key)
	}
	if err != nil {
		t.Errorf("cleanup: delete test profile assignment (%s/%s): %v", scopeType, key, err)
	}
}

func TestStore_SaveAndListProfileAssignments(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 20, []string{"Fleet A"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })

	fleet, err := compliance.NewProfileAssignment(compliance.FleetScope(), []validation.RegulatoryProfile{validation.ProfileMRV})
	if err != nil {
		t.Fatalf("NewProfileAssignment(fleet): %v", err)
	}
	vesselScope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	vesselAssignment, err := compliance.NewProfileAssignment(vesselScope, []validation.RegulatoryProfile{validation.ProfileCII})
	if err != nil {
		t.Fatalf("NewProfileAssignment(vessel): %v", err)
	}

	if err := st.SaveProfileAssignment(ctx, fleet); err != nil {
		t.Fatalf("SaveProfileAssignment(fleet): %v", err)
	}
	t.Cleanup(func() { deleteTestProfileAssignment(t, st, "fleet", "") })
	if err := st.SaveProfileAssignment(ctx, vesselAssignment); err != nil {
		t.Fatalf("SaveProfileAssignment(vessel): %v", err)
	}

	list, err := st.ListProfileAssignments(ctx)
	if err != nil {
		t.Fatalf("ListProfileAssignments: %v", err)
	}
	got := compliance.EffectiveProfiles(list, v.ID, v.Groups)
	if len(got) != 2 {
		t.Fatalf("EffectiveProfiles() = %v, want 2 entries (MRV fleet-wide + CII vessel-specific)", got)
	}

	// Re-saving the same scope updates in place (partial unique index),
	// not accumulate a second row.
	updated, err := compliance.NewProfileAssignment(compliance.FleetScope(), []validation.RegulatoryProfile{validation.ProfileMRV, validation.ProfileDCS})
	if err != nil {
		t.Fatalf("NewProfileAssignment(fleet updated): %v", err)
	}
	if err := st.SaveProfileAssignment(ctx, updated); err != nil {
		t.Fatalf("SaveProfileAssignment(fleet updated): %v", err)
	}
	list2, err := st.ListProfileAssignments(ctx)
	if err != nil {
		t.Fatalf("ListProfileAssignments (after update): %v", err)
	}
	var fleetRows int
	for _, a := range list2 {
		if a.Scope.Type == compliance.ScopeFleet {
			fleetRows++
		}
	}
	if fleetRows != 1 {
		t.Errorf("fleet-scope rows after re-save = %d, want 1", fleetRows)
	}
}

func TestStore_ProfileAssignment_VesselCascadeDelete(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 21, nil)
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}

	scope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	assignment, err := compliance.NewProfileAssignment(scope, []validation.RegulatoryProfile{validation.ProfileDCS})
	if err != nil {
		t.Fatalf("NewProfileAssignment: %v", err)
	}
	if err := st.SaveProfileAssignment(ctx, assignment); err != nil {
		t.Fatalf("SaveProfileAssignment: %v", err)
	}

	deleteTestVessel(t, st, v.ID) // deleting the vessel should cascade-delete its assignment

	list, err := st.ListProfileAssignments(ctx)
	if err != nil {
		t.Fatalf("ListProfileAssignments: %v", err)
	}
	for _, a := range list {
		if a.Scope.Type == compliance.ScopeVessel && a.Scope.Key == v.ID {
			t.Errorf("profile assignment for deleted vessel %s still present", v.ID)
		}
	}
}
