// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
	"github.com/captv89/ovl/pkg/validation"
	"github.com/captv89/ovl/vessel/store"
)

// reportView is the JSON shape returned for a report version across every
// report endpoint.
type reportView struct {
	ReportID         string         `json:"reportId"`
	VersionNo        int            `json:"versionNo"`
	SchemaName       string         `json:"schemaName"`
	EventType        string         `json:"eventType"`
	EventTime        time.Time      `json:"eventTime"`
	Fields           map[string]any `json:"fields"`
	State            domain.State   `json:"state"`
	InvalidatedFrom  domain.State   `json:"invalidatedFrom,omitempty"`
	InvalidatedRules []string       `json:"invalidatedRules,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	CreatedBy        string         `json:"createdBy"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	SubmittedAt      *time.Time     `json:"submittedAt,omitempty"`
	SubmittedBy      string         `json:"submittedBy,omitempty"`
}

func toReportView(r *domain.Report) reportView {
	v := reportView{
		ReportID:         r.ReportID,
		VersionNo:        r.VersionNo,
		SchemaName:       r.SchemaName,
		EventType:        r.EventType,
		EventTime:        r.EventTime,
		Fields:           r.Fields,
		State:            r.State,
		InvalidatedFrom:  r.InvalidatedFrom,
		InvalidatedRules: r.InvalidatedRules,
		CreatedAt:        r.CreatedAt,
		CreatedBy:        r.CreatedBy,
		UpdatedAt:        r.UpdatedAt,
		SubmittedBy:      r.SubmittedBy,
	}
	if !r.SubmittedAt.IsZero() {
		t := r.SubmittedAt
		v.SubmittedAt = &t
	}
	return v
}

// findingView is the JSON shape for one pkg/validation.Finding.
type findingView struct {
	RuleID   string              `json:"ruleId"`
	Severity validation.Severity `json:"severity"`
	Field    string              `json:"field,omitempty"`
	Message  string              `json:"message"`
}

func toFindingViews(findings validation.Findings) []findingView {
	out := make([]findingView, len(findings))
	for i, f := range findings {
		out[i] = findingView{RuleID: f.RuleID, Severity: f.Severity, Field: f.Field, Message: f.Message}
	}
	return out
}

type createReportRequest struct {
	SchemaName string         `json:"schemaName"`
	EventType  string         `json:"eventType"`
	EventTime  time.Time      `json:"eventTime"`
	Fields     map[string]any `json:"fields"`
}

// handleCreateReport starts a new report version 1 in StateDraft
// (architecture 8.1/8.2, pkg/domain.NewReport).
func (s *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	var req createReportRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SchemaName == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "schemaName is required")
		return
	}
	report, event, err := domain.NewReport(req.SchemaName, req.EventType, req.EventTime, req.Fields, user.Username)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := st.SaveReport(r.Context(), report); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eventID, err := st.AppendEvent(r.Context(), event)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Architecture 14: the audit trail "spans vessel and office" for
	// every event type it lists, including `created` — not just
	// submit/resubmit. Previously only handleSubmitReport and
	// handleAcknowledgeFinding called EnqueueReportAuditEvent, so office
	// never saw a report's created/section_saved/health_check_result/
	// correction_started events at all, contradicting that spec (found
	// during manual-test review comparing vessel's and office's audit
	// trails for the same report side by side).
	if _, err := st.EnqueueReportAuditEvent(r.Context(), report.ReportID, report.VersionNo, eventID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A newly created report can itself collide with an existing one (a
	// late report inserted at an already-used timestamp, architecture
	// 8.3) or be the report that makes an existing one collide — run
	// cascade before responding so the returned state is never stale.
	if err := s.runCascade(r.Context(), st, report.SchemaName); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	report, err = st.GetLatestVersion(r.Context(), report.ReportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toReportView(report))
}

// handleGetReport returns the latest version of one report.
func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	report, err := st.GetLatestVersion(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toReportView(report))
}

