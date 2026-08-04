// SPDX-License-Identifier: AGPL-3.0-only

package configbundle

import (
	"testing"

	"github.com/captv89/ovl/office/compliance"
)

func TestNewBundleAssignment(t *testing.T) {
	vesselScope, _ := compliance.VesselScope("vessel-1")
	groupScope, _ := compliance.GroupScope("Fleet A")

	tests := []struct {
		name     string
		scope    compliance.Scope
		bundleID string
		wantErr  bool
	}{
		{"vessel scope ok", vesselScope, "bundle-1", false},
		{"group scope ok", groupScope, "bundle-1", false},
		{"fleet scope ok", compliance.FleetScope(), "bundle-1", false},
		{"missing bundle id rejected", vesselScope, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewBundleAssignment(tt.scope, tt.bundleID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewBundleAssignment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if a.AssignedAt.IsZero() {
				t.Error("AssignedAt is zero")
			}
		})
	}
}

func TestResolve_VesselScopeWinsOverGroup(t *testing.T) {
	vesselScope, _ := compliance.VesselScope("vessel-1")
	groupScope, _ := compliance.GroupScope("Fleet A")
	assignments := []*BundleAssignment{
		{Scope: groupScope, BundleID: "group-bundle"},
		{Scope: vesselScope, BundleID: "vessel-bundle"},
	}
	got := Resolve(assignments, "vessel-1", []string{"Fleet A"})
	if got == nil || got.BundleID != "vessel-bundle" {
		t.Errorf("Resolve() = %v, want the direct vessel-scope assignment", got)
	}
}

func TestResolve_FallsBackToGroup(t *testing.T) {
	groupScope, _ := compliance.GroupScope("Fleet A")
	assignments := []*BundleAssignment{{Scope: groupScope, BundleID: "group-bundle"}}
	got := Resolve(assignments, "vessel-1", []string{"Fleet A", "Fleet B"})
	if got == nil || got.BundleID != "group-bundle" {
		t.Errorf("Resolve() = %v, want the group-scope assignment", got)
	}
}

func TestResolve_NoMatch(t *testing.T) {
	groupScope, _ := compliance.GroupScope("Fleet A")
	assignments := []*BundleAssignment{{Scope: groupScope, BundleID: "group-bundle"}}
	if got := Resolve(assignments, "vessel-1", []string{"Fleet B"}); got != nil {
		t.Errorf("Resolve() = %v, want nil (vessel not in the assigned group)", got)
	}
}

// TestResolve_FallsBackToFleet covers the precedence tier 2026-07-15
// added: fleet-wide is the least-specific fallback, tried only after
// vessel and group both miss.
func TestResolve_FallsBackToFleet(t *testing.T) {
	assignments := []*BundleAssignment{{Scope: compliance.FleetScope(), BundleID: "fleet-bundle"}}
	got := Resolve(assignments, "vessel-1", []string{"Fleet B"})
	if got == nil || got.BundleID != "fleet-bundle" {
		t.Errorf("Resolve() = %v, want the fleet-scope assignment", got)
	}
}

func TestResolve_GroupWinsOverFleet(t *testing.T) {
	groupScope, _ := compliance.GroupScope("Fleet A")
	assignments := []*BundleAssignment{
		{Scope: compliance.FleetScope(), BundleID: "fleet-bundle"},
		{Scope: groupScope, BundleID: "group-bundle"},
	}
	got := Resolve(assignments, "vessel-1", []string{"Fleet A"})
	if got == nil || got.BundleID != "group-bundle" {
		t.Errorf("Resolve() = %v, want the group-scope assignment (more specific than fleet)", got)
	}
}
