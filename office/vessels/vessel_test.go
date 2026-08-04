// SPDX-License-Identifier: AGPL-3.0-only

package vessels

import (
	"testing"

	"github.com/google/uuid"
)

// validIMO is a real, checksum-valid IMO number (the standard example
// used to illustrate the IMO number check-digit formula).
const validIMO = "9074729"

func TestValidateIMO(t *testing.T) {
	tests := []struct {
		name    string
		imo     string
		wantErr bool
	}{
		{"valid", validIMO, false},
		{"wrong check digit", "9074728", true},
		{"too short", "907472", true},
		{"too long", "90747290", true},
		{"non-numeric", "907472X", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIMO(tt.imo)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIMO(%q) error = %v, wantErr %v", tt.imo, err, tt.wantErr)
			}
		})
	}
}

func TestNewVessel(t *testing.T) {
	v, err := NewVessel(validIMO, "MV Example", "Bulk Carrier", []string{"Fleet A", "Pacific"})
	if err != nil {
		t.Fatalf("NewVessel: %v", err)
	}
	if _, err := uuid.Parse(v.ID); err != nil {
		t.Errorf("ID = %q is not a valid UUID: %v", v.ID, err)
	}
	if v.IMO != validIMO {
		t.Errorf("IMO = %q, want %q", v.IMO, validIMO)
	}
	if len(v.Groups) != 2 {
		t.Errorf("Groups = %v, want 2 entries", v.Groups)
	}

	tests := []struct {
		name       string
		imo        string
		vesselName string
		vesselType string
	}{
		{"invalid IMO", "9074720", "MV Example", "Bulk Carrier"},
		{"empty name", validIMO, "", "Bulk Carrier"},
		{"whitespace-only name", validIMO, "   ", "Bulk Carrier"},
		{"empty type", validIMO, "MV Example", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewVessel(tt.imo, tt.vesselName, tt.vesselType, nil); err == nil {
				t.Fatal("got nil error, want an error")
			}
		})
	}
}

func TestNewVessel_TrimsFields(t *testing.T) {
	v, err := NewVessel(validIMO, "  MV Example  ", "  Bulk Carrier  ", nil)
	if err != nil {
		t.Fatalf("NewVessel: %v", err)
	}
	if v.Name != "MV Example" {
		t.Errorf("Name = %q, want trimmed", v.Name)
	}
	if v.Type != "Bulk Carrier" {
		t.Errorf("Type = %q, want trimmed", v.Type)
	}
}

func TestNewVessel_NormalizesGroups(t *testing.T) {
	v, err := NewVessel(validIMO, "MV Example", "Bulk Carrier", []string{" Fleet A ", "", "Fleet A", "Pacific"})
	if err != nil {
		t.Fatalf("NewVessel: %v", err)
	}
	want := []string{"Fleet A", "Pacific"}
	if len(v.Groups) != len(want) {
		t.Fatalf("Groups = %v, want %v", v.Groups, want)
	}
	for i, g := range want {
		if v.Groups[i] != g {
			t.Errorf("Groups[%d] = %q, want %q", i, v.Groups[i], g)
		}
	}
}

func TestNewVessel_NoGroupsIsValid(t *testing.T) {
	v, err := NewVessel(validIMO, "MV Example", "Bulk Carrier", nil)
	if err != nil {
		t.Fatalf("NewVessel: %v", err)
	}
	if len(v.Groups) != 0 {
		t.Errorf("Groups = %v, want empty", v.Groups)
	}
}

func TestVessel_UpdateProfile(t *testing.T) {
	v, err := NewVessel(validIMO, "MV Example", "Bulk Carrier", []string{"Fleet A"})
	if err != nil {
		t.Fatalf("NewVessel: %v", err)
	}
	originalIMO := v.IMO

	if err := v.UpdateProfile("MV Renamed", "Container", []string{"Fleet B"}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if v.Name != "MV Renamed" {
		t.Errorf("Name = %q, want %q", v.Name, "MV Renamed")
	}
	if v.Type != "Container" {
		t.Errorf("Type = %q, want %q", v.Type, "Container")
	}
	if len(v.Groups) != 1 || v.Groups[0] != "Fleet B" {
		t.Errorf("Groups = %v, want [Fleet B]", v.Groups)
	}
	if v.IMO != originalIMO {
		t.Errorf("IMO changed to %q, want unchanged %q", v.IMO, originalIMO)
	}

	if err := v.UpdateProfile("", "Container", nil); err == nil {
		t.Error("UpdateProfile(empty name) = nil error, want an error")
	}
	if err := v.UpdateProfile("MV Renamed", "", nil); err == nil {
		t.Error("UpdateProfile(empty type) = nil error, want an error")
	}
}
