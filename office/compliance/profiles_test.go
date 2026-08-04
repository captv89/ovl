// SPDX-License-Identifier: AGPL-3.0-only

package compliance

import (
	"errors"
	"slices"
	"testing"

	"github.com/captv89/ovl/pkg/validation"
)

func TestNewProfileAssignment(t *testing.T) {
	t.Run("dedups profiles", func(t *testing.T) {
		a, err := NewProfileAssignment(FleetScope(), []validation.RegulatoryProfile{
			validation.ProfileMRV, validation.ProfileMRV, validation.ProfileDCS,
		})
		if err != nil {
			t.Fatalf("NewProfileAssignment: %v", err)
		}
		if len(a.Profiles) != 2 {
			t.Errorf("Profiles = %v, want 2 entries", a.Profiles)
		}
	})

	t.Run("rejects unknown profile", func(t *testing.T) {
		_, err := NewProfileAssignment(FleetScope(), []validation.RegulatoryProfile{"bogus"})
		if err == nil {
			t.Fatal("NewProfileAssignment(bogus) error = nil, want an error")
		}
		var unknownErr *UnknownProfileError
		if !errors.As(err, &unknownErr) {
			t.Errorf("error = %v, want *UnknownProfileError", err)
		}
	})

	t.Run("rejects invalid scope", func(t *testing.T) {
		_, err := NewProfileAssignment(Scope{Type: ScopeGroup}, nil)
		if err == nil {
			t.Fatal("NewProfileAssignment(invalid scope) error = nil, want an error")
		}
	})
}

func TestEffectiveProfiles(t *testing.T) {
	fleet, _ := NewProfileAssignment(FleetScope(), []validation.RegulatoryProfile{validation.ProfileMRV})
	groupScope, _ := GroupScope("Fleet A")
	group, _ := NewProfileAssignment(groupScope, []validation.RegulatoryProfile{validation.ProfileDCS})
	vesselScope, _ := VesselScope("vessel-1")
	vessel, _ := NewProfileAssignment(vesselScope, []validation.RegulatoryProfile{validation.ProfileCII})

	assignments := []*ProfileAssignment{fleet, group, vessel}

	tests := []struct {
		name   string
		vessel string
		groups []string
		want   []validation.RegulatoryProfile
	}{
		{
			name:   "vessel-1 in Fleet A gets fleet + group + vessel-specific",
			vessel: "vessel-1",
			groups: []string{"Fleet A"},
			want:   []validation.RegulatoryProfile{validation.ProfileMRV, validation.ProfileDCS, validation.ProfileCII},
		},
		{
			name:   "vessel-2 not in Fleet A only gets fleet-wide",
			vessel: "vessel-2",
			groups: []string{"Fleet B"},
			want:   []validation.RegulatoryProfile{validation.ProfileMRV},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveProfiles(assignments, tt.vessel, tt.groups)
			for _, want := range tt.want {
				if !slices.Contains(got, want) {
					t.Errorf("EffectiveProfiles() = %v, want to contain %v", got, want)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("EffectiveProfiles() = %v, want exactly %v", got, tt.want)
			}
		})
	}
}
