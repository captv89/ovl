// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(testDSN(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return st
}

// newTestUser builds a user with a name unique to this test run, so
// concurrent/parallel test runs against the same shared Postgres don't
// collide on the users.username UNIQUE constraint.
func newTestUser(t *testing.T, roles auth.Roles) *auth.User {
	t.Helper()
	u, err := auth.NewUser(t.Name(), "correct horse battery staple", roles)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	return u
}

func TestStore_CreateAndGetUser(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u := newTestUser(t, auth.Roles{auth.RoleAdmin, auth.RoleReviewer})
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { deleteTestUser(t, st, u.ID) })

	got, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Username != u.Username {
		t.Errorf("Username = %q, want %q", got.Username, u.Username)
	}
	if !got.Roles.Has(auth.RoleAdmin) || !got.Roles.Has(auth.RoleReviewer) {
		t.Errorf("Roles = %v, want {admin, reviewer}", got.Roles)
	}
	if got.PasswordHash != u.PasswordHash {
		t.Error("PasswordHash round trip mismatch")
	}

	byName, err := st.GetUserByUsername(ctx, u.Username)
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if byName.ID != u.ID {
		t.Errorf("GetUserByUsername returned ID %q, want %q", byName.ID, u.ID)
	}
}

func TestStore_GetUser_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetUser(context.Background(), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetUser(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateUser(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u := newTestUser(t, auth.Roles{auth.RoleViewer})
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { deleteTestUser(t, st, u.ID) })

	if err := u.SetRoles(auth.Roles{auth.RoleAdmin, auth.RoleConfigManager}); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !got.Roles.Has(auth.RoleAdmin) || !got.Roles.Has(auth.RoleConfigManager) || got.Roles.Has(auth.RoleViewer) {
		t.Errorf("Roles = %v, want {admin, configManager} only", got.Roles)
	}
}

func TestStore_UpdateUser_NotFound(t *testing.T) {
	st := openTestStore(t)
	u := newTestUser(t, auth.Roles{auth.RoleViewer})
	u.ID = "00000000-0000-0000-0000-000000000000"
	if err := st.UpdateUser(context.Background(), u); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateUser(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListUsers(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u1 := newTestUser(t, auth.Roles{auth.RoleAdmin})
	u1.Username += "-1"
	u2 := newTestUser(t, auth.Roles{auth.RoleViewer})
	u2.Username += "-2"
	for _, u := range []*auth.User{u1, u2} {
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser(%s): %v", u.Username, err)
		}
		t.Cleanup(func(id string) func() { return func() { deleteTestUser(t, st, id) } }(u.ID))
	}

	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	var foundU1, foundU2 bool
	for _, u := range users {
		if u.ID == u1.ID {
			foundU1 = true
		}
		if u.ID == u2.ID {
			foundU2 = true
		}
	}
	if !foundU1 || !foundU2 {
		t.Errorf("ListUsers() missing created users: foundU1=%v foundU2=%v", foundU1, foundU2)
	}
}

func TestStore_HasAnyUser(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u := newTestUser(t, auth.Roles{auth.RoleViewer})
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { deleteTestUser(t, st, u.ID) })

	has, err := st.HasAnyUser(ctx)
	if err != nil {
		t.Fatalf("HasAnyUser: %v", err)
	}
	if !has {
		t.Error("HasAnyUser() = false after creating a user, want true")
	}
}

func deleteTestUser(t *testing.T, st *Store, id string) {
	t.Helper()
	if _, err := st.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, id); err != nil {
		t.Errorf("cleanup: delete test user %s: %v", id, err)
	}
}
