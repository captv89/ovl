// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"slices"
	"testing"

	"github.com/captv89/ovl/pkg/schema"
)

func regulatoryTestFields() []schema.Field {
	return []schema.Field{
		{Name: "ME_Fuel_HFO", Relevance: "mandatory for MRV&DCS"},
		{Name: "CII_Correction_Factor", Relevance: "for CII correction, voluntary wrt MRV"},
		{Name: "Voyage_Verif_Field", Relevance: "recommended for voyage level verfication schemes"},
		{Name: "Unrelated_Field", Relevance: "out of scope for GHG verification"},
		{Name: "Optional_Field", Relevance: "optional input"},
	}
}

func TestEvaluateRegulatoryReadiness(t *testing.T) {
	fields := regulatoryTestFields()

	tests := []struct {
		name        string
		values      map[string]any
		wantProfile RegulatoryProfile
		wantReady   bool
		wantMissing []string
	}{
		{
			name:        "MRV missing its mandatory field",
			values:      map[string]any{},
			wantProfile: ProfileMRV,
			wantReady:   false,
			wantMissing: []string{"ME_Fuel_HFO"},
		},
		{
			name:        "MRV ready once the mandatory field is filled — the informational CII-correction field doesn't block it",
			values:      map[string]any{"ME_Fuel_HFO": 812.4},
			wantProfile: ProfileMRV,
			wantReady:   true,
			wantMissing: nil,
		},
		{
			name:        "DCS ready — shares the same mandatory field as MRV",
			values:      map[string]any{"ME_Fuel_HFO": 812.4},
			wantProfile: ProfileDCS,
			wantReady:   true,
			wantMissing: nil,
		},
		{
			name:        "CII has no requirementNeeded field, so it's always ready once it appears at all",
			values:      map[string]any{},
			wantProfile: ProfileCII,
			wantReady:   true,
			wantMissing: nil,
		},
		{
			name:        "voyage verification missing its recommended field",
			values:      map[string]any{},
			wantProfile: ProfileVoyageVerification,
			wantReady:   false,
			wantMissing: []string{"Voyage_Verif_Field"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.values}
			result := EvaluateRegulatoryReadiness(r, fields, AllRegulatoryProfiles, nil, nil)
			var got *ProfileReadiness
			for i := range result {
				if result[i].Profile == tt.wantProfile {
					got = &result[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("profile %s not present in result %+v", tt.wantProfile, result)
			}
			if got.Ready() != tt.wantReady {
				t.Errorf("Ready() = %v, want %v", got.Ready(), tt.wantReady)
			}
			if !slices.Equal(got.MissingFields, tt.wantMissing) {
				t.Errorf("MissingFields = %v, want %v", got.MissingFields, tt.wantMissing)
			}
		})
	}
}

func TestEvaluateRegulatoryReadiness_DisabledProfileOmitted(t *testing.T) {
	fields := regulatoryTestFields()
	r := &Report{Fields: map[string]any{}}
	result := EvaluateRegulatoryReadiness(r, fields, []RegulatoryProfile{ProfileDCS}, nil, nil)
	if len(result) != 1 || result[0].Profile != ProfileDCS {
		t.Fatalf("expected only ProfileDCS in result, got %+v", result)
	}
}

func TestEvaluateRegulatoryReadiness_UnrecognizedRelevanceOmitted(t *testing.T) {
	fields := []schema.Field{{Name: "X", Relevance: "not in use"}}
	r := &Report{Fields: map[string]any{}}
	result := EvaluateRegulatoryReadiness(r, fields, AllRegulatoryProfiles, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected no profiles for an unrecognized/unmapped relevance string, got %+v", result)
	}
}

// A field the company has hidden from the crew (architecture 6.1: "field
// does not exist for the crew") must not count toward a profile's
// MissingFields — mirrors EvaluateFieldRules' own FieldHidden skip
// (fieldrules.go), which this readiness check previously didn't share,
// letting hidden fields inflate the health check's missing-field count.
func TestEvaluateRegulatoryReadiness_HiddenFieldNotCountedMissing(t *testing.T) {
	fields := regulatoryTestFields()
	r := &Report{Fields: map[string]any{}}
	policy := FieldPolicy{"ME_Fuel_HFO": FieldHidden}
	result := EvaluateRegulatoryReadiness(r, fields, AllRegulatoryProfiles, policy, nil)
	for _, p := range result {
		if p.Profile == ProfileMRV {
			if !p.Ready() {
				t.Errorf("MRV MissingFields = %v, want empty (its only mandatory field is hidden)", p.MissingFields)
			}
			return
		}
	}
}

// The company can also hide a field for specific voyage events only
// (FieldEvents/StateForEvent) — this must be honored the same way as an
// outright FieldHidden policy entry.
func TestEvaluateRegulatoryReadiness_EventGatedFieldNotCountedMissing(t *testing.T) {
	fields := regulatoryTestFields()
	r := &Report{EventType: "Noon (Position) - Sea passage", Fields: map[string]any{}}
	events := FieldEvents{"ME_Fuel_HFO": []string{"Arrival", "Departure"}}
	result := EvaluateRegulatoryReadiness(r, fields, AllRegulatoryProfiles, nil, events)
	for _, p := range result {
		if p.Profile == ProfileMRV {
			if !p.Ready() {
				t.Errorf("MRV MissingFields = %v, want empty (its only mandatory field is event-gated away)", p.MissingFields)
			}
			return
		}
	}
}
