// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
)

// TestHandleListVesselConfigs walks a single vessel through all four
// states the fleet-wide "Vessel configs" screen (2026-07-25 redesign)
// distinguishes: unassigned, pendingSync (assigned but never applied),
// synced (applied == assigned), and outOfDate (applied a stale bundle).
// Scoped to a unique test group throughout — audit 2026-07-22 §8's own
// fix for this package's shared-Postgres test pollution — so this never
// touches another test's vessels or bundle assignments.
func TestHandleListVesselConfigs(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	// Needs both roles: RoleAdmin to create the vessel, RoleConfigManager
	// to publish/assign bundles — this test exercises both sides of the
	// applied-vs-assigned comparison in one flow.
	cm := createTestUser(t, s, auth.Roles{auth.RoleAdmin, auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	group := "TestHandleListVesselConfigs-" + t.Name()
	vesselRec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{IMO: "9074729", Name: "MV Vessel Configs Test", Type: "Bulk Carrier", Groups: []string{group}})
	if vesselRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", vesselRec.Code, vesselRec.Body)
	}
	vessel := decodeBody[vesselView](t, vesselRec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, vessel.ID)
	})

	rowFor := func(list []vesselConfigView) *vesselConfigView {
		for i := range list {
			if list[i].VesselID == vessel.ID {
				return &list[i]
			}
		}
		return nil
	}
	fetch := func() *vesselConfigView {
		t.Helper()
		rec := c.do(http.MethodGet, "/api/vessel-configs", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/vessel-configs: status %d, body %s", rec.Code, rec.Body)
		}
		row := rowFor(decodeBody[[]vesselConfigView](t, rec))
		if row == nil {
			t.Fatal("GET /api/vessel-configs: test vessel not in the list")
		}
		return row
	}

	t.Run("unassigned before any bundle is assigned", func(t *testing.T) {
		row := fetch()
		if row.Status != "unassigned" {
			t.Errorf("Status = %q, want %q", row.Status, "unassigned")
		}
	})

	publishAndAssign := func(label string) configBundleSummaryView {
		t.Helper()
		rec := c.do(http.MethodPost, "/api/config-bundles/publish", publishBundleRequest{Label: label})
		if rec.Code != http.StatusCreated {
			t.Fatalf("publish %q: status %d, body %s", label, rec.Code, rec.Body)
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
		rec = c.do(http.MethodPut, "/api/config-bundles/assignments", saveBundleAssignmentRequest{
			Scope: scopeView{Type: "group", Key: group}, BundleID: bundle.ID,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("assign %q to group %q: status %d, body %s", label, group, rec.Code, rec.Body)
		}
		return bundle
	}

	first := publishAndAssign("vessel-configs-test-first-" + t.Name())

	t.Run("pendingSync once assigned but never applied", func(t *testing.T) {
		row := fetch()
		if row.Status != "pendingSync" {
			t.Errorf("Status = %q, want %q", row.Status, "pendingSync")
		}
		if row.AssignedBundleLabel != first.Label {
			t.Errorf("AssignedBundleLabel = %q, want %q", row.AssignedBundleLabel, first.Label)
		}
		if row.ActiveSince == nil {
			t.Error("ActiveSince is nil, want the assigned bundle's publish time")
		}
	})

	applyBundle := func(bundleID string, version int64) {
		t.Helper()
		if err := s.st.UpsertVesselSyncStatus(context.Background(), &store.VesselSyncStatus{
			VesselID: vessel.ID, LastSeenAt: time.Now().UTC(), AppliedBundleID: bundleID, AppliedBundleVersion: version,
		}); err != nil {
			t.Fatalf("UpsertVesselSyncStatus: %v", err)
		}
	}

	applyBundle(first.ID, 1)

	t.Run("synced once the vessel applies the assigned bundle", func(t *testing.T) {
		row := fetch()
		if row.Status != "synced" {
			t.Errorf("Status = %q, want %q", row.Status, "synced")
		}
	})

	second := publishAndAssign("vessel-configs-test-second-" + t.Name())

	t.Run("outOfDate once a newer bundle is assigned but not yet applied", func(t *testing.T) {
		row := fetch()
		if row.Status != "outOfDate" {
			t.Errorf("Status = %q, want %q", row.Status, "outOfDate")
		}
		if row.AssignedBundleLabel != second.Label {
			t.Errorf("AssignedBundleLabel = %q, want %q", row.AssignedBundleLabel, second.Label)
		}
	})
}
