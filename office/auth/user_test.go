// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewUser(t *testing.T) {
	u, err := NewUser("reviewer1", "correct horse battery staple", Roles{RoleReviewer})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if _, err := uuid.Parse(u.ID); err != nil {
		t.Errorf("ID = %q is not a valid UUID: %v", u.ID, err)
	}
	if !u.MustChangePassword {
		t.Error("MustChangePassword = false for a newly created user, want true")
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
		roles    Roles
	}{
		{"empty username", "", "correct horse battery staple", Roles{RoleAdmin}},
		{"whitespace-only username", "   ", "correct horse battery staple", Roles{RoleAdmin}},
		{"too-short password", "admin", "short", Roles{RoleAdmin}},
		{"no roles", "admin", "correct horse battery staple", Roles{}},
		{"invalid role", "admin", "correct horse battery staple", Roles{Role("captain")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewUser(tt.username, tt.password, tt.roles); err == nil {
				t.Fatal("got nil error, want an error")
			}
		})
	}
}

func TestNewUser_TrimsUsername(t *testing.T) {
	u, err := NewUser("  admin  ", "correct horse battery staple", Roles{RoleAdmin})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if u.Username != "admin" {
		t.Errorf("Username = %q, want trimmed %q", u.Username, "admin")
	}
}

func TestNewUser_DeduplicatesRoles(t *testing.T) {
	u, err := NewUser("admin", "correct horse battery staple", Roles{RoleAdmin, RoleAdmin, RoleReviewer})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if len(u.Roles) != 2 {
		t.Errorf("Roles = %v, want 2 deduplicated roles", u.Roles)
	}
}

func TestUser_ChangePassword(t *testing.T) {
	u, err := NewUser("admin", "correct horse battery staple", Roles{RoleAdmin})
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
	u, err := NewUser("reviewer1", "correct horse battery staple", Roles{RoleReviewer})
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
		t.Error("MustChangePassword = false after an Admin-issued ResetPassword, want true (it's a temporary password too)")
	}
	if match, err := u.Authenticate("temporary-reset-password"); err != nil || !match {
		t.Errorf("Authenticate(reset password) = (%v, %v), want (true, nil)", match, err)
	}
}

func TestUser_SetRoles(t *testing.T) {
	u, err := NewUser("u", "correct horse battery staple", Roles{RoleViewer})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if err := u.SetRoles(Roles{RoleAdmin, RoleConfigManager}); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	if !u.Roles.Has(RoleAdmin) || !u.Roles.Has(RoleConfigManager) {
		t.Errorf("Roles = %v, want {admin, configManager}", u.Roles)
	}
	if err := u.SetRoles(Roles{}); err == nil {
		t.Error("SetRoles(empty) = nil error, want an error")
	}
}

func TestUser_CapabilityMethods(t *testing.T) {
	tests := []struct {
		name                  string
		roles                 Roles
		canManageUsers        bool
		canManageConfig       bool
		canEditCommercialData bool
		canReview             bool
	}{
		{"admin only", Roles{RoleAdmin}, true, false, false, false},
		{"config manager only", Roles{RoleConfigManager}, false, true, false, false},
		{"commercial editor only", Roles{RoleCommercialEditor}, false, false, true, false},
		{"reviewer only", Roles{RoleReviewer}, false, false, false, true},
		{"viewer only", Roles{RoleViewer}, false, false, false, false},
		{"combined admin + reviewer", Roles{RoleAdmin, RoleReviewer}, true, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := NewUser("u", "correct horse battery staple", tt.roles)
			if err != nil {
				t.Fatalf("NewUser: %v", err)
			}
			if got := u.CanManageUsers(); got != tt.canManageUsers {
				t.Errorf("CanManageUsers() = %v, want %v", got, tt.canManageUsers)
			}
			if got := u.CanManageConfig(); got != tt.canManageConfig {
				t.Errorf("CanManageConfig() = %v, want %v", got, tt.canManageConfig)
			}
			if got := u.CanEditCommercialData(); got != tt.canEditCommercialData {
				t.Errorf("CanEditCommercialData() = %v, want %v", got, tt.canEditCommercialData)
			}
			if got := u.CanReview(); got != tt.canReview {
				t.Errorf("CanReview() = %v, want %v", got, tt.canReview)
			}
		})
	}
}
