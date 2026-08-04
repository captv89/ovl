// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewUser(t *testing.T) {
	u, err := NewUser("2officer", "correct horse battery staple", RoleSecondOfficer)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if _, err := uuid.Parse(u.ID); err != nil {
		t.Errorf("ID = %q is not a valid UUID: %v", u.ID, err)
	}
	if !u.MustChangePassword {
		t.Error("MustChangePassword = false for a newly created user, want true")
	}
	if !u.Active {
		t.Error("Active = false for a newly created user, want true")
	}
	if u.PasswordHash == "correct horse battery staple" {
		t.Error("PasswordHash stores the plaintext password unchanged")
	}
	match, err := u.Authenticate("correct horse battery staple")
	if err != nil || !match {
		t.Errorf("Authenticate(correct password) = (%v, %v), want (true, nil)", match, err)
	}

	tests := []struct {
		name     string
		username string
		password string
		role     Role
	}{
		{"empty username", "", "correct horse battery staple", RoleMaster},
		{"whitespace-only username", "   ", "correct horse battery staple", RoleMaster},
		{"too-short password", "master", "short", RoleMaster},
		{"invalid role", "master", "correct horse battery staple", Role("captain")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewUser(tt.username, tt.password, tt.role); err == nil {
				t.Fatal("got nil error, want an error")
			}
		})
	}
}

func TestNewUser_TrimsUsername(t *testing.T) {
	u, err := NewUser("  master  ", "correct horse battery staple", RoleMaster)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if u.Username != "master" {
		t.Errorf("Username = %q, want trimmed %q", u.Username, "master")
	}
}

func TestUser_ChangePassword(t *testing.T) {
	u, err := NewUser("master", "correct horse battery staple", RoleMaster)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if !u.MustChangePassword {
		t.Fatal("precondition: MustChangePassword should start true")
	}

	if err := u.ChangePassword("a brand new password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if u.MustChangePassword {
		t.Error("MustChangePassword = true after ChangePassword, want false")
	}
	if match, err := u.Authenticate("a brand new password"); err != nil || !match {
		t.Errorf("Authenticate(new password) = (%v, %v), want (true, nil)", match, err)
	}
	if match, _ := u.Authenticate("correct horse battery staple"); match {
		t.Error("Authenticate(old password) = true after ChangePassword, want false")
	}
}

func TestUser_ResetPassword(t *testing.T) {
	u, err := NewUser("2officer", "correct horse battery staple", RoleSecondOfficer)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if err := u.ChangePassword("their own chosen password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if err := u.ResetPassword("temporary-reset-password"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if !u.MustChangePassword {
		t.Error("MustChangePassword = false after a Master-issued ResetPassword, want true (it's a temporary password too)")
	}
	if match, err := u.Authenticate("temporary-reset-password"); err != nil || !match {
		t.Errorf("Authenticate(reset password) = (%v, %v), want (true, nil)", match, err)
	}
}

func TestUser_SetRole(t *testing.T) {
	u, err := NewUser("2officer", "correct horse battery staple", RoleSecondOfficer)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}

	if err := u.SetRole(RoleMaster); err == nil {
		t.Error("SetRole(RoleMaster) = nil error, want a refusal — becoming Master must be a first-run-setup ceremony, never a role change")
	}
	if u.Role != RoleSecondOfficer {
		t.Errorf("Role = %q after a refused SetRole, want unchanged %q", u.Role, RoleSecondOfficer)
	}

	if err := u.SetRole(RoleChiefOfficer); err != nil {
		t.Fatalf("SetRole(RoleChiefOfficer): %v", err)
	}
	if u.Role != RoleChiefOfficer {
		t.Errorf("Role = %q, want %q", u.Role, RoleChiefOfficer)
	}

	if err := u.SetRole(Role("not-a-real-role")); err == nil {
		t.Error("SetRole(invalid role) = nil error, want a validation failure")
	}
}

func TestUser_CanSubmitReports(t *testing.T) {
	tests := []struct {
		name      string
		role      Role
		canSubmit bool
		want      bool
	}{
		{"master always, regardless of flag", RoleMaster, false, true},
		{"non-master without flag", RoleChiefOfficer, false, false},
		{"non-master with flag", RoleChiefOfficer, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := NewUser("u", "correct horse battery staple", tt.role)
			if err != nil {
				t.Fatalf("NewUser: %v", err)
			}
			u.CanSubmit = tt.canSubmit
			if got := u.CanSubmitReports(); got != tt.want {
				t.Errorf("CanSubmitReports() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUser_SetActive(t *testing.T) {
	u, err := NewUser("2officer", "correct horse battery staple", RoleSecondOfficer)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if !u.Active {
		t.Fatal("precondition: Active should start true")
	}

	u.SetActive(false)
	if u.Active {
		t.Error("Active = true after SetActive(false), want false")
	}

	u.SetActive(true)
	if !u.Active {
		t.Error("Active = false after SetActive(true), want true")
	}
}

func TestUser_IsSuperAdmin(t *testing.T) {
	master, _ := NewUser("master", "correct horse battery staple", RoleMaster)
	if !master.IsSuperAdmin() {
		t.Error("IsSuperAdmin() = false for RoleMaster, want true")
	}
	officer, _ := NewUser("officer", "correct horse battery staple", RoleChiefOfficer)
	if officer.IsSuperAdmin() {
		t.Error("IsSuperAdmin() = true for RoleChiefOfficer, want false")
	}
}
