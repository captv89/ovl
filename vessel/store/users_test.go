// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/captv89/ovl/vessel/auth"
)

func newTestUser(t *testing.T, username string, role auth.Role) *auth.User {
	t.Helper()
	u, err := auth.NewUser(username, "correct horse battery staple", role)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	return u
}

func TestStore_CreateAndGetUser(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	u := newTestUser(t, "master", auth.RoleMaster)

	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	byID, err := s.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if byID.Username != "master" || byID.Role != auth.RoleMaster || !byID.MustChangePassword || !byID.Active {
		t.Errorf("GetUser() = %+v, want matching the created user (Active=true)", byID)
	}

	byUsername, err := s.GetUserByUsername(ctx, "master")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if byUsername.ID != u.ID {
		t.Errorf("GetUserByUsername().ID = %q, want %q", byUsername.ID, u.ID)
	}

	if _, err := s.GetUser(ctx, "no-such-id"); err != ErrNotFound {
		t.Errorf("GetUser(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := s.GetUserByUsername(ctx, "no-such-user"); err != ErrNotFound {
		t.Errorf("GetUserByUsername(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_CreateUser_DuplicateUsername(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.CreateUser(ctx, newTestUser(t, "master", auth.RoleMaster)); err != nil {
		t.Fatalf("CreateUser (first): %v", err)
	}
	if err := s.CreateUser(ctx, newTestUser(t, "master", auth.RoleChiefOfficer)); err == nil {
		t.Fatal("CreateUser with a duplicate username: got nil error, want a UNIQUE constraint violation")
	}
}

func TestStore_UpdateUser(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	u := newTestUser(t, "2officer", auth.RoleSecondOfficer)
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u.SetCanSubmit(true)
	u.SetActive(false)
	if err := u.ChangePassword("their own chosen password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if err := s.UpdateUser(ctx, u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err := s.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !got.CanSubmit {
		t.Error("CanSubmit = false after UpdateUser, want true")
	}
	if got.Active {
		t.Error("Active = true after UpdateUser (post SetActive(false)), want false")
	}
	if got.MustChangePassword {
		t.Error("MustChangePassword = true after UpdateUser (post ChangePassword), want false")
	}
	if match, err := got.Authenticate("their own chosen password"); err != nil || !match {
		t.Errorf("Authenticate(new password) = (%v, %v), want (true, nil)", match, err)
	}
}

func TestStore_UpdateUser_Missing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	u := newTestUser(t, "ghost", auth.RoleMaster)
	if err := s.UpdateUser(ctx, u); err != ErrNotFound {
		t.Errorf("UpdateUser(never-created user) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListUsers(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for _, u := range []*auth.User{
		newTestUser(t, "master", auth.RoleMaster),
		newTestUser(t, "2officer", auth.RoleSecondOfficer),
		newTestUser(t, "chief-engineer", auth.RoleChiefEngineer),
	} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser(%s): %v", u.Username, err)
		}
	}

	got, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListUsers returned %d users, want 3", len(got))
	}
	wantOrder := []string{"2officer", "chief-engineer", "master"} // alphabetical
	for i, u := range got {
		if u.Username != wantOrder[i] {
			t.Errorf("ListUsers[%d].Username = %q, want %q", i, u.Username, wantOrder[i])
		}
	}
}
