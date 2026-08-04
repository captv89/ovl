// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/pkg/configwire"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/validation"
	"github.com/captv89/ovl/vessel/auth"
	"github.com/captv89/ovl/vessel/bootstrap"
	"github.com/captv89/ovl/vessel/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newLoggedInTestServer bootstraps a configured vessel (mode + Master
// account) and returns a client already authenticated as that Master,
// ready for report endpoints.
func newLoggedInTestServer(t *testing.T) (*Server, *testClient) {
	t.Helper()
	s := newTestServer(t)
	c := newTestClient(t, s)
	dataDir := filepath.Join(t.TempDir(), "data")
	if rec := c.do(http.MethodPost, "/api/setup/mode", setupModeRequest{Mode: bootstrap.ModeStandalone, DataDir: dataDir}); rec.Code != http.StatusOK {
		t.Fatalf("setup/mode: status %d, body %s", rec.Code, rec.Body)
	}
	c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{Skip: true})
	if rec := c.do(http.MethodPost, "/api/setup/master", setupMasterRequest{Username: "master", Password: "correct horse battery staple"}); rec.Code != http.StatusCreated {
		t.Fatalf("setup/master: status %d, body %s", rec.Code, rec.Body)
	}
	return s, c
}

// commercialPeriodFields returns a fully valid, health-check-clean field
// set for the commercial-period schema (6 fields, 4 schema-mandatory,
// none of them participating in any plausibility rule) — the smallest
// curated schema, chosen so lifecycle tests don't need to hand-author a
// 409-field Log Abstract report just to reach a clean check.
func commercialPeriodFields() map[string]any {
	return map[string]any{
		"IMO":          9876543.0,
		"Period_Id":    "P-2026-01",
		"Period_Start": "2026-01-01 00:00",
		"Period_End":   "2026-01-31 23:59",
	}
}

func createTestReport(t *testing.T, c *testClient, eventTime time.Time, fields map[string]any) reportView {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/reports", createReportRequest{
		SchemaName: "commercial-period",
		EventType:  "Other event",
		EventTime:  eventTime,
		Fields:     fields,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/reports: status %d, body %s", rec.Code, rec.Body)
	}
	return decodeBody[reportView](t, rec)
}

func TestHandleCreateAndGetReport(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if created.State != "draft" || created.SchemaName != "commercial-period" || created.VersionNo != 1 {
		t.Fatalf("created report = %+v, want state=draft schemaName=commercial-period versionNo=1", created)
	}
	if created.CreatedBy != "master" {
		t.Errorf("CreatedBy = %q, want master", created.CreatedBy)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/reports/{id}: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[reportView](t, rec)
	if got.ReportID != created.ReportID || got.Fields["Period_Id"] != "P-2026-01" {
		t.Errorf("GET report = %+v, want matching the created one", got)
	}
}

// TestHandleListReportEvents_ChronologicalAcrossLifecycle exercises the
// audit trail read side (design handoff A7, architecture 14): every
// lifecycle transition already persists an event (created earlier this
// phase), this endpoint just needs to serve them back in order.
func TestHandleListReportEvents_ChronologicalAcrossLifecycle(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d", rec.Code)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET events: status %d, body %s", rec.Code, rec.Body)
	}
	events := decodeBody[[]eventView](t, rec)
	if len(events) < 3 {
		t.Fatalf("len(events) = %d, want at least created/health_check_result/submitted", len(events))
	}
	wantOrder := []string{"created", "health_check_result", "submitted"}
	for i, want := range wantOrder {
		if string(events[i].Type) != want {
			t.Errorf("events[%d].Type = %q, want %q (chronological order)", i, events[i].Type, want)
		}
	}
}

func TestHandleListReportEvents_NotFound(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/reports/does-not-exist/events", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestHandleListReportVersions exercises design handoff A7's History tab
// data source (Phase 5 T6.3): every version of a corrected report,
// oldest first, so the UI can diff consecutive versions.
func TestHandleListReportVersions(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/correction", nil); rec.Code != http.StatusCreated {
		t.Fatalf("correction: status %d", rec.Code)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET versions: status %d, body %s", rec.Code, rec.Body)
	}
	versions := decodeBody[[]reportView](t, rec)
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0].VersionNo != 1 || versions[1].VersionNo != 2 {
		t.Errorf("VersionNo order = [%d, %d], want [1, 2]", versions[0].VersionNo, versions[1].VersionNo)
	}
}

