// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/captv89/ovl/vessel/auth"
)

// loginAs logs c in as username/password, failing the test on any
// non-200 response.
func loginAs(t *testing.T, c *testClient, username, password string) {
	t.Helper()
	if rec := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: username, Password: password}); rec.Code != http.StatusOK {
		t.Fatalf("login as %s: status %d, body %s", username, rec.Code, rec.Body)
	}
}

// newSecondOfficerClient creates and logs in a non-Master user on the
// same server s, for exercising Master-only gating from a second
// session (mirroring backup_test.go's own c2 pattern).
func newSecondOfficerClient(t *testing.T, s *Server) *testClient {
	t.Helper()
	officer, err := auth.NewUser("second-officer", "another long password", auth.RoleSecondOfficer)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	if err := s.storeOrNil().CreateUser(context.Background(), officer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	c2 := newTestClient(t, s)
	loginAs(t, c2, "second-officer", "another long password")
	return c2
}

func TestUserAdmin_CreateListResetToggleDeactivate(t *testing.T) {
	s, c := newLoggedInTestServer(t)

	// Create a new (non-Master) user.
	rec := c.do(http.MethodPost, "/api/admin/users", createUserRequest{Username: "3officer", Role: auth.RoleThirdOfficer})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[createUserResponse](t, rec)
	if created.User.Username != "3officer" || created.User.Role != auth.RoleThirdOfficer {
		t.Errorf("created user = %+v, want username=3officer role=thirdOfficer", created.User)
	}
	if !created.User.Active {
		t.Error("created user Active = false, want true")
	}
	if !created.User.MustChangePassword {
		t.Error("created user MustChangePassword = false, want true")
	}
	if created.TemporaryPassword == "" {
		t.Error("TemporaryPassword is empty")
	}

	// A duplicate username is rejected.
	if rec := c.do(http.MethodPost, "/api/admin/users", createUserRequest{Username: "3officer", Role: auth.RoleThirdOfficer}); rec.Code != http.StatusConflict {
		t.Errorf("create duplicate user: status %d, want %d", rec.Code, http.StatusConflict)
	}

	// A second Master cannot be created through this endpoint.
	if rec := c.do(http.MethodPost, "/api/admin/users", createUserRequest{Username: "second-master", Role: auth.RoleMaster}); rec.Code != http.StatusBadRequest {
		t.Errorf("create user with role=master: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// The temporary password logs the new user in, and forces a change.
	c3 := newTestClient(t, s)
	loginAs(t, c3, "3officer", created.TemporaryPassword)
	rec = c3.do(http.MethodGet, "/api/auth/me", nil)
	me := decodeBody[userView](t, rec)
	if !me.MustChangePassword {
		t.Error("me.MustChangePassword = false right after temp-password login, want true")
	}

	// List includes the new user.
	rec = c.do(http.MethodGet, "/api/admin/users", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]adminUserView](t, rec)
	var target adminUserView
	found := false
	for _, u := range list {
		if u.Username == "3officer" {
			target, found = u, true
		}
	}
	if !found {
		t.Fatalf("list users = %+v, want it to include 3officer", list)
	}

	// Reset password: old temp password stops working, new one does.
	rec = c.do(http.MethodPost, "/api/admin/users/"+target.ID+"/reset-password", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset password: status %d, body %s", rec.Code, rec.Body)
	}
	reset := decodeBody[resetPasswordResponse](t, rec)
	if reset.TemporaryPassword == "" || reset.TemporaryPassword == created.TemporaryPassword {
		t.Errorf("reset TemporaryPassword = %q, want a new non-empty password", reset.TemporaryPassword)
	}
	c4 := newTestClient(t, s)
	if rec := c4.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "3officer", Password: created.TemporaryPassword}); rec.Code != http.StatusUnauthorized {
		t.Errorf("login with old temp password after reset: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	loginAs(t, c4, "3officer", reset.TemporaryPassword)

	// Toggle canSubmit.
	canSubmit := true
	rec = c.do(http.MethodPatch, "/api/admin/users/"+target.ID, updateUserRequest{CanSubmit: &canSubmit})
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle canSubmit: status %d, body %s", rec.Code, rec.Body)
	}
	if updated := decodeBody[adminUserView](t, rec); !updated.CanSubmit {
		t.Error("CanSubmit = false after PATCH canSubmit=true, want true")
	}

	// A Master cannot deactivate themself.
	inactive := false
	var selfID string
	if rec := c.do(http.MethodGet, "/api/auth/me", nil); rec.Code == http.StatusOK {
		selfID = decodeBody[userView](t, rec).ID
	}
	if rec := c.do(http.MethodPatch, "/api/admin/users/"+selfID, updateUserRequest{Active: &inactive}); rec.Code != http.StatusBadRequest {
		t.Errorf("deactivate self: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Deactivate the third officer: existing session dies, login blocked.
	rec = c.do(http.MethodPatch, "/api/admin/users/"+target.ID, updateUserRequest{Active: &inactive})
	if rec.Code != http.StatusOK {
		t.Fatalf("deactivate: status %d, body %s", rec.Code, rec.Body)
	}
	if updated := decodeBody[adminUserView](t, rec); updated.Active {
		t.Error("Active = true after PATCH active=false, want false")
	}
	if rec := c4.do(http.MethodGet, "/api/auth/me", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("me on deactivated user's live session: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := c4.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "3officer", Password: reset.TemporaryPassword}); rec.Code != http.StatusUnauthorized {
		t.Errorf("login as deactivated user: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Reactivate: login works again.
	active := true
	if rec := c.do(http.MethodPatch, "/api/admin/users/"+target.ID, updateUserRequest{Active: &active}); rec.Code != http.StatusOK {
		t.Fatalf("reactivate: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c4.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "3officer", Password: reset.TemporaryPassword}); rec.Code != http.StatusOK {
		t.Errorf("login after reactivate: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUserAdmin_NonMasterForbidden(t *testing.T) {
	s, _ := newLoggedInTestServer(t)
	c2 := newSecondOfficerClient(t, s)

	if rec := c2.do(http.MethodGet, "/api/admin/users", nil); rec.Code != http.StatusForbidden {
		t.Errorf("list users as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := c2.do(http.MethodPost, "/api/admin/users", createUserRequest{Username: "someone", Role: auth.RoleThirdOfficer}); rec.Code != http.StatusForbidden {
		t.Errorf("create user as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := c2.do(http.MethodPost, "/api/admin/users/anything/reset-password", nil); rec.Code != http.StatusForbidden {
		t.Errorf("reset password as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	canSubmit := true
	if rec := c2.do(http.MethodPatch, "/api/admin/users/anything", updateUserRequest{CanSubmit: &canSubmit}); rec.Code != http.StatusForbidden {
		t.Errorf("update user as non-Master: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}
