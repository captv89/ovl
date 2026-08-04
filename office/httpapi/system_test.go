// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

// TestHandleGetSystem confirms design handoff B10's System tab reports
// real values — a reachable test database and the version string this
// test server was constructed with (see newTestServer's own NewServer
// call) — not placeholder/fake data.
func TestHandleGetSystem(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodGet, "/api/system", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/system: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
	view := decodeBody[systemView](t, rec)
	if view.Version != "test" {
		t.Errorf("Version = %q, want %q", view.Version, "test")
	}
	if !view.DatabaseReachable {
		t.Error("DatabaseReachable = false, want true against a live test database")
	}
	if view.AttachmentStoreBytes != 0 || view.AttachmentStoreCount != 0 {
		t.Errorf("AttachmentStoreBytes/Count = %d/%d, want 0/0 for a fresh empty attachment store", view.AttachmentStoreBytes, view.AttachmentStoreCount)
	}
}

func TestHandleGetSystem_RequiresAuth(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/system", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/system unauthenticated: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
