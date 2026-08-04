// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/validation"
)

// commercialSchemaLabels is architecture 7's "Data ownership per
// schema" table, the two rows entered on office rather than vessel
// ("Commercial Period | Office (Commercial Editor role) | Simple office
// form", same for Cargo Nomination) — the only two schemas this
// endpoint will ever accept, and the display label design handoff B8
// wants shown as each report's "event type" (these schemas have no
// OVD event concept of their own, unlike Log Abstract's Departure/Noon/
// Arrival).
var commercialSchemaLabels = map[string]string{
	"commercial-period": "Commercial Period",
	"cargo-nomination":  "Cargo Nomination",
}

// requireCommercialEditor is requireAdmin's counterpart for B8's
// office-authored report creation (architecture 12.2: Commercial
// Editor).
func (s *Server) requireCommercialEditor(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return nil, false
	}
	if !user.CanEditCommercialData() {
		httpjson.WriteError(w, http.StatusForbidden, "only Commercial Editor may author commercial data")
		return nil, false
	}
	return user, true
}

type createCommercialReportRequest struct {
	VesselID string         `json:"vesselId"`
	Fields   map[string]any `json:"fields"`
}

type createCommercialReportResponse struct {
	Report   *reportView   `json:"report,omitempty"`
	Findings []findingView `json:"findings"`
}

type findingView struct {
	RuleID   string `json:"ruleId"`
	Severity string `json:"severity"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

func toFindingViews(findings validation.Findings) []findingView {
	out := make([]findingView, len(findings))
	for i, f := range findings {
		out[i] = findingView{RuleID: f.RuleID, Severity: string(f.Severity), Field: f.Field, Message: f.Message}
	}
	return out
}

// handleCreateCommercialReport is design handoff B8's "single-page
// schema-driven form... with a health check before ready to push" —
// office-authored, not vessel-submitted (architecture 12.2), so there
// is no draft/save-progressively flow the way vessel's own ReportForm
// has: the whole form is submitted as one action, health-checked
// immediately (the same two rule families B3's health cell and vessel's
// own "Check report" use — office is the authoritative source of the
// field policy either evaluates against), and either lands as
// Submitted in one step or is rejected with the findings for the editor
// to fix and resubmit. Nothing is persisted on a failed health check —
// a real scope reduction from vessel's own draft-persists-through-
// health-check-failures behavior, flagged here rather than silently
// matched, since office/store's report_versions table (designed to
// land already-immutable synced versions, see migration 00014's own
// comment) has no equivalent of vessel/store's editable-draft rows or
// section locks to build that on top of without a much larger change.
func (s *Server) handleCreateCommercialReport(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireCommercialEditor(w, r)
	if !ok {
		return
	}
	schemaName := r.PathValue("schemaName")
	label, known := commercialSchemaLabels[schemaName]
	if !known {
		httpjson.WriteError(w, http.StatusNotFound, "unknown commercial schema")
		return
	}
	var req createCommercialReportRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	vessel, err := s.st.GetVessel(r.Context(), req.VesselID)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusBadRequest, "unknown vessel")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	hc := loadSchemaHealthContext(r.Context(), s.st, schemaName, req.VesselID, vessel.Groups)
	if hc.err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, hc.err.Error())
		return
	}

	report, createdEvent, err := domain.NewReport(schemaName, label, time.Now().UTC(), req.Fields, user.Username)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	vr := report.ToValidation()
	var findings validation.Findings
	findings = append(findings, validation.EvaluateFieldRules(vr, hc.schema, hc.policy, hc.events)...)
	findings = append(findings, validation.EvaluatePlausibilityRules(vr, hc.cfg, nil)...)

	readyEvent, markErr := report.MarkReady(findings)
	if markErr != nil {
		httpjson.WriteJSON(w, http.StatusUnprocessableEntity, createCommercialReportResponse{Findings: toFindingViews(findings)})
		return
	}
	submitEvent, err := report.Submit(user.Username)
	if err != nil {
		// MarkReady just set StateReady, so Submit's only precondition is
		// guaranteed met — a failure here means something else is wrong.
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now().UTC()
	if err := s.st.UpsertReportVersion(r.Context(), req.VesselID, report, hc.schema.Version, now); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, e := range []domain.Event{createdEvent, readyEvent, submitEvent} {
		if err := s.st.AppendReportAuditEvent(r.Context(), req.VesselID, e, now, "office"); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	view := toReportView(report)
	httpjson.WriteJSON(w, http.StatusCreated, createCommercialReportResponse{Report: &view, Findings: toFindingViews(findings)})
}
