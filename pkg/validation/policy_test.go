// SPDX-License-Identifier: AGPL-3.0-only

package validation

import "testing"

func TestFieldPolicy_StateFor(t *testing.T) {
	tests := []struct {
		name            string
		policy          FieldPolicy
		fieldName       string
		schemaMandatory bool
		relevance       string
		want            FieldPolicyState
	}{
		{name: "schemaMandatory always wins", policy: FieldPolicy{"F": FieldOptional}, fieldName: "F", schemaMandatory: true, relevance: "optional input", want: FieldSchemaMandatory},
		{name: "explicit override wins over relevance", policy: FieldPolicy{"F": FieldHidden}, fieldName: "F", relevance: "mandatory for MRV&DCS", want: FieldHidden},
		{name: "unlisted GHG-relevant field defaults to recommended", policy: FieldPolicy{}, fieldName: "F", relevance: "mandatory for MRV&DCS", want: FieldRecommended},
		{name: "unlisted informational-relevance field defaults to recommended", policy: FieldPolicy{}, fieldName: "F", relevance: "voluntary wrt MRV", want: FieldRecommended},
		{name: "unlisted optional-input field defaults to optional", policy: FieldPolicy{}, fieldName: "F", relevance: "optional input", want: FieldOptional},
		{name: "unlisted out-of-scope field defaults to optional", policy: FieldPolicy{}, fieldName: "F", relevance: "out of scope for GHG verification", want: FieldOptional},
		{name: "unlisted field with empty relevance defaults to optional", policy: FieldPolicy{}, fieldName: "F", relevance: "", want: FieldOptional},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.policy.StateFor(tt.fieldName, tt.schemaMandatory, tt.relevance); got != tt.want {
				t.Errorf("StateFor(%q, %v, %q) = %q, want %q", tt.fieldName, tt.schemaMandatory, tt.relevance, got, tt.want)
			}
		})
	}
}

func TestFieldEvents_AppliesTo(t *testing.T) {
	events := FieldEvents{
		"Tugs_Used":  {"Arrival", "Departure"},
		"Everywhere": {AllEvents},
		"Empty":      {},
	}
	tests := []struct {
		name      string
		fieldName string
		eventType string
		want      bool
	}{
		{name: "listed event applies", fieldName: "Tugs_Used", eventType: "Arrival", want: true},
		{name: "unlisted event does not", fieldName: "Tugs_Used", eventType: "Noon (Position) - Sea passage", want: false},
		{name: "field with no entry applies everywhere", fieldName: "Wind_Force_Kn", eventType: "Arrival", want: true},
		{name: "explicit wildcard applies everywhere", fieldName: "Everywhere", eventType: "EOSP", want: true},
		{name: "empty list applies everywhere", fieldName: "Empty", eventType: "EOSP", want: true},
		// A schema with no event concept of its own (bunker-report and
		// friends) reports an empty event type; nothing may be gated away
		// from it, or its whole form would vanish.
		{name: "empty event type skips the gate", fieldName: "Tugs_Used", eventType: "", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := events.AppliesTo(tt.fieldName, tt.eventType); got != tt.want {
				t.Errorf("AppliesTo(%q, %q) = %v, want %v", tt.fieldName, tt.eventType, got, tt.want)
			}
		})
	}

	var nilEvents FieldEvents
	if !nilEvents.AppliesTo("Tugs_Used", "Arrival") {
		t.Error("a nil FieldEvents must gate nothing (the no-config-bundle case)")
	}
}

func TestFieldPolicy_StateForEvent(t *testing.T) {
	policy := FieldPolicy{"Tugs_Used": FieldCompanyMandatory}
	events := FieldEvents{"Tugs_Used": {"Arrival", "Departure"}}

	if got := policy.StateForEvent("Tugs_Used", false, "", events, "Arrival"); got != FieldCompanyMandatory {
		t.Errorf("Tugs_Used on Arrival = %q, want companyMandatory", got)
	}
	// The whole point of the feature: a field the company scoped to port
	// events must not appear on a Noon at sea report.
	if got := policy.StateForEvent("Tugs_Used", false, "", events, "Noon (Position) - Sea passage"); got != FieldHidden {
		t.Errorf("Tugs_Used on Noon = %q, want hidden", got)
	}
	// Architecture 6.1: schema mandatoriness is immutable, so an event list
	// can never suppress it — otherwise company config could emit OVD output
	// the standard itself rejects.
	if got := policy.StateForEvent("Tugs_Used", true, "", events, "Noon (Position) - Sea passage"); got != FieldSchemaMandatory {
		t.Errorf("schemaMandatory Tugs_Used on Noon = %q, want schemaMandatory (never gated)", got)
	}
	// No event narrowing at all ⇒ identical to StateFor, which is what keeps
	// every policy authored before this feature behaving exactly as it did.
	if got := policy.StateForEvent("Tugs_Used", false, "", nil, "Noon (Position) - Sea passage"); got != FieldCompanyMandatory {
		t.Errorf("un-narrowed Tugs_Used = %q, want companyMandatory", got)
	}
}
