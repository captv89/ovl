// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/schema"
)

func TestHandleGetFieldPolicy_DefaultsToEmptyOnFreshVersion(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	name := testFieldPolicySchemaName(t)
	version := fmt.Sprintf("test-%s", t.Name())
	content := buildSchemaJSON(t, name, version,
		testSchemaField("Field_A", schema.FieldTypeText, false),
		testSchemaField("Field_B", schema.FieldTypeText, true), // schemaMandatory
	)
	publishTestSchemaVersion(t, s, name, version, "projectCurated", content)

	rec := c.do(http.MethodGet, "/api/field-policies/"+name, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/field-policies/{name}: status %d, body %s", rec.Code, rec.Body)
	}
	view := decodeBody[fieldPolicyView](t, rec)
	if view.Version != version {
		t.Errorf("Version = %q, want %q", view.Version, version)
	}
	if len(view.Fields) != 2 {
		t.Fatalf("Fields = %+v, want 2 entries", view.Fields)
	}
	if len(view.Policy) != 0 || len(view.Prefill) != 0 {
		t.Errorf("Policy/Prefill = %+v/%+v, want both empty on a fresh version", view.Policy, view.Prefill)
	}
	if view.Migration != nil {
		t.Errorf("Migration = %+v, want nil with no previous version", view.Migration)
	}
}

func TestHandleSaveFieldPolicy_PersistsAndGatesRoleAndLocksSchemaMandatory(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	cm := createTestUser2(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple 2")

	name := testFieldPolicySchemaName(t)
	version := fmt.Sprintf("test-%s", t.Name())
	content := buildSchemaJSON(t, name, version,
		testSchemaField("Field_A", schema.FieldTypeText, false),
		testSchemaField("Field_B", schema.FieldTypeText, true), // schemaMandatory
	)
	publishTestSchemaVersion(t, s, name, version, "projectCurated", content)

	loginAs(t, c, viewer, "correct horse battery staple")
	rec := c.do(http.MethodPut, "/api/field-policies/"+name, saveFieldPolicyRequest{Policy: map[string]string{"Field_A": "recommended"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("save as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}

	loginAs(t, c, cm, "correct horse battery staple 2")
	rec = c.do(http.MethodPut, "/api/field-policies/"+name, saveFieldPolicyRequest{
		Policy:  map[string]string{"Field_A": "recommended", "Field_B": "companyMandatory"}, // Field_B attempt should be dropped
		Prefill: map[string]string{"Field_A": "carryForward"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save as Config Manager: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[fieldPolicyView](t, rec)
	if len(saved.Fields) != 2 {
		t.Errorf("Fields = %+v, want the 2 schema fields (a real bug once shipped this as empty, breaking the frontend after save)", saved.Fields)
	}
	if saved.Policy["Field_A"] != "recommended" {
		t.Errorf("Policy[Field_A] = %q, want recommended", saved.Policy["Field_A"])
	}
	if _, ok := saved.Policy["Field_B"]; ok {
		t.Errorf("Policy[Field_B] was set, want the schemaMandatory field silently dropped: %+v", saved.Policy)
	}
	if saved.Prefill["Field_A"] != "carryForward" {
		t.Errorf("Prefill[Field_A] = %q, want carryForward", saved.Prefill["Field_A"])
	}

	// Re-fetching should show the saved state, and migration should
	// never be offered once something has been explicitly saved.
	rec = c.do(http.MethodGet, "/api/field-policies/"+name, nil)
	reloaded := decodeBody[fieldPolicyView](t, rec)
	if reloaded.Policy["Field_A"] != "recommended" {
		t.Errorf("reloaded Policy[Field_A] = %q, want recommended", reloaded.Policy["Field_A"])
	}
	if reloaded.Migration != nil {
		t.Errorf("Migration = %+v, want nil once a policy has been explicitly saved", reloaded.Migration)
	}
}

func TestHandleGetFieldPolicy_ProposesMigrationFromPreviousVersion(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	cm := createTestUser(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	name := testFieldPolicySchemaName(t)
	v1 := fmt.Sprintf("test-v1-%s", t.Name())
	v1Content := buildSchemaJSON(t, name, v1,
		testSchemaField("Field_A", schema.FieldTypeText, false),
		testSchemaField("Field_B", schema.FieldTypeText, false),
	)
	publishTestSchemaVersion(t, s, name, v1, "projectCurated", v1Content)

	// Author a policy against v1 before the new version exists.
	rec := c.do(http.MethodPut, "/api/field-policies/"+name, saveFieldPolicyRequest{Policy: map[string]string{"Field_A": "recommended"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save policy for v1: status %d, body %s", rec.Code, rec.Body)
	}

	v2 := fmt.Sprintf("test-v2-%s", t.Name())
	v2Content := buildSchemaJSON(t, name, v2,
		testSchemaField("Field_A", schema.FieldTypeText, false),
		testSchemaField("Field_B", schema.FieldTypeText, false),
		testSchemaField("Field_C", schema.FieldTypeText, false), // added in v2
	)
	publishTestSchemaVersion(t, s, name, v2, "companyEdited", v2Content)

	rec = c.do(http.MethodGet, "/api/field-policies/"+name, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET field policy for v2: status %d, body %s", rec.Code, rec.Body)
	}
	view := decodeBody[fieldPolicyView](t, rec)
	if view.Version != v2 {
		t.Fatalf("Version = %q, want %q", view.Version, v2)
	}
	if view.Migration == nil {
		t.Fatal("Migration is nil, want a proposed migration from v1")
	}
	if view.Migration.FromVersion != v1 {
		t.Errorf("Migration.FromVersion = %q, want %q", view.Migration.FromVersion, v1)
	}
	if len(view.Migration.NewFields) != 1 || view.Migration.NewFields[0] != "Field_C" {
		t.Errorf("Migration.NewFields = %+v, want [Field_C]", view.Migration.NewFields)
	}
	if view.Policy["Field_A"] != "recommended" {
		t.Errorf("carried-forward Policy[Field_A] = %q, want recommended", view.Policy["Field_A"])
	}
}

// A schema with no saved field-policy assignments must serialise the list
// as [] and never JSON null — the frontend reads assignments.length
// directly, so a null body crashed the whole field-policy editor the
// moment such a schema was selected (Log Abstract, freshly seeded).
func TestHandleListFieldPolicyAssignments_EmptyIsArrayNotNull(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	admin := createTestUser(t, s, auth.Roles{auth.RoleAdmin}, "correct horse battery staple")
	loginAs(t, c, admin, "correct horse battery staple")

	name := testFieldPolicySchemaName(t)
	version := fmt.Sprintf("test-%s", t.Name())
	content := buildSchemaJSON(t, name, version,
		testSchemaField("Field_A", schema.FieldTypeText, false),
	)
	publishTestSchemaVersion(t, s, name, version, "projectCurated", content)

	rec := c.do(http.MethodGet, "/api/field-policies/"+name+"/assignments", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET assignments: status %d, body %s", rec.Code, rec.Body)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("assignments body = %q, want %q (a nil slice serialises to null and crashes the editor)", body, "[]")
	}
	out := decodeBody[[]fieldPolicyAssignmentView](t, rec)
	if len(out) != 0 {
		t.Errorf("assignments = %+v, want empty", out)
	}
}