func TestHandleListReportVersions_NotFound(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/reports/does-not-exist/versions", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleGetReport_NotFound(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/reports/does-not-exist", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteReport_RemovesDraft(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodDelete, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE draft: status %d, body %s", rec.Code, rec.Body)
	}

	if rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET after delete: status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleDeleteReport_AllowsReady covers the user's explicit scope
// answer: draft AND ready (checked but not yet submitted) are both
// deletable, matching the same editable-state boundary Save/Check
// already use (loadEditableReport).
func TestHandleDeleteReport_AllowsReady(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}

	rec := c.do(http.MethodDelete, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE ready report: status %d, body %s", rec.Code, rec.Body)
	}
}

func TestHandleDeleteReport_RejectsOnceSubmitted(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}

	rec := c.do(http.MethodDelete, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("DELETE submitted report: status %d, want %d", rec.Code, http.StatusConflict)
	}
	if rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil); rec.Code != http.StatusOK {
		t.Errorf("GET after rejected delete: status %d, want %d (report should survive)", rec.Code, http.StatusOK)
	}
}

// TestHandleDeleteReport_RejectsCorrectionDraft is the exact 2026-07-14
// manual-test repro: starting a correction on a submitted report
// re-enters state draft (architecture 8.1/8.2's designed correction
// flow), which used to pass loadEditableReport's draft/ready check the
// same as a never-submitted report and let "Delete draft" wipe the
// entire submitted history. VersionNo > 1 is what must block it, not
// state alone.
func TestHandleDeleteReport_RejectsCorrectionDraft(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/correction", nil); rec.Code != http.StatusCreated {
		t.Fatalf("start correction: status %d, body %s", rec.Code, rec.Body)
	}

	rec := c.do(http.MethodDelete, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("DELETE correction draft (v2): status %d, want %d, body %s", rec.Code, http.StatusConflict, rec.Body)
	}
	if rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil); rec.Code != http.StatusOK {
		t.Errorf("GET after rejected delete: status %d, want %d (report should survive)", rec.Code, http.StatusOK)
	}
}

// TestHandleDeleteReport_RejectsWhenLockedByAnotherUser mirrors
// handleSaveSection's own lock-conflict guard: another officer actively
// holding a section lock is a stronger reason to refuse than a single
// field save, since deletion removes the whole report they're editing.
func TestHandleDeleteReport_RejectsWhenLockedByAnotherUser(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c2 := loginSecondOfficer(t, s)
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/locks/details", nil); rec.Code != http.StatusOK {
		t.Fatalf("second officer acquire lock: status %d, body %s", rec.Code, rec.Body)
	}

	rec := c.do(http.MethodDelete, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("DELETE while locked by another user: status %d, want %d, body %s", rec.Code, http.StatusConflict, rec.Body)
	}

	// The lock holder deleting their own actively-held draft is fine —
	// only *someone else's* lock blocks deletion.
	if rec := c2.do(http.MethodDelete, "/api/reports/"+created.ReportID, nil); rec.Code != http.StatusOK {
		t.Errorf("DELETE by the lock's own holder: status %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
	}
}

