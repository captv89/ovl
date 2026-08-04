// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// doCSV issues a GET against the CSV export with an optional bearer API key.
func doCSV(t *testing.T, s *Server, token, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

// TestHandleReportsCSV is architecture 13.3's CSV export end to end (audit
// §5): an issued API key downloads a schema's reports as an OVD CSV, the
// auth gate rejects anonymous callers, and the required schema parameter is
// enforced.
func TestHandleReportsCSV(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	schemaName := testFieldPolicySchemaName(t)
	content := buildSchemaJSON(t, schemaName, "1",
		testSchemaField("Cargo_Mt", schema.FieldTypeDecimal, false),
		testSchemaField("Voyage_Number", schema.FieldTypeText, true),
	)
	publishTestSchemaVersion(t, s, schemaName, "1", "projectCurated", content)

	vesselRec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{IMO: "9074729", Name: "MV CSV Test", Type: "Bulk Carrier"})
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

	landTestReport := func(reportID string, cargoMt float64, voyage string) {
		t.Helper()
		r := &domain.Report{
			ReportID: reportID, VersionNo: 1, SchemaName: schemaName, EventType: "Departure",
			EventTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), State: domain.StateSubmitted,
			Fields: map[string]any{"Cargo_Mt": cargoMt, "Voyage_Number": voyage},
		}
		if err := s.st.UpsertReportVersion(context.Background(), vessel.ID, r, "1", time.Now().UTC()); err != nil {
			t.Fatalf("UpsertReportVersion(%s): %v", reportID, err)
		}
	}
	landTestReport("csv-report-1", 1200.5, "V-001")
	landTestReport("csv-report-2", 900.25, "V-002")

	keyRec := c.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "CSV test key"})
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/api-keys: status %d, body %s", keyRec.Code, keyRec.Body)
	}
	key := decodeBody[createAPIKeyResponse](t, keyRec)

	t.Run("unauthenticated is rejected", func(t *testing.T) {
		rec := doCSV(t, s, "", "/api/v1/reports.csv?schema="+schemaName)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("missing schema parameter is a 400", func(t *testing.T) {
		rec := doCSV(t, s, key.Token, "/api/v1/reports.csv")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d, body %s", rec.Code, http.StatusBadRequest, rec.Body)
		}
	})

	t.Run("downloads the schema's reports as CSV", func(t *testing.T) {
		rec := doCSV(t, s, key.Token, "/api/v1/reports.csv?schema="+schemaName)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
			t.Errorf("Content-Type = %q, want text/csv", ct)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, schemaName+".csv") {
			t.Errorf("Content-Disposition = %q, want attachment named %s.csv", cd, schemaName)
		}
		lines := strings.Split(strings.TrimRight(rec.Body.String(), "\n"), "\n")
		if len(lines) != 3 { // header + 2 reports
			t.Fatalf("CSV has %d lines, want 3 (header + 2 rows):\n%s", len(lines), rec.Body.String())
		}
		if lines[0] != "Cargo_Mt,Voyage_Number" {
			t.Errorf("header = %q, want schema field names in order", lines[0])
		}
		body := rec.Body.String()
		if !strings.Contains(body, "V-001") || !strings.Contains(body, "V-002") {
			t.Errorf("CSV body missing expected voyage values:\n%s", body)
		}
	})
}
