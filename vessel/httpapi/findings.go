// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
)

// acknowledgeFindingRequest mirrors the Detail map pkg/domain.Event
// stores for an EventFindingAcknowledged event — see that type's own
// doc comment. Field is optional (a whole-report finding has none).
type acknowledgeFindingRequest struct {
	RuleID       string `json:"ruleId"`
	Field        string `json:"field,omitempty"`
	Message      string `json:"message"`
	Acknowledged bool   `json:"acknowledged"`
}

// handleAcknowledgeFinding records a warning acknowledgement (design
// handoff A6's Acknowledge button) as a real audit-logged, office-synced
// event instead of client-local UI state — the vessel UI rework's Phase
// 3. Reuses the existing AppendEvent/
// EnqueueReportAuditEvent path every other audit event already takes
// (no new sync primitive), enqueued immediately like chat messages
// rather than gated to submit-time like most report events, since an
// acknowledgement is its own standalone fact the moment it happens.
// Draft/ready only (loadEditableReport) — acknowledging a submitted
// report's findings isn't a real scenario since findings are re-run
// fresh at each check.
func (s *Server) handleAcknowledgeFinding(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	var req acknowledgeFindingRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RuleID == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "ruleId is required")
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	report, ok := s.loadEditableReport(w, r, st, r.PathValue("id"))
	if !ok {
		return
	}
	event := domain.Event{
		ReportID:  report.ReportID,
		VersionNo: report.VersionNo,
		Type:      domain.EventFindingAcknowledged,
		At:        time.Now().UTC(),
		Actor:     user.Username,
		Detail: map[string]any{
			"ruleId":       req.RuleID,
			"field":        req.Field,
			"message":      req.Message,
			"acknowledged": req.Acknowledged,
		},
	}
	eventID, err := st.AppendEvent(r.Context(), event)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := st.EnqueueReportAuditEvent(r.Context(), report.ReportID, report.VersionNo, eventID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, map[string]bool{"acknowledged": req.Acknowledged})
}