func TestHandleDeleteReport_NotFound(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodDelete, "/api/reports/does-not-exist", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListReports(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	createTestReport(t, c, time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodGet, "/api/reports?schema=commercial-period", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[[]reportView](t, rec)
	if len(got) != 2 {
		t.Fatalf("len(reports) = %d, want 2", len(got))
	}
}

func TestHandleSaveSection_UpdatesFieldsAndDemotesReadyToDraft(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	checked := decodeBody[checkResponse](t, rec)
	if checked.Report.State != "ready" {
		t.Fatalf("state after check = %q, want ready (findings: %+v)", checked.Report.State, checked.Findings)
	}

	rec = c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{
		Section: "details",
		Changes: map[string]any{"Description": "revised scope"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[reportView](t, rec)
	if saved.Fields["Description"] != "revised scope" {
		t.Errorf("Fields[Description] = %v, want %q", saved.Fields["Description"], "revised scope")
	}
	if saved.State != "draft" {
		t.Errorf("state after editing a ready report = %q, want draft (a stale health check result must not survive an edit)", saved.State)
	}
}

func TestHandleSaveSection_RejectsOnceLocked(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}

	rec := c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{
		Section: "details",
		Changes: map[string]any{"Description": "too late"},
	})
	if rec.Code != http.StatusConflict {
		t.Errorf("PATCH on a submitted report: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

// createBunkerReport creates a bunker-report (schemas/ovd-3.13/
// bunker-report.json), the smallest curated schema with more than one
// section (header, fuelProperties) — needed so a concurrent-different-
// sections test has two real, distinct sections to target, unlike
// commercial-period's single "details" section.
func createBunkerReport(t *testing.T, c *testClient, eventTime time.Time) reportView {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/reports", createReportRequest{
		SchemaName: "bunker-report",
		EventType:  "Other event",
		EventTime:  eventTime,
		Fields:     map[string]any{},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/reports (bunker-report): status %d, body %s", rec.Code, rec.Body)
	}
	return decodeBody[reportView](t, rec)
}

// TestHandleSaveSection_ConcurrentDifferentSectionsBothSurvive is the
// regression test for the read-modify-write race in vessel/store.
// SaveReport: it UPSERTs the entire fields JSON blob per call
// (vessel/store/reports.go), and handleSaveSection loads the latest
// version, mutates it in memory, and saves the whole row back. Two
// officers concurrently saving *different, individually unlocked*
// sections of the same report (architecture 9.5's own worked example:
// deck locked by 2/O, engine free to claim) can still race here — last
// writer wins on the entire row, silently discarding the other
// officer's section, even though section soft locks (Phase 4 Step 6)
// never see a conflict since neither section is double-booked. This is
// what Server.writeMu (server.go) exists to prevent by serializing the
// load->mutate->save critical section across concurrent requests.
//
// A single iteration could pass by scheduling luck even without the
// fix, so this repeats across several fresh reports: with the fix every
// iteration must pass deterministically regardless of timing; without
// it, real SQLite file I/O gives enough of a window that at least one
// iteration reliably loses a field.
func TestHandleSaveSection_ConcurrentDifferentSectionsBothSurvive(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	sessionCookies := append([]*http.Cookie(nil), c.jar...)

	patch := func(reportID, section, field, value string) *httptest.ResponseRecorder {
		body, err := json.Marshal(saveSectionRequest{Section: section, Changes: map[string]any{field: value}})
		if err != nil {
			panic(err) // marshaling a static map literal cannot fail
		}
		req := httptest.NewRequest(http.MethodPatch, "/api/reports/"+reportID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, ck := range sessionCookies {
			req.AddCookie(ck)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	for i := range 30 {
		// Distinct EventTime per iteration: identical timestamps across
		// bunker-report's chain would trip cascade revalidation's
		// natural-key collision rule and invalidate the report, which
		// is a different mechanism entirely from the write race this
		// test targets.
		eventTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
		created := createBunkerReport(t, c, eventTime)

		var headerRec, fuelRec *httptest.ResponseRecorder
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			headerRec = patch(created.ReportID, "header", "BDN_Number", "BDN-A")
		}()
		go func() {
			defer wg.Done()
			<-start
			fuelRec = patch(created.ReportID, "fuelProperties", "Sustainability", "cert-B")
		}()
		close(start)
		wg.Wait()

		if headerRec.Code != http.StatusOK || fuelRec.Code != http.StatusOK {
			t.Fatalf("iteration %d: PATCH statuses = (%d, %d), want both 200 (bodies: %s / %s)",
				i, headerRec.Code, fuelRec.Code, headerRec.Body, fuelRec.Body)
		}

		rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil)
		got := decodeBody[reportView](t, rec)
		if got.Fields["BDN_Number"] != "BDN-A" || got.Fields["Sustainability"] != "cert-B" {
			t.Fatalf("iteration %d: fields = %+v, want both BDN_Number=BDN-A and Sustainability=cert-B to survive concurrent saves to different sections", i, got.Fields)
		}
	}
}

// TestHandleValidateReport_PreviewsUnsavedValuesWithoutPersisting exercises
// the live-validation endpoint (design handoff A5: "per-field validation on
// blur... plausibility checks run live"): it must reflect field values the
// caller passes in directly — not what's actually saved on the report — and
// must not write anything (no section_saved event, no field/state change).
func TestHandleValidateReport_PreviewsUnsavedValuesWithoutPersisting(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), map[string]any{})

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/validate", validateRequest{Fields: commercialPeriodFields()})
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[validateResponse](t, rec)
	for _, f := range got.Findings {
		if f.Severity == "error" {
			t.Errorf("finding %+v is an error despite every mandatory field being supplied in the preview payload", f)
		}
	}

	// The report itself must be unaffected: still empty Fields, still draft.
	rec = c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status %d, body %s", rec.Code, rec.Body)
	}
	current := decodeBody[reportView](t, rec)
	if len(current.Fields) != 0 {
		t.Errorf("Fields = %+v after a validate call, want unchanged (empty) — validate must not persist", current.Fields)
	}
	if current.State != "draft" {
		t.Errorf("state = %q after a validate call, want unchanged draft", current.State)
	}
}

func TestHandleCheckReport_ErrorsBlockReady(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	// Missing every schema-mandatory field.
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), map[string]any{})

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[checkResponse](t, rec)
	if got.Report.State != "draft" {
		t.Errorf("state = %q, want draft (errors present)", got.Report.State)
	}
	if len(got.Findings) == 0 {
		t.Fatal("Findings is empty, want at least the 4 missing-mandatory-field errors")
	}
	errCount := 0
	for _, f := range got.Findings {
		if f.Severity == "error" {
			errCount++
		}
	}
	if errCount == 0 {
		t.Error("no error-severity findings, want several (IMO/Period_Id/Period_Start/Period_End all missing)")
	}
}