// handleDeleteReport permanently removes a report that has never been
// submitted (2026-07-13 manual-test feedback: "there is no way to delete
// a form in draft status"). Open to any authenticated user, same as every
// other edit action on a draft/ready report (loadEditableReport enforces
// the same draft/ready boundary Save/Check already use, per the
// recommended default the user confirmed) — not restricted to the
// report's own creator. Purely a vessel-local operation and needs no
// office-side counterpart: a draft/ready report was never enqueued to the
// outbox (that only happens at Submit, handleSubmitReport), so nothing
// has synced anywhere for a deletion to undo. Logged to the application
// log in addition to store.DeleteReport's own deleted_report_log row (the
// user's explicit answer: keep both), per the same reasoning that row's
// migration comment gives — nothing else records that this report ever
// existed once its own audit trail is gone with it.
//
// VersionNo == 1 is the real deletability boundary, not loadEditableReport's
// draft/ready state check alone (2026-07-14 manual-test feedback: starting
// a correction on a submitted report — NewCorrection, architecture 8.1/8.2
// — re-enters state draft at VersionNo+1 by design, and that correction
// draft was passing this same state check, letting an already-submitted
// report's entire history be wiped via "Delete draft". The user's own
// distinguishing rule: "as long as the form is not submitted in the
// form's history, it should be deletable" — a report only ever reaches
// VersionNo > 1 via NewCorrection, which requires having left draft/ready
// at least once (lifecycle.go's own state guard), so VersionNo == 1 means
// exactly "never submitted."
func (s *Server) handleDeleteReport(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	reportID := r.PathValue("id")
	report, ok := s.loadEditableReport(w, r, st, reportID)
	if !ok {
		return
	}
	if report.VersionNo != 1 {
		httpjson.WriteError(w, http.StatusConflict, "report has a prior submitted version and cannot be deleted")
		return
	}
	// A section someone else currently holds means they're actively
	// editing this report right now — deleting out from under them would
	// be far more surprising than the same conflict handleSaveSection
	// already guards against for a single field change.
	for _, lock := range s.locks.snapshot(reportID) {
		if lock.UserID != user.ID {
			httpjson.WriteError(w, http.StatusConflict, fmt.Sprintf("section %q is locked by %s", lock.Section, lock.Username))
			return
		}
	}
	if err := st.DeleteReport(r.Context(), reportID, user.Username); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("report deleted", "reportId", report.ReportID, "schema", report.SchemaName, "eventType", report.EventType, "state", report.State, "deletedBy", user.Username)
	// Deleting a report can resolve another report's invalidation (e.g. it
	// was the natural-key collision causing it) — re-run cascade so that
	// shows up immediately rather than waiting for the next unrelated
	// mutation to trigger it.
	if err := s.runCascade(r.Context(), st, report.SchemaName); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// eventView is the JSON shape for one pkg/domain.Event (architecture 14's
// audit trail).
type eventView struct {
	VersionNo int              `json:"versionNo"`
	Type      domain.EventType `json:"type"`
	At        time.Time        `json:"at"`
	Actor     string           `json:"actor,omitempty"`
	Detail    map[string]any   `json:"detail,omitempty"`
}

func toEventViews(events []domain.Event) []eventView {
	out := make([]eventView, len(events))
	for i, e := range events {
		out[i] = eventView{VersionNo: e.VersionNo, Type: e.Type, At: e.At, Actor: e.Actor, Detail: e.Detail}
	}
	return out
}

// handleListReportEvents returns every audit event for one report,
// oldest first — design handoff A7's "Audit trail" tab (architecture 14:
// "chronological event list").
func (s *Server) handleListReportEvents(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	reportID := r.PathValue("id")
	if _, err := st.GetLatestVersion(r.Context(), reportID); errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := st.ListEvents(r.Context(), reportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toEventViews(events))
}

// handleListReportVersions returns every version of one report, oldest
// first — design handoff A7's History tab (Phase 5 T6.3) diffs
// consecutive versions from this.
func (s *Server) handleListReportVersions(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	reportID := r.PathValue("id")
	if _, err := st.GetLatestVersion(r.Context(), reportID); errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	versions, err := st.ListVersions(r.Context(), reportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]reportView, len(versions))
	for i, v := range versions {
		views[i] = toReportView(v)
	}
	httpjson.WriteJSON(w, http.StatusOK, views)
}

// handleListReports returns the latest version of every report for the
// ?schema= query parameter's schema name (design handoff A3/A4's report
// lists).
func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	schemaName := r.URL.Query().Get("schema")
	if schemaName == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "schema query parameter is required")
		return
	}
	reports, err := st.ListLatestReports(r.Context(), schemaName)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]reportView, len(reports))
	for i, rep := range reports {
		views[i] = toReportView(rep)
	}
	httpjson.WriteJSON(w, http.StatusOK, views)
}

