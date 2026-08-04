// SPDX-License-Identifier: AGPL-3.0-only

package configbundle

import (
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/pkg/validation"
)

// TestResolveFor_ScopesFlattenPerVessel proves ResolveFor runs the
// fleet/group/vessel resolution and emits one vessel's effective config as
// a flat configwire.Bundle — the vessel-override wins over fleet, and the
// result carries no scope information (audit 2026-07-22 §1).
func TestResolveFor_ScopesFlattenPerVessel(t *testing.T) {
	const schema, version = "log-abstract", "3.13"
	vesselID := "vessel-A"

	// Fleet-wide field policy hides O_ROB; the vessel-scoped policy shows
	// it as recommended. EffectiveFieldPolicy must pick the vessel's.
	fleetFP, _ := fieldpolicy.New(compliance.FleetScope(), schema, version)
	_ = fleetFP.SetPolicy("O_ROB", validation.FieldHidden, false)
	_ = fleetFP.SetPolicy("Voyage_Number", validation.FieldCompanyMandatory, false)

	vesselScope, _ := compliance.VesselScope(vesselID)
	vesselFP, _ := fieldpolicy.New(vesselScope, schema, version)
	_ = vesselFP.SetPolicy("O_ROB", validation.FieldRecommended, false)

	// Fleet-wide severity override + a vessel-scoped cadence.
	sev, _ := compliance.NewRuleSeverityAssignment(compliance.FleetScope())
	_ = sev.SetSeverity(validation.RuleROBContinuity, validation.SeverityError)

	cad, _ := compliance.NewCadenceRule(vesselScope, 6, 8)

	prof, _ := compliance.NewProfileAssignment(compliance.FleetScope(),
		[]validation.RegulatoryProfile{validation.ProfileMRV, validation.ProfileCII})

	bundle := &ConfigBundle{
		ID:                 "bundle-1",
		SchemaVersions:     []SchemaVersionRef{{SchemaName: schema, Version: version, ID: "sv-1"}},
		FieldPolicies:      []*fieldpolicy.SchemaFieldPolicy{fleetFP, vesselFP},
		RegulatoryProfiles: []*compliance.ProfileAssignment{prof},
		CadenceRules:       []*compliance.CadenceRule{cad},
		RuleSeverities:     []*compliance.RuleSeverityAssignment{sev},
		DefaultRoleNames:   []string{"master"},
	}

	wire := bundle.ResolveFor(vesselID, nil, 42)

	if wire.BundleID != "bundle-1" || wire.VersionNo != 42 || wire.WireVersion != 1 {
		t.Fatalf("wire header = %+v, want bundle-1/42/v1", wire)
	}
	sc := wire.SchemaConfigFor(schema)
	if sc == nil {
		t.Fatalf("no schema config for %q", schema)
	}
	if sc.Policy["O_ROB"] != "recommended" {
		t.Errorf("O_ROB = %q, want recommended (vessel override wins over fleet hidden)", sc.Policy["O_ROB"])
	}
	if sc.Policy["Voyage_Number"] != "companyMandatory" {
		t.Errorf("Voyage_Number = %q, want companyMandatory (inherited from fleet)", sc.Policy["Voyage_Number"])
	}
	if wire.MaxGapHours != 8 {
		t.Errorf("MaxGapHours = %v, want 8 (vessel cadence)", wire.MaxGapHours)
	}
	if wire.RuleSeverities[validation.RuleROBContinuity] != "error" {
		t.Errorf("ROB severity = %q, want error", wire.RuleSeverities[validation.RuleROBContinuity])
	}
	if len(wire.RegulatoryProfiles) != 2 {
		t.Errorf("profiles = %v, want [mrv cii]", wire.RegulatoryProfiles)
	}
}

// TestResolveFor_NoConfigUsesDefaults confirms a bundle with no assignments
// still yields a valid document: empty policy/severities, cadence at the
// architecture 6.3 default, no profiles.
func TestResolveFor_NoConfigUsesDefaults(t *testing.T) {
	bundle := &ConfigBundle{
		ID:             "empty",
		SchemaVersions: []SchemaVersionRef{{SchemaName: "log-abstract", Version: "3.13"}},
	}
	wire := bundle.ResolveFor("vessel-X", nil, 1)
	if wire.MaxGapHours != compliance.DefaultMaxGapHours {
		t.Errorf("MaxGapHours = %v, want default %v", wire.MaxGapHours, compliance.DefaultMaxGapHours)
	}
	if len(wire.RegulatoryProfiles) != 0 {
		t.Errorf("profiles = %v, want none for an unconfigured bundle", wire.RegulatoryProfiles)
	}
	if sc := wire.SchemaConfigFor("log-abstract"); sc == nil || len(sc.Policy) != 0 {
		t.Errorf("schema config = %+v, want present with empty policy", sc)
	}
}