// TestHandleCheckReport_RegulatoryReadiness exercises the real curated
// commercial-period schema: every field on it is "recommended for voyage
// level verfication schemes" (the OVD xlsx's own text, typo and all), so
// an empty report should report the voyage-verification profile not ready
// with every field name listed missing, and filling them all in should
// flip it to ready — design handoff A6 item 3.
func TestHandleCheckReport_RegulatoryReadiness(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), map[string]any{})

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[checkResponse](t, rec)
	profile := findProfileReadiness(t, got.RegulatoryReadiness, "voyageVerification")
	if profile.Ready {
		t.Errorf("voyageVerification.Ready = true on an empty report, want false")
	}
	if len(profile.MissingFields) == 0 {
		t.Error("voyageVerification.MissingFields is empty, want every commercial-period field")
	}

	rec = c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{Section: "all", Changes: commercialPeriodFields()})
	if rec.Code != http.StatusOK {
		t.Fatalf("save-section: status %d, body %s", rec.Code, rec.Body)
	}
	// commercialPeriodFields doesn't set Exclude_From_Period or Description
	// (both optional, architecture-wise) — fill them too so the profile can
	// actually reach ready.
	rec = c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{Section: "all", Changes: map[string]any{"Exclude_From_Period": false, "Description": "test"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save-section: status %d, body %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	got = decodeBody[checkResponse](t, rec)
	profile = findProfileReadiness(t, got.RegulatoryReadiness, "voyageVerification")
	if !profile.Ready {
		t.Errorf("voyageVerification.Ready = false once every field is filled, want true (missing: %v)", profile.MissingFields)
	}
}

// TestHandleCheckReport_RegulatoryReadiness_HiddenFieldExcluded is the bug
// report's reproduction: a company config hiding a regulatory-relevant
// field must drop it from the health check's MissingFields count, not
// report it as missing forever since the crew can never fill in a field
// they cannot see (architecture 6.1's "field does not exist for the
// crew"). Before EvaluateRegulatoryReadiness learned about FieldPolicy,
// every hidden field on the schema still counted here.
func TestHandleCheckReport_RegulatoryReadiness_HiddenFieldExcluded(t *testing.T) {
	office, fake := newFakeOffice(t, "the-test-credential")

	bundleContent, err := json.Marshal(configwire.Bundle{
		WireVersion:        configwire.WireVersion,
		BundleID:           "bundle-hide-1",
		VersionNo:          1,
		PublishedAt:        time.Now().UTC(),
		RegulatoryProfiles: []string{"voyageVerification"},
		Schemas: []configwire.SchemaConfig{{
			SchemaName: "commercial-period",
			Version:    "3.13",
			// Exclude_From_Period, not Period_Id: schema-mandatory fields
			// can never be hidden (architecture 6.1, StateFor checks
			// schemaMandatory before the policy map), so hiding one of
			// those wouldn't exercise this path at all.
			Policy: map[string]string{"Exclude_From_Period": "hidden"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal configwire bundle: %v", err)
	}
	fake.pullResponse = &syncv1.PullInboxResponse{
		ConfigBundles: []*syncv1.ConfigBundle{{
			BundleId:    "bundle-hide-1",
			VersionNo:   1,
			ContentJson: bundleContent,
			PublishedAt: timestamppb.New(time.Now().UTC()),
		}},
		NextCursors: &syncv1.SyncCursors{ConfigBundleCursor: 1},
	}

	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodPost, "/api/setup/enrollment", setupEnrollmentRequest{OfficeURL: office.URL, Code: "SOME-CODE"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/setup/enrollment: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/sync/now", nil); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sync/now: status %d, body %s", rec.Code, rec.Body)
	}

	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), map[string]any{})

	rec = c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[checkResponse](t, rec)
	profile := findProfileReadiness(t, got.RegulatoryReadiness, "voyageVerification")
	for _, name := range profile.MissingFields {
		if name == "Exclude_From_Period" {
			t.Errorf("MissingFields = %v, want Exclude_From_Period excluded (hidden by company policy)", profile.MissingFields)
		}
	}
}

func findProfileReadiness(t *testing.T, all []profileReadinessView, profile string) profileReadinessView {
	t.Helper()
	for _, p := range all {
		if string(p.Profile) == profile {
			return p
		}
	}
	t.Fatalf("profile %q not present in %+v", profile, all)
	return profileReadinessView{}
}

func TestHandleSubmitReport_RequiresReadyAndPermission(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	// Not ready yet (never checked).
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusConflict {
		t.Errorf("submit before check: status %d, want %d", rec.Code, http.StatusConflict)
	}

	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)

	// A non-submit user (Second Officer, no canSubmit override) must be
	// forbidden, even though the report is ready.
	st := s.storeOrNil()
	officer, err := auth.NewUser("second-officer", "another long password", auth.RoleSecondOfficer)
	if err != nil {
		t.Fatalf("auth.NewUser: %v", err)
	}
	if err := st.CreateUser(context.Background(), officer); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	c2 := newTestClient(t, s)
	if rec := c2.do(http.MethodPost, "/api/auth/login", loginRequest{Username: "second-officer", Password: "another long password"}); rec.Code != http.StatusOK {
		t.Fatalf("login as second-officer: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c2.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusForbidden {
		t.Errorf("submit as non-canSubmit user: status %d, want %d", rec.Code, http.StatusForbidden)
	}

	// Master (always canSubmit) succeeds.
	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit as master: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[reportView](t, rec)
	if got.State != "submitted" || got.SubmittedBy != "master" || got.SubmittedAt == nil {
		t.Errorf("submitted report = %+v, want state=submitted submittedBy=master submittedAt set", got)
	}
}

// TestHandleSubmitReport_EnqueuesOutboxItems exercises architecture
// 11.2 step 1 from the producer side: submitting enqueues exactly the
// report version and its "submitted" audit event, in a form the office's
// PushOutbox can later consume.
func TestHandleSubmitReport_EnqueuesOutboxItems(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}

	items, err := s.storeOrNil().ListOutboxItems(context.Background())
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	var sawVersion, sawEvent bool
	for _, item := range items {
		if item.ReportID != created.ReportID || item.VersionNo != 1 {
			continue
		}
		switch item.Kind {
		case store.OutboxItemKindReportVersion:
			sawVersion = true
		case store.OutboxItemKindReportAuditEvent:
			sawEvent = true
			if item.ReportEventID == nil {
				t.Error("reportAuditEvent outbox item has a nil ReportEventID")
			}
		}
	}
	if !sawVersion {
		t.Error("no reportVersion outbox item for the submitted report")
	}
	if !sawEvent {
		t.Error("no reportAuditEvent outbox item for the submit")
	}
}

// TestHandleStartCorrection_CreatesDraftVersionTwo exercises architecture
// 8.1's "any post-submit change creates version N+1... Corrections
// re-enter the flow at draft" and design handoff A7's "Start correction" —
// no submit-permission gate (8.1: "no office approval required"), the
// original version's data preserved, and the new version immediately
// editable/re-checkable/re-submittable through the ordinary endpoints.
func TestHandleStartCorrection_CreatesDraftVersionTwo(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil)
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d", rec.Code)
	}

	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/correction", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start correction: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[reportView](t, rec)
	if got.ReportID != created.ReportID || got.VersionNo != 2 || got.State != "draft" {
		t.Fatalf("correction = %+v, want same reportId, versionNo=2, state=draft", got)
	}
	if got.Fields["Period_Id"] != "P-2026-01" {
		t.Errorf("correction Fields = %+v, want a defensive copy of the submitted version's fields", got.Fields)
	}

	// The new draft is editable through the ordinary save-section endpoint.
	rec = c.do(http.MethodPatch, "/api/reports/"+created.ReportID, saveSectionRequest{Section: "all", Changes: map[string]any{"Description": "corrected"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save-section on correction: status %d, body %s", rec.Code, rec.Body)
	}

	// GET now resolves to the correction (the latest version), not the
	// original submitted one.
	rec = c.do(http.MethodGet, "/api/reports/"+created.ReportID, nil)
	got = decodeBody[reportView](t, rec)
	if got.VersionNo != 2 || got.Fields["Description"] != "corrected" {
		t.Errorf("GET after correction = %+v, want the edited version 2", got)
	}
}

func TestHandleStartCorrection_RejectsDraft(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), map[string]any{})
	rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/correction", nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("start correction on a draft: status %d, want %d", rec.Code, http.StatusConflict)
	}
}

