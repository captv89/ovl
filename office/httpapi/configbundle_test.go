// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

func TestHandlePreviewConfigBundle_ReflectsCurrentSchemaCount(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	names, err := s.st.ListSchemaNames(context.Background())
	if err != nil {
		t.Fatalf("ListSchemaNames: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/config-bundles/preview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	preview := decodeBody[configBundleSummaryView](t, rec)
	if preview.ID != "" {
		t.Errorf("ID = %q, want empty for an unpublished preview", preview.ID)
	}
	if preview.SchemaVersionCount != len(names) {
		t.Errorf("SchemaVersionCount = %d, want %d (one per known schema name)", preview.SchemaVersionCount, len(names))
	}
}

func TestHandlePublishConfigBundle_CreatesImmutableBundleAndGatesRole(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	cm := createTestUser2(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple 2")

	loginAs(t, c, viewer, "correct horse battery staple")
	rec := c.do(http.MethodPost, "/api/config-bundles/publish", publishBundleRequest{Label: "v1"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("publish as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}

	loginAs(t, c, cm, "correct horse battery staple 2")
	rec = c.do(http.MethodPost, "/api/config-bundles/publish", publishBundleRequest{Label: "v1-" + t.Name()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish as Config Manager: status %d, body %s", rec.Code, rec.Body)
	}
	published := decodeBody[configBundleSummaryView](t, rec)
	if published.ID == "" {
		t.Error("ID is empty, want a real bundle ID after publish")
	}
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM config_bundles WHERE id = $1`, published.ID)
	})

	rec = c.do(http.MethodGet, "/api/config-bundles", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]configBundleSummaryView](t, rec)
	found := false
	for _, b := range list {
		if b.ID == published.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("published bundle %s not found in history list", published.ID)
	}
}

func TestHandleBundleAssignment_SaveAndList(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	cm := createTestUser(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/config-bundles/publish", publishBundleRequest{Label: "for-assignment-" + t.Name()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish: status %d, body %s", rec.Code, rec.Body)
	}
	bundle := decodeBody[configBundleSummaryView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM bundle_assignments WHERE bundle_id = $1`, bundle.ID)
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM config_bundles WHERE id = $1`, bundle.ID)
	})

	// Architecture 6.5 (corrected 2026-07-15): fleet-wide is now a real
	// bundle-assignment scope, same as regulatory profiles/cadence/rule
	// severities — see office/configbundle/assignment.go's own doc comment.
	rec = c.do(http.MethodPut, "/api/config-bundles/assignments", saveBundleAssignmentRequest{
		Scope: scopeView{Type: "fleet"}, BundleID: bundle.ID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("assign fleet scope: status %d, body %s", rec.Code, rec.Body)
	}
	fleetAssignment := decodeBody[bundleAssignmentView](t, rec)
	if fleetAssignment.BundleLabel != bundle.Label {
		t.Errorf("fleet assignment BundleLabel = %q, want %q", fleetAssignment.BundleLabel, bundle.Label)
	}

	group := scopeView{Type: "group", Key: "TestHandleBundleAssignment-group"}
	rec = c.do(http.MethodPut, "/api/config-bundles/assignments", saveBundleAssignmentRequest{Scope: group, BundleID: bundle.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("assign group scope: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[bundleAssignmentView](t, rec)
	if saved.BundleLabel != bundle.Label {
		t.Errorf("BundleLabel = %q, want %q", saved.BundleLabel, bundle.Label)
	}

	rec = c.do(http.MethodGet, "/api/config-bundles/assignments", nil)
	list := decodeBody[[]bundleAssignmentView](t, rec)
	foundGroup, foundFleet := false, false
	for _, a := range list {
		if a.ScopeType == "group" && a.ScopeKey == group.Key && a.BundleID == bundle.ID {
			foundGroup = true
		}
		if a.ScopeType == "fleet" && a.BundleID == bundle.ID {
			foundFleet = true
		}
	}
	if !foundGroup {
		t.Errorf("group assignment not found in list: %+v", list)
	}
	if !foundFleet {
		t.Errorf("fleet assignment not found in list: %+v", list)
	}
}
