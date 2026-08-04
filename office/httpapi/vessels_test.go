// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
)

func loginAs(t *testing.T, c *testClient, u *auth.User, password string) {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/auth/login", loginRequest{Username: u.Username, Password: password})
	if rec.Code != http.StatusOK {
		t.Fatalf("login as %s: status %d, body %s", u.Username, rec.Code, rec.Body)
	}
}

func TestHandleCreateVessel_AdminOnly(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{IMO: "9074729", Name: "MV Example", Type: "Bulk Carrier"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /api/vessels as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// TestHandleListVessels_LastReportAndOverdue exercises design handoff
// B2's last-report/overdue columns end to end: a fleet-wide cadence rule
// with a 12h max gap, one vessel whose most recent report is 30 hours old
// (overdue), one whose most recent report is 1 hour old (not overdue),
// and one that has never reported (neither field set) — proving
// vesselView.LastReportAt/OverdueHours are wired through the new
// LastReportEventTimeByVessel aggregate and overdueStatusFor's cadence
// resolution, not just that the JSON keys exist.
func TestHandleListVessels_LastReportAndOverdue(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rule, err := compliance.NewCadenceRule(compliance.FleetScope(), compliance.DefaultMinReportIntervalHours, 12)
	if err != nil {
		t.Fatalf("NewCadenceRule: %v", err)
	}
	if err := s.st.SaveCadenceRule(context.Background(), rule); err != nil {
		t.Fatalf("SaveCadenceRule: %v", err)
	}

	overdueVessel := createTestVesselForReports(t, s, 70)
	onTimeVessel := createTestVesselForReports(t, s, 71)
	neverReportedVessel := createTestVesselForReports(t, s, 72)

	now := time.Now().UTC()
	landTestReport(t, s, overdueVessel.ID, "report-overdue", 1, now.Add(-30*time.Hour), domain.StateSubmitted)
	landTestReport(t, s, onTimeVessel.ID, "report-ontime", 1, now.Add(-1*time.Hour), domain.StateSubmitted)

	rec := c.do(http.MethodGet, "/api/vessels", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	rows := decodeBody[[]vesselView](t, rec)
	byID := make(map[string]vesselView, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	overdue, ok := byID[overdueVessel.ID]
	if !ok {
		t.Fatalf("overdue vessel missing from response")
	}
	if overdue.LastReportAt == nil {
		t.Error("overdue vessel: LastReportAt = nil, want set")
	}
	if overdue.OverdueHours == nil || *overdue.OverdueHours < 12 {
		t.Errorf("overdue vessel: OverdueHours = %v, want a value >= 12", overdue.OverdueHours)
	}

	onTime, ok := byID[onTimeVessel.ID]
	if !ok {
		t.Fatalf("on-time vessel missing from response")
	}
	if onTime.LastReportAt == nil {
		t.Error("on-time vessel: LastReportAt = nil, want set")
	}
	if onTime.OverdueHours != nil {
		t.Errorf("on-time vessel: OverdueHours = %v, want nil", *onTime.OverdueHours)
	}

	never, ok := byID[neverReportedVessel.ID]
	if !ok {
		t.Fatalf("never-reported vessel missing from response")
	}
	if never.LastReportAt != nil || never.OverdueHours != nil {
		t.Errorf("never-reported vessel: LastReportAt = %v, OverdueHours = %v, want both nil", never.LastReportAt, never.OverdueHours)
	}
}

// TestHandleVessel_LastSync proves vesselView.LastSyncAt/AppVersion are
// wired to the vessel_sync_status table (office/store.VesselSyncStatus,
// recorded by office/syncservice.Server.SyncStatus on every successful
// check-in) on both the list and detail endpoints, and that a vessel
// which has never synced surfaces neither field rather than a zero time.
func TestHandleVessel_LastSync(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	syncedVessel := createTestVesselForReports(t, s, 80)
	neverSyncedVessel := createTestVesselForReports(t, s, 81)

	lastSeen := time.Now().UTC().Add(-15 * time.Minute).Truncate(time.Second)
	if err := s.st.UpsertVesselSyncStatus(context.Background(), &store.VesselSyncStatus{
		VesselID: syncedVessel.ID, AppVersion: "1.2.3", LastSeenAt: lastSeen,
	}); err != nil {
		t.Fatalf("UpsertVesselSyncStatus: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/vessels", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/vessels: status %d, body %s", rec.Code, rec.Body)
		}
		rows := decodeBody[[]vesselView](t, rec)
		byID := make(map[string]vesselView, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}

		synced := byID[syncedVessel.ID]
		if synced.LastSyncAt == nil || !synced.LastSyncAt.Equal(lastSeen) {
			t.Errorf("synced vessel: LastSyncAt = %v, want %v", synced.LastSyncAt, lastSeen)
		}
		if synced.AppVersion != "1.2.3" {
			t.Errorf("synced vessel: AppVersion = %q, want %q", synced.AppVersion, "1.2.3")
		}

		never := byID[neverSyncedVessel.ID]
		if never.LastSyncAt != nil {
			t.Errorf("never-synced vessel: LastSyncAt = %v, want nil", never.LastSyncAt)
		}
		if never.AppVersion != "" {
			t.Errorf("never-synced vessel: AppVersion = %q, want empty", never.AppVersion)
		}
	})

	t.Run("detail", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/vessels/"+syncedVessel.ID, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/vessels/{id}: status %d, body %s", rec.Code, rec.Body)
		}
		got := decodeBody[vesselDetailView](t, rec)
		if got.Vessel.LastSyncAt == nil || !got.Vessel.LastSyncAt.Equal(lastSeen) {
			t.Errorf("detail: LastSyncAt = %v, want %v", got.Vessel.LastSyncAt, lastSeen)
		}
		if got.Vessel.AppVersion != "1.2.3" {
			t.Errorf("detail: AppVersion = %q, want %q", got.Vessel.AppVersion, "1.2.3")
		}
	})
}