// TestCascadeRevalidation_NeverLocksADraft is the regression test for the
// bug this whole draft/committed split exists to fix: pressing Save draft
// or Check report part-way through entry flipped the report to
// invalidated, after which every further save came back 409 "report is
// invalidated and locked; start a correction to edit it" — the officer
// locked out of their own half-finished report, with a correction (a new
// version of something never submitted) the only way out.
//
// A draft is not in the chain, so it can neither be invalidated by
// cascade nor invalidate anything else. It stays a draft and stays
// editable no matter what the numbers say.
func TestCascadeRevalidation_NeverLocksADraft(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	sameMinute := time.Date(2026, 1, 15, 12, 0, 30, 0, time.UTC)

	first := createTestReport(t, c, sameMinute, commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+first.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check first: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+first.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit first: status %d, body %s", rec.Code, rec.Body)
	}

	// A second report at the same UTC minute as the committed first one:
	// a genuine continuity break, and the exact shape that used to
	// invalidate both.
	fields := commercialPeriodFields()
	fields["Period_Id"] = "P-2026-02"
	second := createTestReport(t, c, sameMinute.Add(10*time.Second), fields)
	if second.State != "draft" {
		t.Fatalf("second report state after create = %q, want draft", second.State)
	}

	// Save draft: still a draft, still editable.
	rec := c.do(http.MethodPatch, "/api/reports/"+second.ReportID, saveSectionRequest{
		Section: "Basic", Changes: map[string]any{"Description": "still typing"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save section on second: status %d, body %s", rec.Code, rec.Body)
	}
	if got := decodeBody[reportView](t, rec); got.State != "draft" {
		t.Errorf("second report state after save = %q, want draft", got.State)
	}

	// Check report: the collision is reported as an error-severity
	// finding — the officer is told, on the report they can still fix —
	// and the report stays a draft rather than being locked.
	rec = c.do(http.MethodPost, "/api/reports/"+second.ReportID+"/check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("check second: status %d, body %s", rec.Code, rec.Body)
	}
	checked := decodeBody[checkResponse](t, rec)
	if checked.Report.State != "draft" {
		t.Errorf("second report state after check = %q, want draft", checked.Report.State)
	}
	collision := false
	for _, f := range checked.Findings {
		if f.RuleID == validation.RuleTimestampUniqueness && f.Severity == validation.SeverityError {
			collision = true
		}
	}
	if !collision {
		t.Errorf("check findings = %+v, want an error-severity %s", checked.Findings, validation.RuleTimestampUniqueness)
	}

	// And the committed first report was not dragged down by a draft.
	rec = c.do(http.MethodGet, "/api/reports/"+first.ReportID, nil)
	if got := decodeBody[reportView](t, rec); got.State != "submitted" {
		t.Errorf("first report after cascade = %+v, want it still submitted", got)
	}
}

// TestCascadeRevalidation_InvalidatingAnAlreadySubmittedReportReSyncs
// covers a real bug found during manual testing: runCascade persisted a
// report's flip to invalidated locally (SaveReport + AppendEvent) but
// never called EnqueueReportVersion/EnqueueReportAuditEvent the way the
// submit path does, so a report office already knew about would silently
// diverge from vessel's own state forever. Only a report that had left
// draft/ready before being invalidated should re-sync — a still-draft
// report invalidated by cascade (covered above) has no office-side row to
// update at all, and must NOT be enqueued.
func TestCascadeRevalidation_InvalidatingAnAlreadySubmittedReportReSyncs(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	sameMinute := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	first := createTestReport(t, c, sameMinute, commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+first.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d, body %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+first.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d, body %s", rec.Code, rec.Body)
	}

	before, err := s.storeOrNil().ListOutboxItems(context.Background())
	if err != nil {
		t.Fatalf("ListOutboxItems (before): %v", err)
	}

	// A second *committed* report at the same UTC minute triggers
	// continuity.timestampUniqueness across the chain, invalidating the
	// first report via runCascade.
	//
	// It goes in through the store rather than through create/check/
	// submit because the health check now catches its own collision
	// pre-submit (TestCascadeRevalidation_NeverLocksADraft) and would
	// refuse to let it become a committed report at all — which is the
	// point of that half. Cascade's remaining job is the case the report
	// being edited cannot see: a change that breaks somebody *else*.
	// This is that case, set up directly.
	fields := commercialPeriodFields()
	fields["Period_Id"] = "P-2026-03-second"
	second, _, err := domain.NewReport("commercial-period", "Other event", sameMinute.Add(10*time.Second), fields, "master")
	if err != nil {
		t.Fatalf("NewReport: %v", err)
	}
	second.State = domain.StateSubmitted
	if err := s.storeOrNil().SaveReport(context.Background(), second); err != nil {
		t.Fatalf("SaveReport (colliding committed report): %v", err)
	}
	if err := s.runCascade(context.Background(), s.storeOrNil(), "commercial-period"); err != nil {
		t.Fatalf("runCascade: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+first.ReportID, nil)
	got := decodeBody[reportView](t, rec)
	if got.State != "invalidated" || got.InvalidatedFrom != "submitted" {
		t.Fatalf("first report after cascade = %+v, want state=invalidated invalidatedFrom=submitted", got)
	}

	after, err := s.storeOrNil().ListOutboxItems(context.Background())
	if err != nil {
		t.Fatalf("ListOutboxItems (after): %v", err)
	}
	newItems := after[len(before):]

	hasVersion, hasAuditEvent := false, false
	for _, item := range newItems {
		if item.ReportID != first.ReportID || item.VersionNo != first.VersionNo {
			continue
		}
		switch item.Kind {
		case store.OutboxItemKindReportVersion:
			hasVersion = true
		case store.OutboxItemKindReportAuditEvent:
			hasAuditEvent = true
		}
	}
	if !hasVersion {
		t.Error("no reportVersion outbox item enqueued for the cascade-invalidated, already-submitted report")
	}
	if !hasAuditEvent {
		t.Error("no reportAuditEvent outbox item enqueued for the cascade-invalidated, already-submitted report")
	}
}
