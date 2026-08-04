// SPDX-License-Identifier: AGPL-3.0-only

package fieldpolicy

import (
	"errors"
	"testing"

	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

func TestSetPolicy(t *testing.T) {
	tests := []struct {
		name            string
		state           validation.FieldPolicyState
		schemaMandatory bool
		wantErr         error
	}{
		{"hidden ok", validation.FieldHidden, false, nil},
		{"optional ok", validation.FieldOptional, false, nil},
		{"recommended ok", validation.FieldRecommended, false, nil},
		{"companyMandatory ok", validation.FieldCompanyMandatory, false, nil},
		{"schemaMandatory field rejects any policy set", validation.FieldOptional, true, ErrSchemaMandatoryImmutable},
		{"manually assigning schemaMandatory rejected", validation.FieldSchemaMandatory, false, ErrSchemaMandatoryImmutable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := New(compliance.FleetScope(), "log-abstract", "3.13")
			err := p.SetPolicy("Some_Field", tt.state, tt.schemaMandatory)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SetPolicy() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if got := p.Policy.StateFor("Some_Field", tt.schemaMandatory, ""); got != tt.state {
					t.Errorf("StateFor() = %q, want %q", got, tt.state)
				}
			}
		})
	}
}

func TestSetPolicy_UnknownState(t *testing.T) {
	p, _ := New(compliance.FleetScope(), "log-abstract", "3.13")
	if err := p.SetPolicy("Some_Field", validation.FieldPolicyState("bogus"), false); err == nil {
		t.Error("SetPolicy(bogus) error = nil, want an error")
	}
}

func TestSetPrefillClass(t *testing.T) {
	p, _ := New(compliance.FleetScope(), "log-abstract", "3.13")

	if err := p.SetPrefillClass("Wind_Force_Kn", PrefillGhost); err != nil {
		t.Fatalf("SetPrefillClass: %v", err)
	}
	if got := p.PrefillFor("Wind_Force_Kn"); got != PrefillGhost {
		t.Errorf("PrefillFor() = %q, want %q", got, PrefillGhost)
	}
	if got := p.PrefillFor("Never_Set"); got != PrefillNone {
		t.Errorf("PrefillFor(unset) = %q, want %q", got, PrefillNone)
	}

	// Setting back to "none" removes the override entirely rather than
	// storing an explicit "none" entry.
	if err := p.SetPrefillClass("Wind_Force_Kn", PrefillNone); err != nil {
		t.Fatalf("SetPrefillClass(none): %v", err)
	}
	if _, ok := p.Prefill["Wind_Force_Kn"]; ok {
		t.Error("Prefill still has an entry after setting PrefillNone")
	}

	if err := p.SetPrefillClass("Some_Field", PrefillClass("bogus")); err == nil {
		t.Error("SetPrefillClass(bogus) error = nil, want an error")
	}
}

func TestSetEvents(t *testing.T) {
	valid := map[string]bool{"Arrival": true, "Departure": true}
	p, err := New(compliance.FleetScope(), "log-abstract", "3.13")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := p.SetEvents("Tugs_Used", []string{"Arrival", "Departure"}, valid); err != nil {
		t.Fatalf("SetEvents() error = %v", err)
	}
	if got := p.EventsFor("Tugs_Used"); len(got) != 2 {
		t.Errorf("EventsFor = %v, want two events", got)
	}

	// An unknown code must fail loudly. A typo here would not error at
	// render time, it would silently hide the field on every real report.
	if err := p.SetEvents("Tugs_Used", []string{"Arivval"}, valid); err == nil {
		t.Error("SetEvents() accepted an unknown event code")
	}

	// Both ways of saying "every event" clear the narrowing entirely, so the
	// stored map only ever holds real restrictions.
	if err := p.SetEvents("Tugs_Used", []string{validation.AllEvents}, valid); err != nil {
		t.Fatalf("SetEvents(AllEvents) error = %v", err)
	}
	if got := p.EventsFor("Tugs_Used"); got != nil {
		t.Errorf("EventsFor after AllEvents = %v, want nil (no stored narrowing)", got)
	}
	if err := p.SetEvents("Tugs_Used", nil, valid); err != nil {
		t.Fatalf("SetEvents(nil) error = %v", err)
	}
	if _, ok := p.Events["Tugs_Used"]; ok {
		t.Error("an empty event list should remove the entry, not store it")
	}
}