func TestVesselLifecycle(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	// Create.
	rec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{
		IMO: "9074729", Name: "MV Example", Type: "Bulk Carrier", Groups: []string{"Fleet A"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	created := decodeBody[vesselView](t, rec)
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, created.ID)
	})
	if created.EnrollmentState != "notIssued" {
		t.Errorf("EnrollmentState = %q, want notIssued", created.EnrollmentState)
	}

	// List should include it.
	rec = c.do(http.MethodGet, "/api/vessels", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vessels: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]vesselView](t, rec)
	found := false
	for _, v := range list {
		if v.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Error("created vessel not found in list")
	}

	// Get detail: no enrollment, no bundle assignment yet.
	rec = c.do(http.MethodGet, "/api/vessels/"+created.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vessels/{id}: status %d, body %s", rec.Code, rec.Body)
	}
	detail := decodeBody[vesselDetailView](t, rec)
	if detail.Enrollment != nil {
		t.Errorf("Enrollment = %+v, want nil before any issue", detail.Enrollment)
	}
	if detail.BundleAssignment != nil {
		t.Errorf("BundleAssignment = %+v, want nil with no bundle published", detail.BundleAssignment)
	}

	// Update profile.
	rec = c.do(http.MethodPut, "/api/vessels/"+created.ID, updateVesselRequest{
		Name: "MV Renamed", Type: "Container", Groups: []string{"Fleet B"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/vessels/{id}: status %d, body %s", rec.Code, rec.Body)
	}
	updated := decodeBody[vesselView](t, rec)
	if updated.Name != "MV Renamed" || updated.Type != "Container" || len(updated.Groups) != 1 || updated.Groups[0] != "Fleet B" {
		t.Errorf("updated vessel = %+v, want name=MV Renamed type=Container groups=[Fleet B]", updated)
	}

	// Issue enrollment.
	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/issue", issueEnrollmentRequest{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST .../enrollment/issue: status %d, body %s", rec.Code, rec.Body)
	}
	issued := decodeBody[issueResultView](t, rec)
	if issued.Code == "" || issued.InitialMasterPassword == "" {
		t.Errorf("issue result = %+v, want non-empty code and password", issued)
	}
	if issued.Enrollment.State != "issued" || issued.Enrollment.InitialMasterUsername != "master" {
		t.Errorf("issued enrollment = %+v, want state=issued username=master", issued.Enrollment)
	}

	// Vessel detail now reflects the issued enrollment.
	rec = c.do(http.MethodGet, "/api/vessels/"+created.ID, nil)
	detail = decodeBody[vesselDetailView](t, rec)
	if detail.Enrollment == nil || detail.Enrollment.State != "issued" {
		t.Fatalf("detail.Enrollment = %+v, want state=issued", detail.Enrollment)
	}
	rec = c.do(http.MethodGet, "/api/vessels", nil)
	list = decodeBody[[]vesselView](t, rec)
	for _, v := range list {
		if v.ID == created.ID && v.EnrollmentState != "issued" {
			t.Errorf("list EnrollmentState = %q, want issued", v.EnrollmentState)
		}
	}

	// Reissue produces a fresh code.
	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/reissue", issueEnrollmentRequest{})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../enrollment/reissue: status %d, body %s", rec.Code, rec.Body)
	}
	reissued := decodeBody[issueResultView](t, rec)
	if reissued.Code == issued.Code {
		t.Error("reissue produced the same code as the original issue, want a fresh one")
	}

	// Revoke.
	rec = c.do(http.MethodPost, "/api/vessels/"+created.ID+"/enrollment/revoke", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST .../enrollment/revoke: status %d, body %s", rec.Code, rec.Body)
	}
	revoked := decodeBody[enrollmentView](t, rec)
	if revoked.State != "revoked" || revoked.RevokedAt == nil {
		t.Errorf("revoked enrollment = %+v, want state=revoked with RevokedAt set", revoked)
	}
}

func TestHandleIssueEnrollment_UnknownVessel(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/vessels/00000000-0000-0000-0000-000000000000/enrollment/issue", issueEnrollmentRequest{})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
