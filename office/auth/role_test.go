// SPDX-License-Identifier: AGPL-3.0-only

package auth

import "testing"

func TestRole_Valid(t *testing.T) {
	for _, r := range AllRoles() {
		if !r.Valid() {
			t.Errorf("AllRoles() member %q reports Valid() = false", r)
		}
	}
	if Role("captain").Valid() {
		t.Error(`Role("captain").Valid() = true, want false`)
	}
	if len(AllRoles()) != 5 {
		t.Errorf("AllRoles() has %d roles, want 5 (architecture 12.2)", len(AllRoles()))
	}
}

func TestRoles_Has(t *testing.T) {
	rs := Roles{RoleAdmin, RoleReviewer}
	if !rs.Has(RoleAdmin) {
		t.Error("Has(RoleAdmin) = false, want true")
	}
	if rs.Has(RoleConfigManager) {
		t.Error("Has(RoleConfigManager) = true, want false")
	}
}

func TestRoles_Normalize(t *testing.T) {
	rs := Roles{RoleAdmin, RoleReviewer, RoleAdmin}
	got := rs.normalize()
	if len(got) != 2 {
		t.Fatalf("normalize() = %v, want 2 deduplicated roles", got)
	}
	if !got.Has(RoleAdmin) || !got.Has(RoleReviewer) {
		t.Errorf("normalize() = %v, want {admin, reviewer}", got)
	}
}

func TestValidateRoles(t *testing.T) {
	tests := []struct {
		name    string
		roles   Roles
		wantErr bool
	}{
		{"empty", Roles{}, true},
		{"nil", nil, true},
		{"one valid role", Roles{RoleViewer}, false},
		{"multiple valid roles", Roles{RoleAdmin, RoleConfigManager}, false},
		{"invalid role", Roles{Role("captain")}, true},
		{"mixed valid and invalid", Roles{RoleAdmin, Role("captain")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRoles(tt.roles)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRoles(%v) error = %v, wantErr %v", tt.roles, err, tt.wantErr)
			}
		})
	}
}
