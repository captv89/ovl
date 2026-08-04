// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"

	"github.com/captv89/ovl/office/auth"
)

func TestHandleProfileAssignments_SaveAndListAndGateRole(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	cm := createTestUser2(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple 2")

	loginAs(t, c, viewer, "correct horse battery staple")
	rec := c.do(http.MethodPut, "/api/compliance/profiles", saveProfileAssignmentRequest{
		Scope: scopeView{Type: "fleet"}, Profiles: []string{"mrv"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("save as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}

	loginAs(t, c, cm, "correct horse battery staple 2")
	rec = c.do(http.MethodPut, "/api/compliance/profiles", saveProfileAssignmentRequest{
		Scope: scopeView{Type: "fleet"}, Profiles: []string{"mrv", "dcs"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save as Config Manager: status %d, body %s", rec.Code, rec.Body)
	}
	saved := decodeBody[profileAssignmentView](t, rec)
	if len(saved.Profiles) != 2 {
		t.Errorf("Profiles = %+v, want 2 entries", saved.Profiles)
	}
	t.Cleanup(func() {
		_ = c.do(http.MethodPut, "/api/compliance/profiles", saveProfileAssignmentRequest{Scope: scopeView{Type: "fleet"}, Profiles: []string{}})
	})

	rec = c.do(http.MethodGet, "/api/compliance/profiles", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d, body %s", rec.Code, rec.Body)
	}
	list := decodeBody[[]profileAssignmentView](t, rec)
	found := false
	for _, a := range list {
		if a.Scope.Type == "fleet" && len(a.Profiles) == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("fleet-scope assignment not found in list: %+v", list)
	}
}

func TestHandleCadenceRules_SaveAndList(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	cm := createTestUser(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	scope := scopeView{Type: "group", Key: "TestHandleCadenceRules_SaveAndList-group"}
	rec := c.do(http.MethodPut, "/api/compliance/cadence", saveCadenceRuleRequest{
		Scope: scope, MinReportIntervalHours: 12, MaxGapHours: 6,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save: status %d, body %s", rec.Code, rec.Body)
	}
	t.Cleanup(func() {
		_ = c.do(http.MethodPut, "/api/compliance/cadence", saveCadenceRuleRequest{Scope: scope, MinReportIntervalHours: 24, MaxGapHours: 12})
	})
	saved := decodeBody[cadenceRuleView](t, rec)
	if saved.MinReportIntervalHours != 12 || saved.MaxGapHours != 6 {
		t.Errorf("saved = %+v, want min=12 max=6", saved)
	}

	rec = c.do(http.MethodGet, "/api/compliance/cadence", nil)
	list := decodeBody[[]cadenceRuleView](t, rec)
	found := false
	for _, r := range list {
		if r.Scope.Type == "group" && r.Scope.Key == scope.Key {
			found = true
			if r.MinReportIntervalHours != 12 || r.MaxGapHours != 6 {
				t.Errorf("listed rule = %+v, want min=12 max=6", r)
			}
		}
	}
	if !found {
		t.Errorf("group-scope cadence rule not found in list: %+v", list)
	}
}

func TestHandleListOverridableRules_ReturnsKnownRuleCatalog(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodGet, "/api/compliance/rules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	catalog := decodeBody[map[string][]string](t, rec)
	if len(catalog["overridable"]) == 0 {
		t.Error("overridable rule list is empty, want the known plausibility/continuity rule IDs")
	}
	if len(catalog["hard"]) == 0 {
		t.Error("hard rule list is empty, want consumption-scheme-exclusivity")
	}
}

func TestHandleRuleSeverities_SaveAndListAndRejectsHardRule(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	cm := createTestUser(t, s, auth.Roles{auth.RoleConfigManager}, "correct horse battery staple")
	loginAs(t, c, cm, "correct horse battery staple")

	scope := scopeView{Type: "fleet"}

	// The hard rule can never be overridden.
	rec := c.do(http.MethodPut, "/api/compliance/rule-severities", saveRuleSeverityAssignmentRequest{
		Scope:      scope,
		Severities: map[string]string{"plausibility.consumptionSchemeExclusivity": "warning"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("save hard rule override: status %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = c.do(http.MethodPut, "/api/compliance/rule-severities", saveRuleSeverityAssignmentRequest{
		Scope:      scope,
		Severities: map[string]string{"plausibility.impliedSpeed": "info"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save overridable rule: status %d, body %s", rec.Code, rec.Body)
	}
	t.Cleanup(func() {
		_ = c.do(http.MethodPut, "/api/compliance/rule-severities", saveRuleSeverityAssignmentRequest{Scope: scope, Severities: map[string]string{}})
	})
	saved := decodeBody[ruleSeverityAssignmentView](t, rec)
	if saved.Severities["plausibility.impliedSpeed"] != "info" {
		t.Errorf("Severities = %+v, want plausibility.impliedSpeed=info", saved.Severities)
	}

	rec = c.do(http.MethodGet, "/api/compliance/rule-severities", nil)
	list := decodeBody[[]ruleSeverityAssignmentView](t, rec)
	found := false
	for _, a := range list {
		if a.Scope.Type == "fleet" && a.Severities["plausibility.impliedSpeed"] == "info" {
			found = true
		}
	}
	if !found {
		t.Errorf("fleet-scope rule severity assignment not found in list: %+v", list)
	}
}
