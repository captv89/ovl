// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/schema"
)

// TestHandleCreateCommercialReport exercises design handoff B8 end to
// end: a role gate (only Commercial Editor may author), a health-check
// rejection that persists nothing (a required field left empty), and a
// successful submission that lands as a real, listable Submitted report
// — the same /api/reports the general reports explorer already serves.
func TestHandleCreateCommercialReport(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)

	schemaName := "commercial-period"
	version := "test-" + t.Name()
	content := buildSchemaJSON(t, schemaName, version,
		testSchemaField("Period_Id", schema.FieldTypeText, true),
		testSchemaField("Description", schema.FieldTypeText, false),
	)
	publishTestSchemaVersion(t, s, schemaName, version, "companyEdited", content)

	v := createTestVesselForReports(t, s, 100)

	editor := createTestUser(t, s, auth.Roles{auth.RoleCommercialEditor}, "correct horse battery staple")
	loginAs(t, c, editor, "correct horse battery staple")

	viewer := createTestUser2(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")

	t.Run("role gate rejects a non-editor", func(t *testing.T) {
		vc := newTestClient(t, s)
		loginAs(t, vc, viewer, "correct horse battery staple")
		rec := vc.do(http.MethodPost, "/api/commercial/commercial-period/reports", createCommercialReportRequest{
			VesselID: v.ID, Fields: map[string]any{"Period_Id": "2026-Q3-TC"},
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("unknown schema name 404s", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/commercial/not-a-real-schema/reports", createCommercialReportRequest{VesselID: v.ID})
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("health check failure persists nothing", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/commercial/commercial-period/reports", createCommercialReportRequest{
			VesselID: v.ID, Fields: map[string]any{}, // Period_Id left empty
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusUnprocessableEntity, rec.Body)
		}
		resp := decodeBody[createCommercialReportResponse](t, rec)
		if resp.Report != nil {
			t.Errorf("Report = %+v, want nil (nothing should persist on a failed health check)", resp.Report)
		}
		var foundRequired bool
		for _, f := range resp.Findings {
			if f.Field == "Period_Id" && f.Severity == "error" {
				foundRequired = true
			}
		}
		if !foundRequired {
			t.Errorf("Findings = %+v, want a field.required error on Period_Id", resp.Findings)
		}

		listRec := c.do(http.MethodGet, "/api/reports?vesselId="+v.ID, nil)
		list := decodeBody[[]reportListItemView](t, listRec)
		if len(list) != 0 {
			t.Errorf("reports after a failed health check = %+v, want none persisted", list)
		}
	})

	t.Run("a filled form submits successfully and is listable", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/commercial/commercial-period/reports", createCommercialReportRequest{
			VesselID: v.ID, Fields: map[string]any{"Period_Id": "2026-Q3-TC"},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body)
		}
		resp := decodeBody[createCommercialReportResponse](t, rec)
		if resp.Report == nil || resp.Report.State != "submitted" {
			t.Fatalf("Report = %+v, want State=submitted", resp.Report)
		}

		listRec := c.do(http.MethodGet, "/api/reports?vesselId="+v.ID, nil)
		list := decodeBody[[]reportListItemView](t, listRec)
		var found bool
		for _, r := range list {
			if r.ReportID == resp.Report.ReportID {
				found = true
				if r.SchemaName != schemaName || r.State != "submitted" || r.EventType != "Commercial Period" {
					t.Errorf("listed row = %+v, want schema=%s state=submitted eventType=%q", r, schemaName, "Commercial Period")
				}
			}
		}
		if !found {
			t.Errorf("newly submitted commercial report not found in %+v", list)
		}
	})
}
