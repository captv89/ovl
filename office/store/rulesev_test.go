// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

func deleteTestRuleSeverityAssignment(t *testing.T, st *Store, scopeType string, key string) {
	t.Helper()
	ctx := context.Background()
	var err error
	switch scopeType {
	case "fleet":
		_, err = st.db.ExecContext(ctx, `DELETE FROM rule_severity_assignments WHERE scope_type = 'fleet'`)
	case "vessel":
		_, err = st.db.ExecContext(ctx, `DELETE FROM rule_severity_assignments WHERE scope_type = 'vessel' AND vessel_id = $1`, key)
	case "group":
		_, err = st.db.ExecContext(ctx, `DELETE FROM rule_severity_assignments WHERE scope_type = 'group' AND group_tag = $1`, key)
	}
	if err != nil {
		t.Errorf("cleanup: delete test rule severity assignment (%s/%s): %v", scopeType, key, err)
	}
}

func TestStore_SaveAndListRuleSeverityAssignments(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 30, []string{"Fleet A"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })

	fleet, err := compliance.NewRuleSeverityAssignment(compliance.FleetScope())
	if err != nil {
		t.Fatalf("NewRuleSeverityAssignment(fleet): %v", err)
	}
	if err := fleet.SetSeverity(validation.RuleImpliedSpeed, validation.SeverityWarning); err != nil {
		t.Fatalf("SetSeverity: %v", err)
	}

	vesselScope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	vesselAssignment, err := compliance.NewRuleSeverityAssignment(vesselScope)
	if err != nil {
		t.Fatalf("NewRuleSeverityAssignment(vessel): %v", err)
	}
	if err := vesselAssignment.SetSeverity(validation.RuleTimeChain, validation.SeverityError); err != nil {
		t.Fatalf("SetSeverity: %v", err)
	}

	if err := st.SaveRuleSeverityAssignment(ctx, fleet); err != nil {
		t.Fatalf("SaveRuleSeverityAssignment(fleet): %v", err)
	}
	t.Cleanup(func() { deleteTestRuleSeverityAssignment(t, st, "fleet", "") })
	if err := st.SaveRuleSeverityAssignment(ctx, vesselAssignment); err != nil {
		t.Fatalf("SaveRuleSeverityAssignment(vessel): %v", err)
	}

	list, err := st.ListRuleSeverityAssignments(ctx)
	if err != nil {
		t.Fatalf("ListRuleSeverityAssignments: %v", err)
	}
	got := compliance.EffectiveSeverities(list, v.ID, v.Groups)
	if got[validation.RuleImpliedSpeed] != validation.SeverityWarning {
		t.Errorf("RuleImpliedSpeed = %q, want warning (fleet-wide)", got[validation.RuleImpliedSpeed])
	}
	if got[validation.RuleTimeChain] != validation.SeverityError {
		t.Errorf("RuleTimeChain = %q, want error (vessel-specific)", got[validation.RuleTimeChain])
	}

	// Re-saving the same scope updates in place (partial unique index),
	// not accumulate a second row.
	updated, err := compliance.NewRuleSeverityAssignment(compliance.FleetScope())
	if err != nil {
		t.Fatalf("NewRuleSeverityAssignment(fleet updated): %v", err)
	}
	if err := updated.SetSeverity(validation.RuleImpliedSpeed, validation.SeverityInfo); err != nil {
		t.Fatalf("SetSeverity: %v", err)
	}
	if err := st.SaveRuleSeverityAssignment(ctx, updated); err != nil {
		t.Fatalf("SaveRuleSeverityAssignment(fleet updated): %v", err)
	}
	list2, err := st.ListRuleSeverityAssignments(ctx)
	if err != nil {
		t.Fatalf("ListRuleSeverityAssignments (after update): %v", err)
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

func TestStore_RuleSeverityAssignment_VesselCascadeDelete(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 31, nil)
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}

	scope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	assignment, err := compliance.NewRuleSeverityAssignment(scope)
	if err != nil {
		t.Fatalf("NewRuleSeverityAssignment: %v", err)
	}
	if err := assignment.SetSeverity(validation.RuleROBContinuity, validation.SeverityError); err != nil {
		t.Fatalf("SetSeverity: %v", err)
	}
	if err := st.SaveRuleSeverityAssignment(ctx, assignment); err != nil {
		t.Fatalf("SaveRuleSeverityAssignment: %v", err)
	}

	deleteTestVessel(t, st, v.ID) // deleting the vessel should cascade-delete its assignment

	list, err := st.ListRuleSeverityAssignments(ctx)
	if err != nil {
		t.Fatalf("ListRuleSeverityAssignments: %v", err)
	}
	for _, a := range list {
		if a.Scope.Type == compliance.ScopeVessel && a.Scope.Key == v.ID {
			t.Errorf("rule severity assignment for deleted vessel %s still present", v.ID)
		}
	}
}
