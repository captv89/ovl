// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import "testing"

func TestScope_Validate(t *testing.T) {
	tests := []struct {
		name    string
		scope   Scope
		wantErr bool
	}{
		{"fleet ok", FleetScope(), false},
		{"fleet with key rejected", Scope{Type: ScopeFleet, Key: "oops"}, true},
		{"group ok", Scope{Type: ScopeGroup, Key: "Fleet A"}, false},
		{"group without key rejected", Scope{Type: ScopeGroup}, true},
		{"vessel ok", Scope{Type: ScopeVessel, Key: "vessel-1"}, false},
		{"vessel without key rejected", Scope{Type: ScopeVessel}, true},
		{"unknown type rejected", Scope{Type: "bogus"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scope.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGroupScope_TrimsAndRejectsEmpty(t *testing.T) {
	s, err := GroupScope("  Fleet A  ")
	if err != nil {
		t.Fatalf("GroupScope: %v", err)
	}
	if s.Key != "Fleet A" {
		t.Errorf("Key = %q, want %q", s.Key, "Fleet A")
	}
	if _, err := GroupScope("   "); err == nil {
		t.Error("GroupScope(blank) error = nil, want an error")
	}
}

func TestVesselScope_RejectsEmpty(t *testing.T) {
	if _, err := VesselScope(""); err == nil {
		t.Error("VesselScope(\"\") error = nil, want an error")
	}
}

func TestScope_coversVessel(t *testing.T) {
	groupA, _ := GroupScope("Fleet A")
	vessel1, _ := VesselScope("vessel-1")

	tests := []struct {
		name    string
		scope   Scope
		vessel  string
		groups  []string
		covered bool
	}{
		{"fleet covers any vessel", FleetScope(), "vessel-1", nil, true},
		{"group covers matching vessel", groupA, "vessel-1", []string{"Fleet A", "Pacific"}, true},
		{"group excludes non-matching vessel", groupA, "vessel-2", []string{"Fleet B"}, false},
		{"vessel scope matches only that vessel", vessel1, "vessel-1", nil, true},
		{"vessel scope excludes other vessels", vessel1, "vessel-2", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scope.CoversVessel(tt.vessel, tt.groups); got != tt.covered {
				t.Errorf("coversVessel() = %v, want %v", got, tt.covered)
			}
		})
	}
}
