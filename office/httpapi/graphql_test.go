// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// graphqlRequestBody is the standard POST body shape any GraphQL client
// sends — gqlgen's generated handler expects exactly this.
type graphqlRequestBody struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

func doGraphQL(t *testing.T, s *Server, token, query string, variables map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(graphqlRequestBody{Query: query, Variables: variables})
	if err != nil {
		t.Fatalf("marshal graphql request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

// TestHandleGraphQLPlayground_RequiresAdmin covers 18.07.26 manual-test
// item 5's playground route: unauthenticated and non-admin requests must
// be rejected before the static playground HTML is ever served, and an
// Admin gets a real page back naming the real query endpoint.
func TestHandleGraphQLPlayground_RequiresAdmin(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)

	t.Run("unauthenticated is rejected", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/v1/graphql/playground", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("non-admin is forbidden", func(t *testing.T) {
		viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
		vc := newTestClient(t, s)
		loginAs(t, vc, viewer, "correct horse battery staple")
		rec := vc.do(http.MethodGet, "/api/v1/graphql/playground", nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("admin gets the playground page", func(t *testing.T) {
		admin := createTestUser2(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
		ac := newTestClient(t, s)
		loginAs(t, ac, admin, "correct horse battery staple")
		rec := ac.do(http.MethodGet, "/api/v1/graphql/playground", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("/api/v1/graphql")) {
			t.Error("playground body doesn't reference the real /api/v1/graphql endpoint")
		}
	})
}

// TestHandleGraphQL_Unauthenticated exercises the auth gate itself — no
// bearer token, or a garbage one, must never reach a resolver.
func TestHandleGraphQL_Unauthenticated(t *testing.T) {
	s := newTestServer(t)

	rec := doGraphQL(t, s, "", `{ reports { items { reportId } } }`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want %d, body %s", rec.Code, http.StatusUnauthorized, rec.Body)
	}

	rec = doGraphQL(t, s, "not-a-real-key", `{ reports { items { reportId } } }`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("garbage token: status = %d, want %d, body %s", rec.Code, http.StatusUnauthorized, rec.Body)
	}
}

// graphqlErrorsBody decodes just enough of a GraphQL response to check
// for a top-level errors array, independent of the exact success shape.
type graphqlErrorsBody struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data map[string]any `json:"data"`
}

// TestHandleGraphQL_FieldSelectionFilterAndPagination is the end-to-end
// exercise of architecture 13.2's whole point: an issued API key can run
// a GraphQL query selecting specific fields over a specific date range
// and get back correctly typed, paginated results.
func TestHandleGraphQL_FieldSelectionFilterAndPagination(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	schemaName := testFieldPolicySchemaName(t) // synthetic, scoped to this test
	content := buildSchemaJSON(t, schemaName, "1",
		testSchemaField("Cargo_Mt", schema.FieldTypeDecimal, false),
		testSchemaField("Voyage_Number", schema.FieldTypeText, true),
	)
	publishTestSchemaVersion(t, s, schemaName, "1", "projectCurated", content)

	vesselRec := c.do(http.MethodPost, "/api/vessels", createVesselRequest{IMO: "9074729", Name: "MV GraphQL Test", Type: "Bulk Carrier"})
	if vesselRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/vessels: status %d, body %s", vesselRec.Code, vesselRec.Body)
	}
	vessel := decodeBody[vesselView](t, vesselRec)
	// IMO 9074729 is a shared test-fixture convention across this
	// package (vessels_test.go, vesselgroups_test.go) — every user of it
	// self-cleans via t.Cleanup (see createTestVesselWithGroups' own
	// comment), so this must too or it permanently collides with the
	// next test in the same run that reuses it.
	t.Cleanup(func() {
		raw, err := sql.Open("pgx", testDSN(t))
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		_, _ = raw.ExecContext(context.Background(), `DELETE FROM vessels WHERE id = $1`, vessel.ID)
	})

	landTestReport := func(reportID string, eventTime time.Time, cargoMt float64, voyageNumber string) {
		t.Helper()
		r := &domain.Report{
			ReportID: reportID, VersionNo: 1, SchemaName: schemaName, EventType: "Departure",
			EventTime: eventTime, State: domain.StateSubmitted,
			Fields: map[string]any{"Cargo_Mt": cargoMt, "Voyage_Number": voyageNumber},
		}
		if err := s.st.UpsertReportVersion(context.Background(), vessel.ID, r, "1", time.Now().UTC()); err != nil {
			t.Fatalf("UpsertReportVersion(%s): %v", reportID, err)
		}
	}
	landTestReport("gql-report-1", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), 1200.5, "V-001")
	landTestReport("gql-report-2", time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), 1500.0, "V-002")
	landTestReport("gql-report-3", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 900.25, "V-003")

	keyRec := c.do(http.MethodPost, "/api/api-keys", createAPIKeyRequest{Label: "GraphQL test key"})
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("POST /api/api-keys: status %d, body %s", keyRec.Code, keyRec.Body)
	}
	key := decodeBody[createAPIKeyResponse](t, keyRec)

	t.Run("field selection returns exactly the requested fields, typed", func(t *testing.T) {
		query := `query($schema: String!) {
			reports(filter: { schemaName: $schema, eventType: "Departure" }, limit: 10) {
				items {
					reportId
					fields(names: ["Cargo_Mt"]) { name type numberValue stringValue }
				}
				hasNextPage
			}
		}`
		rec := doGraphQL(t, s, key.Token, query, map[string]any{"schema": schemaName})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body)
		}
		var resp graphqlErrorsBody
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v, body %s", err, rec.Body)
		}
		if len(resp.Errors) > 0 {
			t.Fatalf("graphql errors: %+v", resp.Errors)
		}
		reportsData, ok := resp.Data["reports"].(map[string]any)
		if !ok {
			t.Fatalf("data.reports missing or wrong shape: %+v", resp.Data)
		}
		items, ok := reportsData["items"].([]any)
		if !ok || len(items) != 3 {
			t.Fatalf("items = %+v, want 3 reports", reportsData["items"])
		}
		first := items[0].(map[string]any)
		fields, ok := first["fields"].([]any)
		if !ok || len(fields) != 1 {
			t.Fatalf("fields = %+v, want exactly 1 (only Cargo_Mt requested)", first["fields"])
		}
		fv := fields[0].(map[string]any)
		if fv["name"] != "Cargo_Mt" {
			t.Errorf("fields[0].name = %v, want Cargo_Mt (Voyage_Number must not appear, it wasn't requested)", fv["name"])
		}
		if fv["numberValue"] == nil {
			t.Error("fields[0].numberValue is nil, want the projected decimal value")
		}
	})

	t.Run("date range filter narrows results", func(t *testing.T) {
		query := `query($schema: String!) {
			reports(filter: { schemaName: $schema, dateFrom: "2026-06-10T00:00:00Z", dateTo: "2026-06-20T00:00:00Z" }) {
				items { reportId }
			}
		}`
		rec := doGraphQL(t, s, key.Token, query, map[string]any{"schema": schemaName})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var resp graphqlErrorsBody
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Errors) > 0 {
			t.Fatalf("graphql errors: %+v", resp.Errors)
		}
		reportsData := resp.Data["reports"].(map[string]any)
		items := reportsData["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("items = %+v, want exactly 1 (only gql-report-2 falls in range)", items)
		}
		if items[0].(map[string]any)["reportId"] != "gql-report-2" {
			t.Errorf("reportId = %v, want gql-report-2", items[0].(map[string]any)["reportId"])
		}
	})

	t.Run("pagination bounds the result set and reports hasNextPage", func(t *testing.T) {
		query := `query($schema: String!) {
			reports(filter: { schemaName: $schema }, limit: 2, offset: 0) {
				items { reportId }
				hasNextPage
			}
		}`
		rec := doGraphQL(t, s, key.Token, query, map[string]any{"schema": schemaName})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var resp graphqlErrorsBody
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(resp.Errors) > 0 {
			t.Fatalf("graphql errors: %+v", resp.Errors)
		}
		reportsData := resp.Data["reports"].(map[string]any)
		items := reportsData["items"].([]any)
		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2 (limit)", len(items))
		}
		if reportsData["hasNextPage"] != true {
			t.Error("hasNextPage = false, want true (3 total reports, limit 2)")
		}

		query2 := `query($schema: String!) {
			reports(filter: { schemaName: $schema }, limit: 2, offset: 2) {
				items { reportId }
				hasNextPage
			}
		}`
		rec2 := doGraphQL(t, s, key.Token, query2, map[string]any{"schema": schemaName})
		var resp2 graphqlErrorsBody
		if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		reportsData2 := resp2.Data["reports"].(map[string]any)
		items2 := reportsData2["items"].([]any)
		if len(items2) != 1 {
			t.Fatalf("len(items2) = %d, want 1 (3rd report, offset past the first page)", len(items2))
		}
		if reportsData2["hasNextPage"] != false {
			t.Error("hasNextPage = true on the last page, want false")
		}
	})
}
