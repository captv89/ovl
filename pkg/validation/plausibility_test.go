// SPDX-License-Identifier: AGPL-3.0-only

package validation

import (
	"maps"
	"testing"
)

func TestEvaluateTimeBucketSum(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name   string
		fields map[string]any
		want   bool
	}{
		{
			name: "buckets sum matches within tolerance",
			fields: map[string]any{
				"Time_Since_Previous_Report": 12.0,
				"Time_Elapsed_Sailing":       11.5,
				"Time_Elapsed_Anchoring":     0.5,
			},
			want: false,
		},
		{
			name: "buckets sum drifts beyond tolerance",
			fields: map[string]any{
				"Time_Since_Previous_Report": 12.0,
				"Time_Elapsed_Sailing":       9.0,
			},
			want: true,
		},
		{
			name:   "no buckets reported skips the check",
			fields: map[string]any{"Time_Since_Previous_Report": 12.0},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.fields}
			got := len(evaluateTimeBucketSum(r, cfg)) > 0
			if got != tt.want {
				t.Errorf("evaluateTimeBucketSum() finding = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateImpliedSpeed(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name   string
		fields map[string]any
		want   bool
	}{
		{"within bounds", map[string]any{"Distance": 120.0, "Time_Elapsed_Sailing": 12.0}, false},   // 10kn
		{"implausibly fast", map[string]any{"Distance": 500.0, "Time_Elapsed_Sailing": 10.0}, true}, // 50kn
		{"no sailing time skips the check", map[string]any{"Distance": 500.0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.fields}
			got := len(evaluateImpliedSpeed(r, cfg)) > 0
			if got != tt.want {
				t.Errorf("evaluateImpliedSpeed() finding = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateNoDistanceStationary(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name   string
		fields map[string]any
		want   bool
	}{
		{
			name: "distance while fully at anchor",
			fields: map[string]any{
				"Distance": 5.0, "Time_Since_Previous_Report": 12.0, "Time_Elapsed_Anchoring": 12.0,
			},
			want: true,
		},
		{
			name:   "distance while moored (InPort)",
			fields: map[string]any{"Distance": 5.0, "Mode": "InPort"},
			want:   true,
		},
		{
			name: "distance while partially at anchor is fine",
			fields: map[string]any{
				"Distance": 5.0, "Time_Since_Previous_Report": 12.0, "Time_Elapsed_Anchoring": 2.0,
			},
			want: false,
		},
		{
			name:   "no distance reported",
			fields: map[string]any{"Mode": "InPort"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.fields}
			got := len(evaluateNoDistanceStationary(r, cfg)) > 0
			if got != tt.want {
				t.Errorf("evaluateNoDistanceStationary() finding = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateConsumptionSchemeExclusivity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FuelTypeConsumptionFields = []string{"ME_Consumption_HFO"}
	cfg.BDNMarkerFields = []string{"ME_Fuel_BDN"}

	t.Run("both schemes reported is an error", func(t *testing.T) {
		r := &Report{Fields: map[string]any{"ME_Consumption_HFO": 10.0, "ME_Fuel_BDN": "BDN-1"}}
		findings := evaluateConsumptionSchemeExclusivity(r, cfg, nil)
		if len(findings) == 0 || findings[0].Severity != SeverityError {
			t.Fatalf("findings = %+v, want a severity=error finding", findings)
		}
	})

	t.Run("only fuel-type scheme reported is fine", func(t *testing.T) {
		r := &Report{Fields: map[string]any{"ME_Consumption_HFO": 10.0}}
		if findings := evaluateConsumptionSchemeExclusivity(r, cfg, nil); len(findings) != 0 {
			t.Fatalf("findings = %+v, want none", findings)
		}
	})

	t.Run("BDN with no matching bunker report is an error", func(t *testing.T) {
		r := &Report{Fields: map[string]any{"ME_Fuel_BDN": "BDN-missing"}}
		lookup := func(bdn string) bool { return bdn == "BDN-known" }
		findings := evaluateConsumptionSchemeExclusivity(r, cfg, lookup)
		if len(findings) == 0 || findings[0].Severity != SeverityError {
			t.Fatalf("findings = %+v, want a severity=error finding", findings)
		}
	})

	t.Run("BDN with a matching bunker report is fine", func(t *testing.T) {
		r := &Report{Fields: map[string]any{"ME_Fuel_BDN": "BDN-known"}}
		lookup := func(bdn string) bool { return bdn == "BDN-known" }
		if findings := evaluateConsumptionSchemeExclusivity(r, cfg, lookup); len(findings) != 0 {
			t.Fatalf("findings = %+v, want none", findings)
		}
	})
}

func TestEvaluatePositionRequired(t *testing.T) {
	cfg := DefaultConfig()
	validPosition := map[string]any{
		"Latitude_Degree": 10.0, "Latitude_North_South": "N",
		"Longitude_Degree": 20.0, "Longitude_East_West": "E",
	}

	tests := []struct {
		name       string
		fields     map[string]any
		wantRuleID string
	}{
		{
			name:       "at sea and moving without position is required",
			fields:     map[string]any{"Mode": "AtSea"},
			wantRuleID: RulePositionRequired,
		},
		{
			name:   "at sea and moving with valid position is fine",
			fields: mergeMaps(map[string]any{"Mode": "Sailing"}, validPosition),
		},
		{
			name:   "in port without position is fine",
			fields: map[string]any{"Mode": "InPort"},
		},
		{
			name:       "latitude degrees out of range",
			fields:     mergeMaps(map[string]any{"Mode": "InPort"}, map[string]any{"Latitude_Degree": 95.0, "Latitude_North_South": "N", "Longitude_Degree": 20.0, "Longitude_East_West": "E"}),
			wantRuleID: RulePositionConsistency,
		},
		{
			name:       "invalid hemisphere letter",
			fields:     mergeMaps(map[string]any{"Mode": "InPort"}, map[string]any{"Latitude_Degree": 10.0, "Latitude_North_South": "X", "Longitude_Degree": 20.0, "Longitude_East_West": "E"}),
			wantRuleID: RulePositionConsistency,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{Fields: tt.fields}
			findings := evaluatePositionRequired(r, cfg)
			if tt.wantRuleID == "" {
				if len(findings) != 0 {
					t.Fatalf("findings = %+v, want none", findings)
				}
				return
			}
			if len(findings.ByRule(tt.wantRuleID)) == 0 {
				t.Fatalf("findings = %+v, want a finding for rule %s", findings, tt.wantRuleID)
			}
		})
	}
}

func mergeMaps(ms ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range ms {
		maps.Copy(out, m)
	}
	return out
}
