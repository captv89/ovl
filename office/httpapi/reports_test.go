// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/office/fieldpolicy"
	"github.com/captv89/ovl/office/vessels"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/validation"
)

// landTestReport lands one report version directly through the store,
// bypassing PushOutbox (Phase 4's territory, already covered by
// office/syncservice's own tests) since these tests only need to
// exercise the read surface.
func landTestReport(t *testing.T, s *Server, vesselID, reportID string, versionNo int, eventTime time.Time, state domain.State) {
	t.Helper()
	r := &domain.Report{
		ReportID: reportID, VersionNo: versionNo, SchemaName: "log-abstract", EventType: "Departure",
		EventTime: eventTime, Fields: map[string]any{"IMO": float64(9074729)}, State: state,
	}
	if err := s.st.UpsertReportVersion(context.Background(), vesselID, r, "3.13", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}
}

// createTestVesselForReports mirrors office/store's own createTestVessel
// (unexported there, so not reusable from this package) — cleanup goes
// through a raw connection like createTestUser's, since office/store
// exposes no DeleteVessel.
func createTestVesselForReports(t *testing.T, s *Server, first int) *vessels.Vessel {
	t.Helper()
	imo := distinctIMOForHTTP(t, first)
	v, err := vessels.NewVessel(imo, t.Name(), "Bulk Carrier", nil)
	if err != nil {
		t.Fatalf("vessels.NewVessel: %v", err)
	}
	if err := s.st.CreateVessel(context.Background(), v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			t.Errorf("cleanup: open raw connection: %v", err)
			return
		}
		defer func() { _ = raw.Close() }()
		if _, err := raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, v.ID); err != nil {
			t.Errorf("cleanup: delete test vessel %s: %v", v.ID, err)
		}
	})
	return v
}

// distinctIMOForHTTP mirrors office/store's own distinctIMO helper
// (unexported there, so not reusable across packages) — a checksum-valid
// IMO distinct per caller id.
func distinctIMOForHTTP(t *testing.T, id int) string {
	t.Helper()
	digits := [6]int{9, 1, 7, (id / 10) % 10, id % 10, 3}
	sum := 0
	for i, weight := 0, 7; i < 6; i, weight = i+1, weight-1 {
		sum += digits[i] * weight
	}
	check := sum % 10
	imo := ""
	for _, d := range digits {
		imo += string(rune('0' + d))
	}
	imo += string(rune('0' + check))
	if err := vessels.ValidateIMO(imo); err != nil {
		t.Fatalf("distinctIMOForHTTP(%d) produced an invalid IMO %q: %v", id, imo, err)
	}
	return imo
}

func TestHandleListReports(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 60)

	landTestReport(t, s, v.ID, "report-x", 1, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	landTestReport(t, s, v.ID, "report-y", 1, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), domain.StateInvalidated)

	rec := c.do(http.MethodGet, "/api/reports", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/reports: status %d, body %s", rec.Code, rec.Body)
	}
	rows := decodeBody[[]reportListItemView](t, rec)
	found := map[string]bool{}
	for _, row := range rows {
		if row.VesselID == v.ID {
			found[row.ReportID] = true
		}
	}
	if !found["report-x"] || !found["report-y"] {
		t.Errorf("rows = %+v, want both report-x and report-y for vessel %s", rows, v.ID)
	}

	// Filter by vesselId narrows to just this vessel's reports.
	rec = c.do(http.MethodGet, "/api/reports?vesselId="+v.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/reports?vesselId=...: status %d, body %s", rec.Code, rec.Body)
	}
	rows = decodeBody[[]reportListItemView](t, rec)
	if len(rows) != 2 {
		t.Errorf("len(rows) = %d, want 2", len(rows))
	}

	// invalidatedOnly=true narrows to report-y.
	rec = c.do(http.MethodGet, "/api/reports?vesselId="+v.ID+"&invalidatedOnly=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/reports?invalidatedOnly=true: status %d, body %s", rec.Code, rec.Body)
	}
	rows = decodeBody[[]reportListItemView](t, rec)
	if len(rows) != 1 || rows[0].ReportID != "report-y" {
		t.Errorf("rows = %+v, want just report-y", rows)
	}
}

// TestHandleListReports_HealthCell exercises design handoff B3's health
// cell end to end: a schema-mandatory field left empty on the report
// counts as one error, an unmet FieldRecommended policy on another field
// counts as one warning — proving reportListItemView.Health is wired
// through store.ListReports' new Fields column, the schema/field-policy
// load, and evaluateListRowHealth's rule evaluation, not just that the
// JSON key exists.
func TestHandleListReports_HealthCell(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 62)

	schemaName := fmt.Sprintf("test-health-%s", t.Name())
	content := buildSchemaJSON(t, schemaName, "1",
		testSchemaField("Required_Field", schema.FieldTypeText, true),
		testSchemaField("Recommended_Field", schema.FieldTypeText, false),
	)
	publishTestSchemaVersion(t, s, schemaName, "1", "companyEdited", content)
	if err := s.st.SaveFieldPolicy(context.Background(), &fieldpolicy.SchemaFieldPolicy{
		Scope: compliance.FleetScope(), SchemaName: schemaName, SchemaVersion: "1",
		Policy: validation.FieldPolicy{"Recommended_Field": validation.FieldRecommended},
	}); err != nil {
		t.Fatalf("SaveFieldPolicy: %v", err)
	}

	r := &domain.Report{
		ReportID: "report-health", VersionNo: 1, SchemaName: schemaName, EventType: "Departure",
		EventTime: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), State: domain.StateSubmitted,
		Fields: map[string]any{}, // both fields left empty
	}
	if err := s.st.UpsertReportVersion(context.Background(), v.ID, r, "1", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports?vesselId="+v.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/reports: status %d, body %s", rec.Code, rec.Body)
	}
	rows := decodeBody[[]reportListItemView](t, rec)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if got := rows[0].Health; got.Errors != 1 || got.Warnings != 1 {
		t.Errorf("Health = %+v, want {Errors:1 Warnings:1}", got)
	}
}

