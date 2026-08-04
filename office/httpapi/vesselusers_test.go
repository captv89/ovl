// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
)

// TestHandleCreateVesselUser_QueuesAndSurfacesInDetail is architecture
// 9.3/12.4's remote vessel-user-administration path exercised at
// office's own HTTP surface: queuing a create shows up in the same
// vessel detail fetch's UserCommands, reveals the temporary password
// exactly once, and is refused for role=master (a second Master must
// never be created with no ceremony, remotely least of all).
func TestHandleCreateVesselUser_QueuesAndSurfacesInDetail(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 89)

	rec := c.do(http.MethodPost, "/api/vessels/"+v.ID+"/users", createVesselUserRequest{Username: "second-officer", Role: "secondOfficer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body)
	}
	queued := decodeBody[queuedUserCommandResponse](t, rec)
	if queued.TemporaryPassword == "" {
		t.Error("TemporaryPassword is empty, want a real reveal-once value")
	}
	if queued.Command.Action != "create" || queued.Command.Username != "second-officer" {
		t.Errorf("Command = %+v, want action=create username=second-officer", queued.Command)
	}

	rec = c.do(http.MethodGet, "/api/vessels/"+v.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get vessel: status %d, body %s", rec.Code, rec.Body)
	}
	detail := decodeBody[vesselDetailView](t, rec)
	if len(detail.UserCommands) != 1 || detail.UserCommands[0].ID != queued.Command.ID {
		t.Fatalf("UserCommands = %+v, want exactly the queued command", detail.UserCommands)
	}
	if detail.UserCommands[0].FetchedAt != nil || detail.UserCommands[0].AppliedAt != nil {
		t.Errorf("UserCommands[0] = %+v, want neither fetched nor applied yet", detail.UserCommands[0])
	}

	rec = c.do(http.MethodPost, "/api/vessels/"+v.ID+"/users", createVesselUserRequest{Username: "second-master", Role: "master"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("create role=master: status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleResetVesselUserPassword_RevealsOnce(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 88)

	rec := c.do(http.MethodPost, "/api/vessels/"+v.ID+"/users/master/reset-password", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reset: status %d, body %s", rec.Code, rec.Body)
	}
	queued := decodeBody[queuedUserCommandResponse](t, rec)
	if queued.TemporaryPassword == "" {
		t.Error("TemporaryPassword is empty — this is the Master-forgot-their-password recovery path, it must always reveal one")
	}
	if queued.Command.Action != "resetPassword" || queued.Command.Username != "master" {
		t.Errorf("Command = %+v, want action=resetPassword username=master", queued.Command)
	}
}

func TestHandleSetVesselUserRole_RefusesPromotionToMaster(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 87)

	rec := c.do(http.MethodPut, "/api/vessels/"+v.ID+"/users/second-officer/role", setVesselUserRoleRequest{Role: "master"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("promote to master: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = c.do(http.MethodPut, "/api/vessels/"+v.ID+"/users/second-officer/role", setVesselUserRoleRequest{Role: "chiefOfficer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid role change: status %d, body %s", rec.Code, rec.Body)
	}
	queued := decodeBody[queuedUserCommandResponse](t, rec)
	if queued.Command.Action != "setRole" || queued.Command.Role != "chiefOfficer" {
		t.Errorf("Command = %+v, want action=setRole role=chiefOfficer", queued.Command)
	}
}

func TestHandleDeactivateVesselUser_RefusesMaster(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 86)

	if err := s.st.ReplaceVesselUsers(context.Background(), v.ID, []store.VesselUser{
		{Username: "master", Role: "master", Active: true, CanSubmit: true, UpdatedAt: time.Now().UTC()},
		{Username: "second-officer", Role: "secondOfficer", Active: true, CanSubmit: false, UpdatedAt: time.Now().UTC()},
	}, time.Now().UTC()); err != nil {
		t.Fatalf("ReplaceVesselUsers: %v", err)
	}

	rec := c.do(http.MethodPost, "/api/vessels/"+v.ID+"/users/master/deactivate", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("deactivate master: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = c.do(http.MethodPost, "/api/vessels/"+v.ID+"/users/second-officer/deactivate", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("deactivate non-master: status %d, body %s", rec.Code, rec.Body)
	}
	queued := decodeBody[queuedUserCommandResponse](t, rec)
	if queued.Command.Action != "setActive" {
		t.Errorf("Command = %+v, want action=setActive", queued.Command)
	}
}

func TestHandleCreateVesselUser_RequiresAdmin(t *testing.T) {
	s := newTestServer(t)
	admin := newTestClient(t, s)
	loginAs(t, admin, createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple"), "correct horse battery staple")
	v := createTestVesselForReports(t, s, 85)

	c := newTestClient(t, s)
	viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels/"+v.ID+"/users", createVesselUserRequest{Username: "second-officer", Role: "secondOfficer"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
