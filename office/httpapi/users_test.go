// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

// deleteTestUser is createTestUser's own cleanup logic, exposed
// standalone for users created through the HTTP endpoint under test
// (handleCreateUser) rather than directly through the store.
func deleteTestUser(t *testing.T, id string) {
	t.Helper()
	raw, err := sql.Open("pgx", testDSN(t))
	if err != nil {
		t.Errorf("cleanup: open raw connection: %v", err)
		return
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, id); err != nil {
		t.Errorf("cleanup: delete test user %s: %v", id, err)
	}
}

// TestHandleUsers_ProvisionListRolesDeactivateReactivate exercises
// design handoff B10's Users tab end to end: an Admin provisions a new
// account (a real temporary password comes back, reveal-once), the new
// account can log in with it, the Admin reassigns its roles, deactivates
// it (login and any live session immediately stop working), then
// reactivates it (login works again).
func TestHandleUsers_ProvisionListRolesDeactivateReactivate(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	t.Run("non-admin is forbidden", func(t *testing.T) {
		viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
		vc := newTestClient(t, s)
		loginAs(t, vc, viewer, "correct horse battery staple")
		rec := vc.do(http.MethodGet, "/api/users", nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET /api/users as viewer: status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	rec := c.do(http.MethodPost, "/api/users", createUserRequest{Username: "o9.newuser", Roles: []string{"viewer"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/users: status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body)
	}
	created := decodeBody[createUserResponse](t, rec)
	if created.TemporaryPassword == "" {
		t.Fatal("TemporaryPassword is empty, want a generated value")
	}
	if !created.User.Active {
		t.Error("newly created user Active = false, want true")
	}
	t.Cleanup(func() { deleteTestUser(t, created.User.ID) })

	nc := newTestClient(t, s)
	loginRec := nc.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "o9.newuser", Password: created.TemporaryPassword})
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login with generated temporary password: status = %d, want %d", loginRec.Code, http.StatusOK)
	}

	listRec := c.do(http.MethodGet, "/api/users", nil)
	list := decodeBody[[]userView](t, listRec)
	var found bool
	for _, u := range list {
		if u.ID == created.User.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("GET /api/users = %+v, want the newly created user listed", list)
	}

	rolesRec := c.do(http.MethodPut, "/api/users/"+created.User.ID+"/roles", updateUserRolesRequest{Roles: []string{"reviewer", "commercialEditor"}})
	if rolesRec.Code != http.StatusOK {
		t.Fatalf("PUT roles: status = %d, want %d, body %s", rolesRec.Code, http.StatusOK, rolesRec.Body)
	}
	updated := decodeBody[userView](t, rolesRec)
	if !updated.Roles.Has(auth.RoleReviewer) || !updated.Roles.Has(auth.RoleCommercialEditor) || updated.Roles.Has(auth.RoleViewer) {
		t.Errorf("Roles after reassignment = %+v, want exactly [reviewer commercialEditor]", updated.Roles)
	}

	deactivateRec := c.do(http.MethodPost, "/api/users/"+created.User.ID+"/deactivate", nil)
	if deactivateRec.Code != http.StatusOK {
		t.Fatalf("deactivate: status = %d, want %d", deactivateRec.Code, http.StatusOK)
	}
	if decodeBody[userView](t, deactivateRec).Active {
		t.Error("Active after deactivate = true, want false")
	}

	// A session already logged in before deactivation stops working
	// immediately, not just at its next login (matches
	// handleRevokeEnrollment's own "cut off anything already issued"
	// contract).
	meRec := nc.do(http.MethodGet, "/api/auth/me", nil)
	if meRec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/auth/me on a deactivated user's live session: status = %d, want %d", meRec.Code, http.StatusUnauthorized)
	}

	loginBlockedRec := newTestClient(t, s).do(http.MethodPost, "/api/auth/login", loginRequest{Username: "o9.newuser", Password: created.TemporaryPassword})
	if loginBlockedRec.Code != http.StatusUnauthorized {
		t.Errorf("login while deactivated: status = %d, want %d", loginBlockedRec.Code, http.StatusUnauthorized)
	}

	reactivateRec := c.do(http.MethodPost, "/api/users/"+created.User.ID+"/reactivate", nil)
	if reactivateRec.Code != http.StatusOK {
		t.Fatalf("reactivate: status = %d, want %d", reactivateRec.Code, http.StatusOK)
	}
	loginAgainRec := newTestClient(t, s).do(http.MethodPost, "/api/auth/login", loginRequest{Username: "o9.newuser", Password: created.TemporaryPassword})
	if loginAgainRec.Code != http.StatusOK {
		t.Errorf("login after reactivate: status = %d, want %d", loginAgainRec.Code, http.StatusOK)
	}
}

// TestHandleResetUserPassword fulfills Login.tsx's own "Ask an Admin to
// reset it" copy: an Admin-issued reset produces a fresh temporary
// password that supersedes the old one and forces a change on next
// login.
func TestHandleResetUserPassword(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	target := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "original-password")

	rec := c.do(http.MethodPost, "/api/users/"+target.ID+"/reset-password", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset-password: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	resp := decodeBody[createUserResponse](t, rec)
	if resp.TemporaryPassword == "" {
		t.Fatal("TemporaryPassword is empty, want a generated value")
	}
	if !resp.User.MustChangePassword {
		t.Error("MustChangePassword after Admin reset = false, want true")
	}

	oldRec := newTestClient(t, s).do(http.MethodPost, "/api/auth/login", loginRequest{Username: target.Username, Password: "original-password"})
	if oldRec.Code != http.StatusUnauthorized {
		t.Errorf("login with superseded password: status = %d, want %d", oldRec.Code, http.StatusUnauthorized)
	}
	newRec := newTestClient(t, s).do(http.MethodPost, "/api/auth/login", loginRequest{Username: target.Username, Password: resp.TemporaryPassword})
	if newRec.Code != http.StatusOK {
		t.Errorf("login with new temporary password: status = %d, want %d", newRec.Code, http.StatusOK)
	}
}
