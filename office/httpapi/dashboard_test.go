// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/enrollment"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// enrollTestVessel marks v as enrolled directly through the store
// (bypassing the real Issue/Redeem crypto handshake, which is Phase 4's
// own territory and already covered elsewhere) — the dashboard only
// cares about enrollment *state*, not how a vessel got there.
func enrollTestVessel(t *testing.T, s *Server, vesselID string) {
	t.Helper()
	e := &enrollment.Enrollment{
		VesselID: vesselID, State: enrollment.StateEnrolled,
		InitialMasterUsername: "master", IssuedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.st.UpsertEnrollment(context.Background(), e); err != nil {
		t.Fatalf("UpsertEnrollment: %v", err)
	}
}

// TestHandleGetDashboard exercises design handoff B1's four real
// widgets end to end: one enrolled vessel overdue (30h since its last
// report against a 12h fleet cadence) drives both the overdue-vessels
// list and pulls compliance below 100%; one enrolled vessel with a
// recent report stays compliant; a remarked-but-unreviewed report counts
// toward "reports needing review"; a report landed today contributes to
// the data-quality trend's most recent day.
func TestHandleGetDashboard(t *testing.T) {
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

	overdueVessel := createTestVesselForReports(t, s, 80)
	enrollTestVessel(t, s, overdueVessel.ID)
	onTimeVessel := createTestVesselForReports(t, s, 81)
	enrollTestVessel(t, s, onTimeVessel.ID)

	now := time.Now().UTC()
	landTestReport(t, s, overdueVessel.ID, "report-overdue", 1, now.Add(-30*time.Hour), domain.StateSubmitted)
	landTestReport(t, s, onTimeVessel.ID, "report-ontime", 1, now.Add(-1*time.Hour), domain.StateSubmitted)
	landTestReport(t, s, onTimeVessel.ID, "report-remarked", 1, now.Add(-2*time.Hour), domain.StateRemarked)

	// A real schema-mandatory field left empty so today's data-quality
	// trend point has a real, non-zero error count to check for —
	// landTestReport's own reports are all schema "log-abstract", which
	// has no published version in this test server (newTestServer seeds
	// no curated schemas), so evaluateListRowHealth would silently fall
	// back to {0,0} for them (by design — see its own doc comment).
	trendSchemaName := "test-dashboard-trend-" + t.Name()
	content := buildSchemaJSON(t, trendSchemaName, "1", testSchemaField("Required_Field", schema.FieldTypeText, true))
	publishTestSchemaVersion(t, s, trendSchemaName, "1", "companyEdited", content)
	trendReport := &domain.Report{
		ReportID: "report-trend", VersionNo: 1, SchemaName: trendSchemaName, EventType: "Departure",
		EventTime: now, State: domain.StateSubmitted, Fields: map[string]any{},
	}
	if err := s.st.UpsertReportVersion(context.Background(), onTimeVessel.ID, trendReport, "1", now); err != nil {
		t.Fatalf("UpsertReportVersion (trend): %v", err)
	}

	rec := c.do(http.MethodGet, "/api/dashboard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/dashboard: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[dashboardView](t, rec)

	if got.EnrolledVesselCount < 2 {
		t.Errorf("EnrolledVesselCount = %d, want at least 2", got.EnrolledVesselCount)
	}
	if got.CompliancePercent >= 100 {
		t.Errorf("CompliancePercent = %v, want < 100 (one enrolled vessel is overdue)", got.CompliancePercent)
	}

	var foundOverdue bool
	for _, v := range got.OverdueVessels {
		if v.VesselID == overdueVessel.ID {
			foundOverdue = true
			if v.OverdueHours < 12 {
				t.Errorf("overdue vessel OverdueHours = %v, want >= 12", v.OverdueHours)
			}
		}
		if v.VesselID == onTimeVessel.ID {
			t.Errorf("on-time vessel %s appeared in OverdueVessels", onTimeVessel.ID)
		}
	}
	if !foundOverdue {
		t.Errorf("OverdueVessels = %+v, want the overdue vessel present", got.OverdueVessels)
	}
	if got.OverdueVesselCount < 1 {
		t.Errorf("OverdueVesselCount = %d, want at least 1", got.OverdueVesselCount)
	}

	if got.ReportsNeedingReview < 1 {
		t.Errorf("ReportsNeedingReview = %d, want at least 1 (the remarked, unreviewed report)", got.ReportsNeedingReview)
	}

	if len(got.DataQualityTrend) != dashboardTrendDays {
		t.Fatalf("len(DataQualityTrend) = %d, want %d", len(got.DataQualityTrend), dashboardTrendDays)
	}
	today := now.Format("2006-01-02")
	last := got.DataQualityTrend[len(got.DataQualityTrend)-1]
	if last.Date != today {
		t.Errorf("last trend point Date = %q, want today %q", last.Date, today)
	}
	if last.Errors == 0 && last.Warnings == 0 {
		t.Error("today's trend point has zero errors and warnings, want the minimally-filled reports landed today to show up")
	}

	// No log-abstract schema is published in this test server (this
	// file's own comment above, same reason the trend point above needed
	// its own synthetic schema) — operations overview must degrade to
	// report counts only rather than erroring the whole dashboard, the
	// same resilience loadSchemaHealthContext's "missing schema" path
	// already established for the data-quality trend.
	if got.OperationsPeriodDays != dashboardOperationsDefaultDays {
		t.Errorf("OperationsPeriodDays = %d, want default %d", got.OperationsPeriodDays, dashboardOperationsDefaultDays)
	}
	var sawOverdueVesselOps bool
	for _, row := range got.OperationsOverview {
		if row.VesselID == overdueVessel.ID {
			sawOverdueVesselOps = true
			if row.ReportCount < 1 {
				t.Errorf("operations row for overdue vessel ReportCount = %d, want at least 1", row.ReportCount)
			}
		}
	}
	if !sawOverdueVesselOps {
		t.Errorf("OperationsOverview = %+v, want a row for the vessel with a landed log-abstract report", got.OperationsOverview)
	}
}

// TestDashboardOperationsOverview_SumsDistanceAndConsumption is the real,
// schema-backed exercise of architecture 16's "operations overview" —
// TestHandleGetDashboard above only proves the degrade-gracefully path
// (no log-abstract schema published), this proves the actual
// summing-per-vessel behavior once a real schema exists.
func TestDashboardOperationsOverview_SumsDistanceAndConsumption(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	mt := "mt"
	nm := "NM"
	content := buildSchemaJSON(t, "log-abstract", "test-ops-"+t.Name(),
		schema.Field{Name: "Distance", Label: "Distance over Ground", Type: schema.FieldTypeDecimal, Unit: &nm, Relevance: "test", Section: "test", AppliesToEvents: []string{"*"}},
		schema.Field{Name: "ME_Consumption_HFO", Label: "ME HFO", Type: schema.FieldTypeDecimal, Unit: &mt, Relevance: "test", Section: "test", AppliesToEvents: []string{"*"}},
		schema.Field{Name: "AE_Consumption_MGO", Label: "AE MGO", Type: schema.FieldTypeDecimal, Unit: &mt, Relevance: "test", Section: "test", AppliesToEvents: []string{"*"}},
	)
	publishTestSchemaVersion(t, s, "log-abstract", "test-ops-"+t.Name(), "companyEdited", content)

	v := createTestVesselForReports(t, s, 82)
	now := time.Now().UTC()
	for i, dist := range []float64{100.5, 200.0} {
		r := &domain.Report{
			ReportID: "ops-report", VersionNo: i + 1, SchemaName: "log-abstract", EventType: "Departure",
			EventTime: now.Add(-time.Duration(i) * time.Hour), State: domain.StateSubmitted,
			Fields: map[string]any{"Distance": dist, "ME_Consumption_HFO": 10.0, "AE_Consumption_MGO": 2.5},
		}
		if err := s.st.UpsertReportVersion(context.Background(), v.ID, r, "test-ops-"+t.Name(), now); err != nil {
			t.Fatalf("UpsertReportVersion: %v", err)
		}
	}
	// Only the latest version (version 2, Distance=200.0) should count —
	// ListReports (which this aggregates over) already only ever surfaces
	// the latest version of a report, same "not double-counted" guarantee
	// its other callers rely on.

	rec := c.do(http.MethodGet, "/api/dashboard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/dashboard: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[dashboardView](t, rec)

	var row *dashboardOperationsRow
	for i := range got.OperationsOverview {
		if got.OperationsOverview[i].VesselID == v.ID {
			row = &got.OperationsOverview[i]
		}
	}
	if row == nil {
		t.Fatalf("OperationsOverview = %+v, want a row for %s", got.OperationsOverview, v.ID)
	}
	if row.ReportCount != 1 {
		t.Errorf("ReportCount = %d, want 1 (only the latest version of the one report)", row.ReportCount)
	}
	if row.TotalDistanceNM != 200.0 {
		t.Errorf("TotalDistanceNM = %v, want 200 (the latest version's value, not summed across versions)", row.TotalDistanceNM)
	}
	if row.TotalConsumptionMt != 12.5 {
		t.Errorf("TotalConsumptionMt = %v, want 12.5 (10.0 ME HFO + 2.5 AE MGO)", row.TotalConsumptionMt)
	}
}