// A field's policy state and its event narrowing are one rule authored on one
// editor row, so resolution must never take the state from one scope and the
// event list from another.
func TestEffectiveFieldPolicy_RuleNeverSplitsAcrossScopes(t *testing.T) {
	fleet := newUnchecked(compliance.FleetScope(), "log-abstract", "3.13")
	fleet.Policy["Tugs_Used"] = validation.FieldCompanyMandatory
	fleet.Events["Tugs_Used"] = []string{"Arrival", "Departure"}

	vessel := newUnchecked(compliance.Scope{Type: compliance.ScopeVessel, Key: "v1"}, "log-abstract", "3.13")
	vessel.Policy["Tugs_Used"] = validation.FieldHidden // no event narrowing of its own

	eff := EffectiveFieldPolicy([]*SchemaFieldPolicy{fleet, vessel}, "v1", nil, "log-abstract", "3.13")
	if got := eff.Policy["Tugs_Used"]; got != validation.FieldHidden {
		t.Errorf("state = %q, want hidden (vessel scope wins)", got)
	}
	if got := eff.Events["Tugs_Used"]; got != nil {
		t.Errorf("events = %v, want nil — the vessel row replaced the fleet rule whole, "+
			"it must not inherit the fleet's event narrowing", got)
	}
}

// An event narrowing with no state override of its own is a real authoring
// outcome ("leave this field at its default state, but only on Arrival"), so
// it must survive resolution rather than being dropped for having no state.
func TestEffectiveFieldPolicy_EventsOnlyRuleResolves(t *testing.T) {
	fleet := newUnchecked(compliance.FleetScope(), "log-abstract", "3.13")
	fleet.Events["Tugs_Used"] = []string{"Arrival"}

	eff := EffectiveFieldPolicy([]*SchemaFieldPolicy{fleet}, "v1", nil, "log-abstract", "3.13")
	if got := eff.Events["Tugs_Used"]; len(got) != 1 || got[0] != "Arrival" {
		t.Errorf("events = %v, want [Arrival]", got)
	}
	if _, ok := eff.Policy["Tugs_Used"]; ok {
		t.Error("no state was authored; none should be invented")
	}
}

// Groups still resolve by strictest state, and the winning group's own event
// narrowing comes with it.
func TestEffectiveFieldPolicy_StrictestGroupCarriesItsEvents(t *testing.T) {
	lax := newUnchecked(compliance.Scope{Type: compliance.ScopeGroup, Key: "bulk"}, "log-abstract", "3.13")
	lax.Policy["Tugs_Used"] = validation.FieldOptional
	lax.Events["Tugs_Used"] = []string{"EOSP"}

	strict := newUnchecked(compliance.Scope{Type: compliance.ScopeGroup, Key: "tanker"}, "log-abstract", "3.13")
	strict.Policy["Tugs_Used"] = validation.FieldCompanyMandatory
	strict.Events["Tugs_Used"] = []string{"Arrival", "Departure"}

	eff := EffectiveFieldPolicy([]*SchemaFieldPolicy{lax, strict}, "v1", []string{"bulk", "tanker"}, "log-abstract", "3.13")
	if got := eff.Policy["Tugs_Used"]; got != validation.FieldCompanyMandatory {
		t.Errorf("state = %q, want companyMandatory (strictest group)", got)
	}
	if got := eff.Events["Tugs_Used"]; len(got) != 2 || got[0] != "Arrival" {
		t.Errorf("events = %v, want the strictest group's own [Arrival Departure], not the other group's", got)
	}
}
