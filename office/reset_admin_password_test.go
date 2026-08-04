// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
)

// testDSN mirrors every other package's own copy of this helper (office/
// httpapi, office/syncservice, office/store) — not shared across
// packages, same reasoning those each give for their own copy.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("OVL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OVL_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return dsn
}

// createLockedOutTestAdmin creates a deactivated Admin with a password
// the test itself doesn't know (a random one from auth.NewUser's own
// caller-supplied string) — simulating "forgot the password AND the
// account somehow ended up deactivated," the worst case
// runResetAdminPassword should still recover from in one call.
func createLockedOutTestAdmin(t *testing.T, dsn string) (*store.Store, *auth.User) {
	t.Helper()
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	u, err := auth.NewUser(t.Name(), "some-forgotten-password-nobody-remembers", auth.Roles{auth.RoleAdmin})
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	u.Deactivate()
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", dsn)
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID)
	})
	return st, u
}

func TestRunResetAdminPassword_RecoversLockedOutAdmin(t *testing.T) {
	dsn := testDSN(t)
	st, u := createLockedOutTestAdmin(t, dsn)

	if err := runResetAdminPassword([]string{"-db-dsn", dsn, "-username", u.Username}); err != nil {
		t.Fatalf("runResetAdminPassword: %v", err)
	}

	reloaded, err := st.GetUserByUsername(context.Background(), u.Username)
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if !reloaded.Active {
		t.Error("Active = false after runResetAdminPassword, want true (it reactivates too, not just resets the password)")
	}
	if !reloaded.MustChangePassword {
		t.Error("MustChangePassword = false, want true — the new password is temporary")
	}
	if match, _ := reloaded.Authenticate("some-forgotten-password-nobody-remembers"); match {
		t.Error("the old password still authenticates after a reset, want it invalidated")
	}
}

func TestRunResetAdminPassword_UnknownUsername(t *testing.T) {
	dsn := testDSN(t)

	err := runResetAdminPassword([]string{"-db-dsn", dsn, "-username", "definitely-does-not-exist-" + t.Name()})
	if err == nil {
		t.Error("runResetAdminPassword(unknown username) = nil error, want a clear failure")
	}
}

func TestRunResetAdminPassword_RequiresUsername(t *testing.T) {
	dsn := testDSN(t)

	err := runResetAdminPassword([]string{"-db-dsn", dsn})
	if err == nil {
		t.Error("runResetAdminPassword(no -username) = nil error, want a validation failure")
	}
}
