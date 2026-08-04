// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"testing"

	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/validation"
)

func TestFieldConfigFromBundle_FallsBackWhenNil(t *testing.T) {
	// nil bundle ⇒ the built-in demo stand-in (fieldConfigFor), so an
	// un-synced vessel is unchanged.
	policy, _, _ := fieldConfigFromBundle(nil, "log-abstract")
	want, _ := fieldConfigFor("log-abstract")
	if len(policy) != len(want) || policy["Voyage_Number"] != want["Voyage_Number"] {
		t.Errorf("nil-bundle policy = %v, want demo stand-in %v", policy, want)
	}
}

func TestFieldConfigFromBundle_BundleIsAuthoritative(t *testing.T) {
	b := &configwire.Bundle{
		WireVersion: configwire.WireVersion,
		Schemas: []configwire.SchemaConfig{{
			SchemaName: "log-abstract",
			Version:    "3.13",
			Policy:     map[string]string{"O_ROB": "recommended"},
			Prefill:    map[string]string{"HFO_ROB": "computed"},
			Events:     map[string][]string{"O_ROB": {"Arrival", "Departure"}},
		}},
	}
	policy, events, prefill := fieldConfigFromBundle(b, "log-abstract")
	// The bundle's policy wins outright — the demo stand-in's hidden O_ROB
	// and companyMandatory Voyage_Number must NOT leak through.
	if policy["O_ROB"] != "recommended" {
		t.Errorf("O_ROB = %q, want recommended (from bundle)", policy["O_ROB"])
	}
	if _, ok := policy["Voyage_Number"]; ok {
		t.Errorf("Voyage_Number present (%q) — demo stand-in leaked past an applied bundle", policy["Voyage_Number"])
	}
	if prefill["HFO_ROB"] == nil || prefill["HFO_ROB"].Class != "computed" {
		t.Errorf("HFO_ROB prefill = %+v, want class=computed", prefill["HFO_ROB"])
	}
	// The bundle's per-event narrowing reaches the form: O_ROB is recommended
	// on an Arrival and absent from a Noon, while a field the bundle narrows
	// nothing for stays on every event.
	if !events.AppliesTo("O_ROB", "Arrival") {
		t.Error("O_ROB should apply to Arrival (bundle narrowed it to Arrival/Departure)")
	}
	if events.AppliesTo("O_ROB", "Noon (Position) - Sea passage") {
		t.Error("O_ROB should NOT apply to a Noon at sea report")
	}
	if !events.AppliesTo("HFO_ROB", "Noon (Position) - Sea passage") {
		t.Error("HFO_ROB has no event narrowing and must apply to every event")
	}

	// A schema the bundle carries nothing for resolves to all-defaults
	// (empty), not the demo stand-in.
	other, _, _ := fieldConfigFromBundle(b, "commercial-period")
	if len(other) != 0 {
		t.Errorf("commercial-period policy = %v, want empty (bundle is authoritative)", other)
	}
}

func TestValidationConfigFromBundle_AppliesSeverities(t *testing.T) {
	base := validationConfigFromBundle(nil, "log-abstract")
	if len(base.ROBSeriesList) == 0 {
		t.Fatal("nil-bundle config lost the curated ROB series")
	}

	b := &configwire.Bundle{
		WireVersion:    configwire.WireVersion,
		RuleSeverities: map[string]string{validation.RuleROBContinuity: "error"},
	}
	cfg := validationConfigFromBundle(b, "log-abstract")
	if cfg.Severities[validation.RuleROBContinuity] != validation.SeverityError {
		t.Errorf("ROB severity = %q, want error (from bundle)", cfg.Severities[validation.RuleROBContinuity])
	}
	// Base series/tolerances are preserved — only severities are overlaid.
	if len(cfg.ROBSeriesList) != len(base.ROBSeriesList) {
		t.Errorf("ROB series count changed: %d vs base %d", len(cfg.ROBSeriesList), len(base.ROBSeriesList))
	}
}

func TestProfilesFromBundle(t *testing.T) {
	// nil ⇒ every known profile (pre-config behavior).
	if got := profilesFromBundle(nil); len(got) != len(validation.AllRegulatoryProfiles) {
		t.Errorf("nil-bundle profiles = %v, want all", got)
	}
	// A bundle with an explicit (possibly empty) set is authoritative.
	b := &configwire.Bundle{WireVersion: configwire.WireVersion, RegulatoryProfiles: []string{"mrv"}}
	got := profilesFromBundle(b)
	if len(got) != 1 || got[0] != validation.ProfileMRV {
		t.Errorf("profiles = %v, want [mrv]", got)
	}
}

func TestMaxGapHoursFromBundle(t *testing.T) {
	if got := maxGapHoursFromBundle(nil); got != 12 {
		t.Errorf("nil-bundle maxGapHours = %v, want 12", got)
	}
	if got := maxGapHoursFromBundle(&configwire.Bundle{MaxGapHours: 8}); got != 8 {
		t.Errorf("maxGapHours = %v, want 8", got)
	}
}
