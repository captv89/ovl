// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/captv89/ovl/office/compliance"
)

func deleteTestCadenceRule(t *testing.T, st *Store, scopeType, key string) {
	t.Helper()
	ctx := context.Background()
	var err error
	switch scopeType {
	case "fleet":
		_, err = st.db.ExecContext(ctx, `DELETE FROM cadence_rules WHERE scope_type = 'fleet'`)
	case "vessel":
		_, err = st.db.ExecContext(ctx, `DELETE FROM cadence_rules WHERE scope_type = 'vessel' AND vessel_id = $1`, key)
	case "group":
		_, err = st.db.ExecContext(ctx, `DELETE FROM cadence_rules WHERE scope_type = 'group' AND group_tag = $1`, key)
	}
	if err != nil {
		t.Errorf("cleanup: delete test cadence rule (%s/%s): %v", scopeType, key, err)
	}
}

func TestStore_SaveAndListCadenceRules(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 22, []string{"Fleet A"})
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })

	fleet, err := compliance.NewCadenceRule(compliance.FleetScope(), compliance.DefaultMinReportIntervalHours, compliance.DefaultMaxGapHours)
	if err != nil {
		t.Fatalf("NewCadenceRule(fleet): %v", err)
	}
	vesselScope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	vesselRule, err := compliance.NewCadenceRule(vesselScope, 6, 3)
	if err != nil {
		t.Fatalf("NewCadenceRule(vessel): %v", err)
	}

	if err := st.SaveCadenceRule(ctx, fleet); err != nil {
		t.Fatalf("SaveCadenceRule(fleet): %v", err)
	}
	t.Cleanup(func() { deleteTestCadenceRule(t, st, "fleet", "") })
	if err := st.SaveCadenceRule(ctx, vesselRule); err != nil {
		t.Fatalf("SaveCadenceRule(vessel): %v", err)
	}

	list, err := st.ListCadenceRules(ctx)
	if err != nil {
		t.Fatalf("ListCadenceRules: %v", err)
	}
	got := compliance.EffectiveCadence(list, v.ID, v.Groups)
	if got.MinReportIntervalHours != 6 || got.MaxGapHours != 3 {
		t.Errorf("EffectiveCadence() = %+v, want the vessel-specific rule (6/3)", got)
	}

	other := compliance.EffectiveCadence(list, "some-other-vessel", nil)
	if other.MinReportIntervalHours != compliance.DefaultMinReportIntervalHours || other.MaxGapHours != compliance.DefaultMaxGapHours {
		t.Errorf("EffectiveCadence(other vessel) = %+v, want fleet rule (%v/%v)", other,
			compliance.DefaultMinReportIntervalHours, compliance.DefaultMaxGapHours)
	}
}

func TestStore_CadenceRule_VesselCascadeDelete(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	v := newTestVessel(t, 23, nil)
	if err := st.CreateVessel(ctx, v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}

	scope, err := compliance.VesselScope(v.ID)
	if err != nil {
		t.Fatalf("VesselScope: %v", err)
	}
	rule, err := compliance.NewCadenceRule(scope, 6, 3)
	if err != nil {
		t.Fatalf("NewCadenceRule: %v", err)
	}
	if err := st.SaveCadenceRule(ctx, rule); err != nil {
		t.Fatalf("SaveCadenceRule: %v", err)
	}

	deleteTestVessel(t, st, v.ID) // deleting the vessel should cascade-delete its cadence rule

	list, err := st.ListCadenceRules(ctx)
	if err != nil {
		t.Fatalf("ListCadenceRules: %v", err)
	}
	for _, r := range list {
		if r.Scope.Type == compliance.ScopeVessel && r.Scope.Key == v.ID {
			t.Errorf("cadence rule for deleted vessel %s still present", v.ID)
		}
	}
}
