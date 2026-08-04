// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import "testing"

func TestNewCadenceRule(t *testing.T) {
	tests := []struct {
		name                   string
		scope                  Scope
		minReportIntervalHours float64
		maxGapHours            float64
		wantErr                bool
	}{
		{"defaults ok", FleetScope(), DefaultMinReportIntervalHours, DefaultMaxGapHours, false},
		{"zero min interval rejected", FleetScope(), 0, 12, true},
		{"negative max gap rejected", FleetScope(), 24, -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCadenceRule(tt.scope, tt.minReportIntervalHours, tt.maxGapHours)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCadenceRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveCadence(t *testing.T) {
	groupScope, _ := GroupScope("Fleet A")
	otherGroupScope, _ := GroupScope("Pacific")
	vesselScope, _ := VesselScope("vessel-1")

	fleet, _ := NewCadenceRule(FleetScope(), 24, 12)
	strictGroup, _ := NewCadenceRule(groupScope, 12, 6)
	looseGroup, _ := NewCadenceRule(otherGroupScope, 24, 10)
	vesselRule, _ := NewCadenceRule(vesselScope, 6, 3)

	tests := []struct {
		name   string
		rules  []*CadenceRule
		vessel string
		groups []string
		want   CadenceRule
	}{
		{
			name:   "no rules at all: hardcoded defaults",
			rules:  nil,
			vessel: "vessel-9",
			groups: nil,
			want:   CadenceRule{Scope: FleetScope(), MinReportIntervalHours: DefaultMinReportIntervalHours, MaxGapHours: DefaultMaxGapHours},
		},
		{
			name:   "fleet rule applies with no closer match",
			rules:  []*CadenceRule{fleet},
			vessel: "vessel-9",
			groups: nil,
			want:   *fleet,
		},
		{
			name:   "group rule beats fleet rule",
			rules:  []*CadenceRule{fleet, strictGroup},
			vessel: "vessel-2",
			groups: []string{"Fleet A"},
			want:   *strictGroup,
		},
		{
			name:   "most restrictive of two group rules wins",
			rules:  []*CadenceRule{strictGroup, looseGroup},
			vessel: "vessel-3",
			groups: []string{"Fleet A", "Pacific"},
			want:   *strictGroup,
		},
		{
			name:   "vessel rule beats everything",
			rules:  []*CadenceRule{fleet, strictGroup, vesselRule},
			vessel: "vessel-1",
			groups: []string{"Fleet A"},
			want:   *vesselRule,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveCadence(tt.rules, tt.vessel, tt.groups)
			if got.MinReportIntervalHours != tt.want.MinReportIntervalHours || got.MaxGapHours != tt.want.MaxGapHours {
				t.Errorf("EffectiveCadence() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
