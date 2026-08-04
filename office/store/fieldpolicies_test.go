// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/pkg/validation"
)

func deleteTestFieldPolicy(t *testing.T, st *Store, schemaName, schemaVersion string) {
	t.Helper()
	if _, err := st.db.ExecContext(context.Background(),
		`DELETE FROM field_policy_assignments WHERE schema_name = $1 AND schema_version = $2`, schemaName, schemaVersion); err != nil {
		t.Errorf("cleanup: delete test field policies for %s@%s: %v", schemaName, schemaVersion, err)
	}
}

func TestStore_LoadFieldPolicy_NoRowsIsEmptyNotError(t *testing.T) {
	st := openTestStore(t)

	p, err := st.LoadFieldPolicy(context.Background(), compliance.FleetScope(), "log-abstract", t.Name())
	if err != nil {
		t.Fatalf("LoadFieldPolicy: %v", err)
	}
	if len(p.Policy) != 0 || len(p.Prefill) != 0 {
		t.Errorf("LoadFieldPolicy(never saved) = %+v, want empty maps", p)
	}
}

func TestStore_SaveAndLoadFieldPolicy(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	schemaVersion := t.Name()

	p, err := fieldpolicy.New(compliance.FleetScope(), "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("fieldpolicy.New: %v", err)
	}
	if err := p.SetPolicy("Wind_Force_Kn", validation.FieldRecommended, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if err := p.SetPrefillClass("Wind_Force_Kn", fieldpolicy.PrefillGhost); err != nil {
		t.Fatalf("SetPrefillClass: %v", err)
	}
	if err := p.SetPolicy("HFO_ROB", validation.FieldOptional, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if err := p.SetPrefillClass("HFO_ROB", fieldpolicy.PrefillComputed); err != nil {
		t.Fatalf("SetPrefillClass: %v", err)
	}

	if err := st.SaveFieldPolicy(ctx, p); err != nil {
		t.Fatalf("SaveFieldPolicy: %v", err)
	}
	t.Cleanup(func() { deleteTestFieldPolicy(t, st, "log-abstract", schemaVersion) })

	got, err := st.LoadFieldPolicy(ctx, compliance.FleetScope(), "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("LoadFieldPolicy: %v", err)
	}
	if state := got.Policy.StateFor("Wind_Force_Kn", false, ""); state != validation.FieldRecommended {
		t.Errorf("Wind_Force_Kn state = %q, want recommended", state)
	}
	if prefill := got.PrefillFor("Wind_Force_Kn"); prefill != fieldpolicy.PrefillGhost {
		t.Errorf("Wind_Force_Kn prefill = %q, want ghost", prefill)
	}
	if prefill := got.PrefillFor("HFO_ROB"); prefill != fieldpolicy.PrefillComputed {
		t.Errorf("HFO_ROB prefill = %q, want computed", prefill)
	}

	// A second save with a smaller field set fully replaces the first —
	// stale entries for fields no longer present must not linger.
	p2, err := fieldpolicy.New(compliance.FleetScope(), "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("fieldpolicy.New: %v", err)
	}
	if err := p2.SetPolicy("HFO_ROB", validation.FieldHidden, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if err := st.SaveFieldPolicy(ctx, p2); err != nil {
		t.Fatalf("SaveFieldPolicy (replace): %v", err)
	}

	got2, err := st.LoadFieldPolicy(ctx, compliance.FleetScope(), "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("LoadFieldPolicy (after replace): %v", err)
	}
	if state := got2.Policy.StateFor("Wind_Force_Kn", false, ""); state != validation.FieldOptional {
		t.Errorf("Wind_Force_Kn state after replace = %q, want optional (default, entry gone)", state)
	}
	if state := got2.Policy.StateFor("HFO_ROB", false, ""); state != validation.FieldHidden {
		t.Errorf("HFO_ROB state after replace = %q, want hidden", state)
	}
}

// TestStore_FieldPolicy_MultipleScopes covers the real-world case this
// scoping exists for (per the user's own example): a fleet-wide default
// hides a specialty field, while one vessel group's own assignment shows
// it — e.g. DP-related fields shown for a "DP" vessel group but hidden
// fleet-wide for container/bulk carriers.
func TestStore_FieldPolicy_MultipleScopes(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	schemaVersion := t.Name()

	v := newTestVessel(t, 90, []string{"DP"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })
	t.Cleanup(func() { deleteTestFieldPolicy(t, st, "log-abstract", schemaVersion) })

	fleet, err := fieldpolicy.New(compliance.FleetScope(), "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("fieldpolicy.New(fleet): %v", err)
	}
	if err := fleet.SetPolicy("DP_Running_Hours", validation.FieldHidden, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if err := st.SaveFieldPolicy(ctx, fleet); err != nil {
		t.Fatalf("SaveFieldPolicy(fleet): %v", err)
	}

	dpGroupScope, err := compliance.GroupScope("DP")
	if err != nil {
		t.Fatalf("GroupScope: %v", err)
	}
	dpGroup, err := fieldpolicy.New(dpGroupScope, "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("fieldpolicy.New(group): %v", err)
	}
	if err := dpGroup.SetPolicy("DP_Running_Hours", validation.FieldCompanyMandatory, false); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if err := st.SaveFieldPolicy(ctx, dpGroup); err != nil {
		t.Fatalf("SaveFieldPolicy(group): %v", err)
	}

	list, err := st.ListFieldPolicyAssignments(ctx, "log-abstract", schemaVersion)
	if err != nil {
		t.Fatalf("ListFieldPolicyAssignments: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListFieldPolicyAssignments = %d entries, want 2 (fleet + DP group)", len(list))
	}

	effective := fieldpolicy.EffectiveFieldPolicy(list, v.ID, v.Groups, "log-abstract", schemaVersion)
	if got := effective.Policy.StateFor("DP_Running_Hours", false, ""); got != validation.FieldCompanyMandatory {
		t.Errorf("DP vessel effective DP_Running_Hours = %q, want companyMandatory (group override)", got)
	}

	otherVesselGroups := []string{"Container"}
	effectiveOther := fieldpolicy.EffectiveFieldPolicy(list, "some-other-vessel", otherVesselGroups, "log-abstract", schemaVersion)
	if got := effectiveOther.Policy.StateFor("DP_Running_Hours", false, ""); got != validation.FieldHidden {
		t.Errorf("container vessel effective DP_Running_Hours = %q, want hidden (fleet default)", got)
	}
}