func TestHandleGetReport(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 61)
	landTestReport(t, s, v.ID, "report-z", 1, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	landTestReport(t, s, v.ID, "report-z", 2, time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), domain.StateSynced)

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-z", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET report detail: status %d, body %s", rec.Code, rec.Body)
	}
	detail := decodeBody[reportDetailView](t, rec)
	if detail.Latest.VersionNo != 2 || detail.Latest.State != domain.StateSynced {
		t.Errorf("detail.Latest = %+v, want versionNo=2 state=synced", detail.Latest)
	}
	if len(detail.Versions) != 2 || detail.Versions[0] != 1 || detail.Versions[1] != 2 {
		t.Errorf("detail.Versions = %v, want [1, 2]", detail.Versions)
	}
	if detail.VesselIMO != v.IMO {
		t.Errorf("detail.VesselIMO = %q, want %q", detail.VesselIMO, v.IMO)
	}
}

func TestHandleGetReport_NotFound(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 62)

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/no-such-report", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET missing report detail: status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListReportEvents(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 63)
	e1 := domain.Event{ReportID: "report-w", VersionNo: 1, Type: domain.EventSubmitted, Actor: "master", At: time.Now().UTC()}
	e2 := domain.Event{ReportID: "report-w", VersionNo: 1, Type: domain.EventInvalidated, At: time.Now().UTC().Add(time.Minute)}
	if err := s.st.AppendReportAuditEvent(context.Background(), v.ID, e1, time.Now().UTC(), "vessel"); err != nil {
		t.Fatalf("AppendReportAuditEvent: %v", err)
	}
	if err := s.st.AppendReportAuditEvent(context.Background(), v.ID, e2, time.Now().UTC(), "office"); err != nil {
		t.Fatalf("AppendReportAuditEvent: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-w/events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET report events: status %d, body %s", rec.Code, rec.Body)
	}
	events := decodeBody[[]eventView](t, rec)
	if len(events) != 2 || events[0].Type != domain.EventSubmitted || events[1].Type != domain.EventInvalidated {
		t.Errorf("events = %+v, want [submitted, invalidated]", events)
	}
	if events[0].Origin != "vessel" || events[1].Origin != "office" {
		t.Errorf("origins = [%q, %q], want [vessel, office]", events[0].Origin, events[1].Origin)
	}
}

func TestHandleListReportVersions(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 64)
	landTestReport(t, s, v.ID, "report-v", 1, time.Now().UTC(), domain.StateSubmitted)
	landTestReport(t, s, v.ID, "report-v", 2, time.Now().UTC(), domain.StateSynced)

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-v/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET report versions: status %d, body %s", rec.Code, rec.Body)
	}
	versions := decodeBody[[]reportView](t, rec)
	if len(versions) != 2 || versions[0].VersionNo != 1 || versions[1].VersionNo != 2 {
		t.Errorf("versions = %+v, want versionNo [1, 2]", versions)
	}
}

func TestHandleChat_PostAndList(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 66)
	landTestReport(t, s, v.ID, "report-chat-1", 1, time.Now().UTC(), domain.StateSubmitted)

	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-chat-1/chat", postChatRequest{Body: "any authenticated role may chat"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST chat: status %d, body %s", rec.Code, rec.Body)
	}
	posted := decodeBody[chatMessageView](t, rec)
	if posted.Body != "any authenticated role may chat" || posted.Direction != string(domain.ChatFromOffice) {
		t.Errorf("posted = %+v, want body set and direction=office", posted)
	}
	if posted.Sender != viewer.Username {
		t.Errorf("posted.Sender = %q, want %q", posted.Sender, viewer.Username)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-chat-1/chat", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET chat: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]chatMessageView](t, rec)
	if len(list) != 1 || list[0].ID != posted.ID {
		t.Errorf("list = %+v, want exactly the posted message", list)
	}
}

func TestHandleChat_Post_RejectsOverCapBody(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 67)

	overCap := strings.Repeat("a", domain.MaxChatBodyBytes+1)
	rec := c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-chat-2/chat", postChatRequest{Body: overCap})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST over-cap chat body: status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListReports_RequiresAuth(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	rec := c.do(http.MethodGet, "/api/reports", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/reports without login: status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