type saveSectionRequest struct {
	Section string         `json:"section"`
	Changes map[string]any `json:"changes"`
}

// handleSaveSection applies field changes to a report still in
// draft/ready (architecture 9.5: "section saves are independent
// writes"). Rejected once the report is locked post-submit — the caller
// must start a correction first (a separate, not-yet-exposed endpoint).
func (s *Server) handleSaveSection(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	var req saveSectionRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	report, ok := s.loadEditableReport(w, r, st, r.PathValue("id"))
	if !ok {
		return
	}
	// The real backstop behind the client-side selection gate
	// (FormWizard already disables a locked section's nav button,
	// design handoff A5) — a stale tab, a second browser, or a lock
	// that expired mid-typing must never be trusted to have honored
	// that gate itself (architecture 9.5).
	if conflict, ok := s.lockConflict(report, req.Changes, user.ID); ok {
		httpjson.WriteJSON(w, http.StatusConflict, lockConflictResponse{
			Error: fmt.Sprintf("section %q is locked by %s", conflict.Section, conflict.Username),
			Lock:  toLockView(conflict),
		})
		return
	}
	event := report.SaveSection(req.Section, req.Changes, user.Username)
	if err := st.SaveReport(r.Context(), report); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eventID, err := st.AppendEvent(r.Context(), event)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Architecture 14: section_saved is one of the audit trail's cross-
	// side event types — see handleCreateReport's own comment on this
	// same gap.
	if _, err := st.EnqueueReportAuditEvent(r.Context(), report.ReportID, report.VersionNo, eventID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.runCascade(r.Context(), st, report.SchemaName); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Re-read: runCascade may have flipped this same report to invalidated.
	report, err = st.GetLatestVersion(r.Context(), report.ReportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toReportView(report))
}

type validateRequest struct {
	Fields map[string]any `json:"fields"`
}

type validateResponse struct {
	Findings []findingView `json:"findings"`
}

// handleValidateReport is a stateless preview of the health check
// (architecture 9.4/design handoff A5: "per-field validation on blur...
// plausibility checks run live") against caller-supplied field values —
// the officer's still-unsaved in-progress edits, not what's persisted.
// Unlike handleCheckReport it never writes anything (no SaveReport, no
// AppendEvent, no MarkReady state change), so it's cheap enough to call on
// every field blur or a short debounce while typing without spamming the
// audit trail with a section_saved/health_check_result event per keystroke.
// Runs the same two rule passes evaluateReport does for Check (field rules
// + plausibility, chain-aware rules excluded — those are cascade's job,
// see runCascade) so a live preview never shows something Check wouldn't.
func (s *Server) handleValidateReport(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	var req validateRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	report, err := st.GetLatestVersion(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	preview := *report
	preview.Fields = req.Fields
	findings, _, _, _, err := s.evaluateReport(r.Context(), st, &preview)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, validateResponse{Findings: toFindingViews(findings)})
}

// profileReadinessView is the JSON shape for one
// pkg/validation.ProfileReadiness (design handoff A6 item 3: regulatory
// readiness chips).
type profileReadinessView struct {
	Profile       validation.RegulatoryProfile `json:"profile"`
	Ready         bool                         `json:"ready"`
	MissingFields []string                     `json:"missingFields"`
}

func toProfileReadinessViews(readiness []validation.ProfileReadiness) []profileReadinessView {
	out := make([]profileReadinessView, len(readiness))
	for i, p := range readiness {
		missing := p.MissingFields
		if missing == nil {
			missing = []string{}
		}
		out[i] = profileReadinessView{Profile: p.Profile, Ready: p.Ready(), MissingFields: missing}
	}
	return out
}

// continuityImpactView names another report in the same chain that is
// currently invalidated (design handoff A6 item 4: continuity preview).
type continuityImpactView struct {
	ReportID         string    `json:"reportId"`
	EventType        string    `json:"eventType"`
	EventTime        time.Time `json:"eventTime"`
	InvalidatedRules []string  `json:"invalidatedRules"`
}

type checkResponse struct {
	Report              reportView             `json:"report"`
	Findings            []findingView          `json:"findings"`
	RegulatoryReadiness []profileReadinessView `json:"regulatoryReadiness"`
	ContinuityImpact    []continuityImpactView `json:"continuityImpact"`
}

// handleCheckReport runs the health check (architecture 9.4/10) and
// records the result via pkg/domain.MarkReady: no error-severity finding
// moves the report to StateReady, otherwise it stays/returns to
// StateDraft. Findings are always returned so the UI can show warnings
// even when the report is already ready.
func (s *Server) handleCheckReport(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	report, ok := s.loadEditableReport(w, r, st, r.PathValue("id"))
	if !ok {
		return
	}
	findings, sch, policy, events, err := s.evaluateReport(r.Context(), st, report)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// MarkReady's own error return (health check has errors) is expected,
	// routine control flow here, not a request failure — the response
	// carries the findings either way and the client decides what to show.
	event, _ := report.MarkReady(findings)
	if err := st.SaveReport(r.Context(), report); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eventID, err := st.AppendEvent(r.Context(), event)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Architecture 14: health_check_result is one of the audit trail's
	// cross-side event types — see handleCreateReport's own comment on
	// this same gap. Bounded, not noisy: the client only calls this
	// endpoint on an explicit "Check report" click (ReportForm.tsx's
	// handleCheck), never live per-keystroke.
	if _, err := st.EnqueueReportAuditEvent(r.Context(), report.ReportID, report.VersionNo, eventID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	readiness := validation.EvaluateRegulatoryReadiness(report.ToValidation(), sch.Fields, profilesFromBundle(s.appliedBundle(r.Context())), policy, events)
	impact, err := s.continuityImpact(r.Context(), st, report)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, checkResponse{
		Report:              toReportView(report),
		Findings:            toFindingViews(findings),
		RegulatoryReadiness: toProfileReadinessViews(readiness),
		ContinuityImpact:    impact,
	})
}

// continuityImpact lists other reports in report's chain that are currently
// StateInvalidated (design handoff A6 item 4: "if this report's values
// would invalidate later reports... show it here before submit"). Cascade
// revalidation (architecture 8.3) already runs — and persists — on every
// section save via runCascade, called by the vessel UI's Save/Check flow
// just before it calls this endpoint; this reads that already-computed
// state back rather than recomputing it, since re-deriving "would
// invalidate" independently here could disagree with what actually got
// persisted moments earlier.
func (s *Server) continuityImpact(ctx context.Context, st *store.Store, report *domain.Report) ([]continuityImpactView, error) {
	chain, err := st.ListChain(ctx, report.SchemaName)
	if err != nil {
		return nil, fmt.Errorf("list chain for continuity impact: %w", err)
	}
	// Non-nil even when empty: a nil slice marshals to JSON null, not [],
	// which would crash the frontend's continuityImpact.length check on the
	// (extremely common) no-impact case.
	out := []continuityImpactView{}
	for _, other := range chain {
		if other.ReportID == report.ReportID || other.State != domain.StateInvalidated {
			continue
		}
		out = append(out, continuityImpactView{
			ReportID:         other.ReportID,
			EventType:        other.EventType,
			EventTime:        other.EventTime,
			InvalidatedRules: other.InvalidatedRules,
		})
	}
	return out, nil
}

// handleSubmitReport transitions a StateReady report to StateSubmitted
// (architecture 8.1). Submit permission (Master, or any user flagged
// canSubmit — architecture 9.3) is checked here; pkg/domain.Submit only
// enforces the lifecycle precondition.
func (s *Server) handleSubmitReport(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	if !user.CanSubmitReports() {
		httpjson.WriteError(w, http.StatusForbidden, "you do not have permission to submit reports")
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	report, err := st.GetLatestVersion(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	event, err := report.Submit(user.Username)
	if err != nil {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err := st.SaveReport(r.Context(), report); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eventID, err := st.AppendEvent(r.Context(), event)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Enqueue for the next sync cycle (architecture 11.2 step 1). A
	// submitted report version is immutable, so re-enqueueing it here
	// would never be needed even if this handler ran twice for the same
	// submit — the lifecycle precondition in report.Submit above already
	// rejects a second submit of the same version.
	if _, err := st.EnqueueReportVersion(r.Context(), report.ReportID, report.VersionNo); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := st.EnqueueReportAuditEvent(r.Context(), report.ReportID, report.VersionNo, eventID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.runCascade(r.Context(), st, report.SchemaName); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	report, err = st.GetLatestVersion(r.Context(), report.ReportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toReportView(report))
}

// handleStartCorrection creates version N+1 of a submitted-or-later report
// as a new draft (architecture 8.1/8.2, design handoff A7's "Start
// correction"). No submit-permission check: architecture 8.1 is explicit
// that "vessel users may self-initiate corrections. No office approval is
// required" — the same as any other draft edit, not the gated action
// Submit is. Everything downstream of this endpoint (editing the new
// draft's fields, re-checking natural-key uniqueness, cascade
// revalidation) reuses the existing save-section/check machinery
// unchanged — a correction is just a report that happens to start at
// version N+1 instead of 1.
//
// Architecture 8.2's "warn and require confirmation if a correction
// changes the timestamp of an already-pushed report" is not implemented
// here: "pushed" is a real pkg/domain state (domain.StatePushed), but the
// Veracity batch-push integration it described was cancelled (DNV
// declined API access) before anything ever set it, and no replacement
// "pushed to an external consumer" concept has been decided — the
// condition can never be true today, and may never need to be.
func (s *Server) handleStartCorrection(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	report, err := st.GetLatestVersion(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	next, event, err := report.NewCorrection(user.Username)
	if err != nil {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err := st.SaveReport(r.Context(), next); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	eventID, err := st.AppendEvent(r.Context(), event)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Architecture 14: correction_started is one of the audit trail's
	// cross-side event types — see handleCreateReport's own comment on
	// this same gap. Attached to the prior version (event.VersionNo,
	// same as report.VersionNo here), matching what Report.NewCorrection
	// itself stamped the event with — the new draft's own version gets
	// its "created" event separately once it's actually saved/edited.
	if _, err := st.EnqueueReportAuditEvent(r.Context(), report.ReportID, report.VersionNo, eventID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toReportView(next))
}

// loadEditableReport loads reportID's latest version and rejects (409)
// if it has already left draft/ready — matching the reports_locked_
// after_submit DB trigger's own scope (vessel/store/migrations), so the
// API surfaces the same rule before ever reaching that trigger.
func (s *Server) loadEditableReport(w http.ResponseWriter, r *http.Request, st *store.Store, reportID string) (*domain.Report, bool) {
	report, err := st.GetLatestVersion(r.Context(), reportID)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return nil, false
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if report.State != domain.StateDraft && report.State != domain.StateReady {
		httpjson.WriteError(w, http.StatusConflict, fmt.Sprintf("report is %s and locked; start a correction to edit it", report.State))
		return nil, false
	}
	return report, true
}

// lockConflict reports whether any field name in changes belongs to a
// schema section currently soft-locked (architecture 9.5) by someone
// other than callerID — called from handleSaveSection as the real
// enforcement behind FormWizard's client-side "can't select a locked
// section" gate. Fails open (no conflict) if the schema can't be
// loaded — evaluateReport's own callers already surface a schema-load
// failure as a hard error elsewhere in this same request path, so this
// check silently agreeing "no conflict" here doesn't hide anything real.
func (s *Server) lockConflict(report *domain.Report, changes map[string]any, callerID string) (sectionLock, bool) {
	sch, err := s.loadSchemaFor(report.SchemaName)
	if err != nil {
		return sectionLock{}, false
	}
	sectionByField := make(map[string]string, len(sch.Fields))
	for _, f := range sch.Fields {
		sectionByField[f.Name] = f.Section
	}
	checked := make(map[string]bool)
	for field := range changes {
		section, ok := sectionByField[field]
		if !ok || checked[section] {
			continue
		}
		checked[section] = true
		if lock, ok := s.locks.holder(report.ReportID, section); ok && lock.UserID != callerID {
			return lock, true
		}
	}
	return sectionLock{}, false
}

// evaluateReport runs field rules, plausibility rules and continuity
// rules for report against its schema and this vessel's applied config
// bundle's field policy/plausibility config (fieldConfigFromBundle/
// validationConfigFromBundle; the built-in stand-in when no bundle is
// synced). Also returns the loaded schema so callers needing it too (e.g.
// regulatory readiness) don't load it a second time.
//
// The continuity pass (validation.EvaluateContinuity against the
// committed chain) is what makes "this report doesn't continue the
// chain" something the officer is told while they can still fix it. It
// used to be cascade's job exclusively, which meant the only way to
// learn about it was the report flipping to invalidated after the fact
// — for a draft, mid-entry, locking the form.
func (s *Server) evaluateReport(ctx context.Context, st *store.Store, report *domain.Report) (validation.Findings, *schema.Schema, validation.FieldPolicy, validation.FieldEvents, error) {
	sch, err := s.loadSchemaFor(report.SchemaName)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	appliedBundle := s.appliedBundle(ctx)
	rawPolicy, events, _ := fieldConfigFromBundle(appliedBundle, report.SchemaName)
	policy := toFieldPolicy(rawPolicy)
	cfg := validationConfigFromBundle(appliedBundle, report.SchemaName)

	// vr carries the report's own EventType, which is what EvaluateFieldRules
	// gates events against — and it is fixed at creation (the Event field is
	// locked in the form, see web/vessel FieldRow's LOCKED_FIELDS), so a
	// report is always health-checked against the same event applicability
	// its officer entered data under.
	vr := report.ToValidation()
	var findings validation.Findings
	findings = append(findings, validation.EvaluateFieldRules(vr, sch, policy, events)...)
	findings = append(findings, validation.EvaluatePlausibilityRules(vr, cfg, nil)...)

	chain, err := st.ListCommittedChain(ctx, report.SchemaName)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	vchain := make([]*validation.Report, len(chain))
	for i, r := range chain {
		vchain[i] = r.ToValidation()
	}
	findings = append(findings, validation.EvaluateContinuity(vr, vchain, cfg)...)
	return findings, sch, policy, events, nil
}

// runCascade re-runs cascade revalidation (architecture 8.3) over
// schemaName's committed report chain and persists an Invalidate
// transition for any report ID it flags — called after any change that
// can affect continuity between reports (a field save, a submit).
//
// Only committed reports take part (domain.State.InChain): drafts are
// neither invalidated nor allowed to invalidate anyone else. A draft
// gets its continuity feedback from the health check instead
// (evaluateReport → validation.EvaluateContinuity), where the officer
// can still act on it.
func (s *Server) runCascade(ctx context.Context, st *store.Store, schemaName string) error {
	chain, err := st.ListCommittedChain(ctx, schemaName)
	if err != nil {
		return err
	}
	vchain := make([]*validation.Report, len(chain))
	for i, r := range chain {
		vchain[i] = r.ToValidation()
	}
	cfg := validationConfigFromBundle(s.appliedBundle(ctx), schemaName)
	result := validation.Revalidate(vchain, cfg)
	now := time.Now().UTC()
	for _, report := range chain {
		rules, invalidated := result.Invalidated[report.ReportID]
		if !invalidated {
			continue
		}
		if report.State == domain.StateInvalidated && stringsEqual(report.InvalidatedRules, rules) {
			continue // already recorded with the same broken rules
		}
		event := report.Invalidate(rules, now)
		if err := st.SaveReport(ctx, report); err != nil {
			return fmt.Errorf("persist invalidation for %s: %w", report.ReportID, err)
		}
		eventID, err := st.AppendEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("append invalidation event for %s: %w", report.ReportID, err)
		}
		// Only re-enqueue if office already knew about this version —
		// InvalidatedFrom is the state captured immediately before this
		// call (Report.Invalidate's own doc comment), preserved across a
		// second invalidation with different rules. draft/ready means
		// this version was never submitted, so office has no row for it
		// to update; anything else (submitted and later) already has a
		// report_versions row on the office side, and previously this
		// state flip never reached it at all — a cascade-invalidated
		// report silently diverged between vessel and office forever.
		if report.InvalidatedFrom != domain.StateDraft && report.InvalidatedFrom != domain.StateReady {
			if _, err := st.EnqueueReportVersion(ctx, report.ReportID, report.VersionNo); err != nil {
				return fmt.Errorf("enqueue invalidated version for %s: %w", report.ReportID, err)
			}
			if _, err := st.EnqueueReportAuditEvent(ctx, report.ReportID, report.VersionNo, eventID); err != nil {
				return fmt.Errorf("enqueue invalidation event for %s: %w", report.ReportID, err)
			}
		}
	}
	return nil
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func toFieldPolicy(raw map[string]string) validation.FieldPolicy {
	policy := make(validation.FieldPolicy, len(raw))
	for name, state := range raw {
		policy[name] = validation.FieldPolicyState(state)
	}
	return policy
}
