// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/compliance"
	"github.com/captv89/ovl/pkg/validation"
)

// scopeView is the JSON shape for a compliance.Scope, shared across
// profiles/cadence/rule-severities/bundle-assignments (design handoff
// B7: every one of these is "assignable fleet-wide, per group or per
// vessel").
type scopeView struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

func toScopeView(s compliance.Scope) scopeView {
	return scopeView{Type: string(s.Type), Key: s.Key}
}

func (v scopeView) toScope() (compliance.Scope, error) {
	switch compliance.ScopeType(v.Type) {
	case compliance.ScopeFleet:
		return compliance.FleetScope(), nil
	case compliance.ScopeGroup:
		return compliance.GroupScope(v.Key)
	case compliance.ScopeVessel:
		return compliance.VesselScope(v.Key)
	default:
		return compliance.Scope{Type: compliance.ScopeType(v.Type), Key: v.Key}, nil // let Scope.Validate reject it uniformly
	}
}

// --- Regulatory profiles ---

type profileAssignmentView struct {
	Scope     scopeView `json:"scope"`
	Profiles  []string  `json:"profiles"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toProfileAssignmentView(a *compliance.ProfileAssignment) profileAssignmentView {
	profiles := make([]string, len(a.Profiles))
	for i, p := range a.Profiles {
		profiles[i] = string(p)
	}
	return profileAssignmentView{Scope: toScopeView(a.Scope), Profiles: profiles, UpdatedAt: a.UpdatedAt}
}

// handleListProfileAssignments serves design handoff B7's regulatory
// profile toggle cards, current state across every scope. Viewable by
// any authenticated user; only Config Manager may save (B7: "Roles:
// Config Manager").
func (s *Server) handleListProfileAssignments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListProfileAssignments(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]profileAssignmentView, len(list))
	for i, a := range list {
		out[i] = toProfileAssignmentView(a)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type saveProfileAssignmentRequest struct {
	Scope    scopeView `json:"scope"`
	Profiles []string  `json:"profiles"`
}

func (s *Server) handleSaveProfileAssignment(w http.ResponseWriter, r *http.Request) {
	if !s.requireConfigManager(w, r) {
		return
	}
	var req saveProfileAssignmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, err := req.Scope.toScope()
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	profiles := make([]validation.RegulatoryProfile, len(req.Profiles))
	for i, p := range req.Profiles {
		profiles[i] = validation.RegulatoryProfile(p)
	}
	a, err := compliance.NewProfileAssignment(scope, profiles)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.SaveProfileAssignment(r.Context(), a); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toProfileAssignmentView(a))
}

// --- Cadence rules ---

type cadenceRuleView struct {
	Scope                  scopeView `json:"scope"`
	MinReportIntervalHours float64   `json:"minReportIntervalHours"`
	MaxGapHours            float64   `json:"maxGapHours"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

func toCadenceRuleView(r *compliance.CadenceRule) cadenceRuleView {
	return cadenceRuleView{
		Scope: toScopeView(r.Scope), MinReportIntervalHours: r.MinReportIntervalHours,
		MaxGapHours: r.MaxGapHours, UpdatedAt: r.UpdatedAt,
	}
}

func (s *Server) handleListCadenceRules(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListCadenceRules(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]cadenceRuleView, len(list))
	for i, rule := range list {
		out[i] = toCadenceRuleView(rule)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type saveCadenceRuleRequest struct {
	Scope                  scopeView `json:"scope"`
	MinReportIntervalHours float64   `json:"minReportIntervalHours"`
	MaxGapHours            float64   `json:"maxGapHours"`
}

func (s *Server) handleSaveCadenceRule(w http.ResponseWriter, r *http.Request) {
	if !s.requireConfigManager(w, r) {
		return
	}
	var req saveCadenceRuleRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, err := req.Scope.toScope()
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule, err := compliance.NewCadenceRule(scope, req.MinReportIntervalHours, req.MaxGapHours)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.SaveCadenceRule(r.Context(), rule); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toCadenceRuleView(rule))
}

// --- Rule severities ---

// handleListOverridableRules serves the fixed rule catalog design
// handoff B7's severity table is built from: compliance.OverridableRuleIDs
// (company may retune) and compliance.HardRuleIDs (locked at error).
func (s *Server) handleListOverridableRules(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string][]string{
		"overridable": compliance.OverridableRuleIDs,
		"hard":        compliance.HardRuleIDs,
	})
}

type ruleSeverityAssignmentView struct {
	Scope      scopeView         `json:"scope"`
	Severities map[string]string `json:"severities"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

func toRuleSeverityAssignmentView(a *compliance.RuleSeverityAssignment) ruleSeverityAssignmentView {
	severities := make(map[string]string, len(a.Severities))
	for k, v := range a.Severities {
		severities[k] = string(v)
	}
	return ruleSeverityAssignmentView{Scope: toScopeView(a.Scope), Severities: severities, UpdatedAt: a.UpdatedAt}
}

func (s *Server) handleListRuleSeverityAssignments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListRuleSeverityAssignments(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]ruleSeverityAssignmentView, len(list))
	for i, a := range list {
		out[i] = toRuleSeverityAssignmentView(a)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type saveRuleSeverityAssignmentRequest struct {
	Scope      scopeView         `json:"scope"`
	Severities map[string]string `json:"severities"`
}

func (s *Server) handleSaveRuleSeverityAssignment(w http.ResponseWriter, r *http.Request) {
	if !s.requireConfigManager(w, r) {
		return
	}
	var req saveRuleSeverityAssignmentRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	scope, err := req.Scope.toScope()
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := compliance.NewRuleSeverityAssignment(scope)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	for ruleID, sev := range req.Severities {
		if err := a.SetSeverity(ruleID, validation.Severity(sev)); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.st.SaveRuleSeverityAssignment(r.Context(), a); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toRuleSeverityAssignmentView(a))
}
