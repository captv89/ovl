// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/schema"
)

func testSchemaName(t *testing.T) string {
	t.Helper()
	// Must be one of the meta-schema's fixed enum values — see
	// buildSchemaJSON's own doc comment. cargo-nomination is the
	// smallest curated schema, cheapest to build fixtures for.
	return "cargo-nomination"
}

func TestHandleListLatestSchemaVersions_IncludesPublishedVersion(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	name := testSchemaName(t)
	version := fmt.Sprintf("test-%s", t.Name())
	content := buildSchemaJSON(t, name, version, testSchemaField("Field_A", schema.FieldTypeText, false))
	publishTestSchemaVersion(t, s, name, version, "projectCurated", content)

	rec := c.do(http.MethodGet, "/api/schema-versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/schema-versions: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]schemaVersionSummaryView](t, rec)
	var found *schemaVersionSummaryView
	for i, v := range list {
		if v.SchemaName == name && v.Version == version {
			found = &list[i]
		}
	}
	if found == nil {
		t.Fatalf("published version %s@%s not found in %+v", name, version, list)
	}
	if found.FieldCount != 1 {
		t.Errorf("FieldCount = %d, want 1", found.FieldCount)
	}
}

func TestHandlePreviewSchemaUpload_RequiresConfigManager(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/schema-versions/"+testSchemaName(t)+"/preview", schemaUploadPreviewRequest{Content: "{}"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandlePreviewSchemaUpload_InvalidDocument(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	cm := createTestUser(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/schema-versions/"+testSchemaName(t)+"/preview", schemaUploadPreviewRequest{Content: `{"not":"a schema"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	preview := decodeBody[schemaUploadPreviewResponse](t, rec)
	if preview.Valid {
		t.Error("Valid = true for a document missing every required meta-schema property, want false")
	}
	if preview.Error == "" {
		t.Error("Error is empty, want a validation message")
	}
}

func TestHandlePreviewSchemaUpload_DiffsAgainstCurrentVersion(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	cm := createTestUser(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	name := testSchemaName(t)
	v1 := fmt.Sprintf("test-v1-%s", t.Name())
	v1Content := buildSchemaJSON(t, name, v1,
		testSchemaField("Field_A", schema.FieldTypeText, false),
		testSchemaField("Field_B", schema.FieldTypeText, false),
	)
	publishTestSchemaVersion(t, s, name, v1, "projectCurated", v1Content)

	v2 := fmt.Sprintf("test-v2-%s", t.Name())
	v2Content := buildSchemaJSON(t, name, v2,
		testSchemaField("Field_A", schema.FieldTypeText, false),
		testSchemaField("Field_B", schema.FieldTypeWholeNumber, false), // type changed
		testSchemaField("Field_C", schema.FieldTypeText, false),        // added
	)

	rec := c.do(http.MethodPost, "/api/schema-versions/"+name+"/preview", schemaUploadPreviewRequest{Content: string(v2Content)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	preview := decodeBody[schemaUploadPreviewResponse](t, rec)
	if !preview.Valid {
		t.Fatalf("Valid = false, want true; error: %s", preview.Error)
	}
	if preview.Diff == nil {
		t.Fatal("Diff is nil, want a diff against the current published version")
	}
	if len(preview.Diff.Added) != 1 || preview.Diff.Added[0].Name != "Field_C" {
		t.Errorf("Diff.Added = %+v, want [Field_C]", preview.Diff.Added)
	}
	if len(preview.Diff.TypeChanged) != 1 || preview.Diff.TypeChanged[0] != "Field_B" {
		t.Errorf("Diff.TypeChanged = %+v, want [Field_B]", preview.Diff.TypeChanged)
	}

	// Nothing should be persisted by a preview.
	if _, err := s.st.GetSchemaVersion(t.Context(), name, v2); err == nil {
		t.Error("preview persisted a new schema version, want no store write")
	}
}

func TestHandlePublishSchemaVersion_CreatesVersionAndGatesRole(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	cm := createTestUser2(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple 2")

	name := testSchemaName(t)
	version := fmt.Sprintf("test-%s", t.Name())
	content := buildSchemaJSON(t, name, version, testSchemaField("Field_A", schema.FieldTypeText, false))

	loginAs(t, c, viewer, "correct horse battery staple")
	rec := c.do(http.MethodPost, "/api/schema-versions/"+name+"/publish", schemaPublishRequest{Version: version, Content: string(content)})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("publish as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}

	loginAs(t, c, cm, "correct horse battery staple 2")
	rec = c.do(http.MethodPost, "/api/schema-versions/"+name+"/publish", schemaPublishRequest{Version: version, Content: string(content)})
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish as Config Manager: status %d, body %s", rec.Code, rec.Body)
	}
	registerSchemaVersionCleanup(t, name, version)
	published := decodeBody[schemaVersionSummaryView](t, rec)
	if published.Version != version || published.FieldCount != 1 {
		t.Errorf("published = %+v, want version=%q fieldCount=1", published, version)
	}

	sv, err := s.st.GetSchemaVersion(t.Context(), name, version)
	if err != nil {
		t.Fatalf("GetSchemaVersion after publish: %v", err)
	}
	if sv.Source != "projectCurated" && sv.Source != "companyEdited" {
		t.Errorf("Source = %q, want a valid source", sv.Source)
	}
}
