// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/schema"
)

// requireReviewer is authenticatedUser's counterpart for remark
// authoring (design handoff B4: "Roles: Reviewer").
func (s *Server) requireReviewer(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return nil, false
	}
	if !user.CanReview() {
		httpjson.WriteError(w, http.StatusForbidden, "only Reviewer may flag fields with remarks")
		return nil, false
	}
	return user, true
}

// remarkView is the JSON shape for one pkg/domain.Remark — design
// handoff B4/A7's Remarks tab.
type remarkView struct {
	ID        string    `json:"id"`
	ReportID  string    `json:"reportId"`
	VersionNo int       `json:"versionNo"`
	FieldName string    `json:"fieldName"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	Resolved  bool      `json:"resolved"`
}

func toRemarkView(r domain.Remark) remarkView {
	return remarkView{
		ID: r.ID, ReportID: r.ReportID, VersionNo: r.VersionNo, FieldName: r.FieldName,
		Body: r.Body, Author: r.Author, CreatedAt: r.CreatedAt, Resolved: r.Resolved,
	}
}

func toRemarkViews(remarks []domain.Remark) []remarkView {
	out := make([]remarkView, len(remarks))
	for i, r := range remarks {
		out[i] = toRemarkView(r)
	}
	return out
}

type remarkFieldInput struct {
	FieldName string `json:"fieldName"`
	Body      string `json:"body"`
}

type createRemarkSetRequest struct {
	Remarks []remarkFieldInput `json:"remarks"`
}

// handleCreateRemarkSet is design handoff B4's "send remark set":
// Reviewer-only, flags one or more fields with comments in a single
// call and transitions the target report to remarked in the same call
// (architecture 12.3) — both the remark rows and the state change/audit
// event land together, so a client never observes one without the
// other.
func (s *Server) handleCreateRemarkSet(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireReviewer(w, r)
	if !ok {
		return
	}
	vesselID, reportID := r.PathValue("vesselId"), r.PathValue("reportId")
	var req createRemarkSetRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Remarks) == 0 {
		httpjson.WriteError(w, http.StatusBadRequest, "at least one remark is required")
		return
	}

	versions, err := s.st.ListReportVersions(r.Context(), vesselID, reportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(versions) == 0 {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	latest := versions[len(versions)-1]

	now := time.Now().UTC()
	setID, err := uuid.NewV7()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	remarks := make([]domain.Remark, len(req.Remarks))
	fieldNames := make([]string, len(req.Remarks))
	for i, f := range req.Remarks {
		id, err := uuid.NewV7()
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		remarks[i] = domain.Remark{
			ID: id.String(), ReportID: reportID, VersionNo: latest.VersionNo,
			FieldName: f.FieldName, Body: f.Body, Author: user.Username, CreatedAt: now,
		}
		fieldNames[i] = f.FieldName
	}
	if err := s.st.InsertRemarkSet(r.Context(), vesselID, remarks, setID.String()); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	report := &domain.Report{ReportID: reportID, VersionNo: latest.VersionNo, State: latest.State}
	event, err := report.MarkRemarked(fieldNames, user.Username, now)
	if err != nil {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.st.UpdateReportVersionState(r.Context(), vesselID, reportID, latest.VersionNo, report.State); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.st.AppendReportAuditEvent(r.Context(), vesselID, event, now, "office"); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Phase 5 vessel-UI-rework restructure: the standalone Remarks tab is
	// dropped client-side, so a remark set now also auto-posts a linking
	// chat message (same InsertChatMessage path handlePostChat uses) —
	// otherwise a remark would land with no trace in the tab that
	// replaced it.
	//
	// 18.07.26 manual-test item 13: this text is persisted verbatim into
	// the chat message body, so the human-friendly field labels have to
	// be resolved here, at write time — unlike the Remarks & findings
	// panel (which resolves labels client-side against a schema it
	// already has), a chat message is immutable once sent. fieldLabels
	// falls back to the raw field name on any lookup failure (missing
	// schema, unrecognized field) rather than failing remark creation
	// over what is ultimately just display text.
	summaryLabels := fieldLabels(r.Context(), s.st, latest.SchemaName, fieldNames)
	chatMsg, err := domain.NewChatMessage(reportID, user.Username, remarkChatSummary(summaryLabels, remarks), domain.ChatFromOffice)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.st.InsertChatMessage(r.Context(), vesselID, chatMsg); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpjson.WriteJSON(w, http.StatusCreated, toRemarkViews(remarks))
}

// remarkChatSummary builds the linking chat message body for a new
// remark set: the flagged fields (first 3, "+N more" beyond that), plus
// the single remark's own body when there's exactly one — enough context
// to be useful in Chat without duplicating every comment when several
// fields are flagged at once. Truncated to MaxChatBodyBytes so this
// always satisfies NewChatMessage's own cap regardless of how long an
// individual remark body is. Takes display labels (see fieldLabels), not
// raw schema field keys — the caller resolves those once up front.
func remarkChatSummary(fieldLabels []string, remarks []domain.Remark) string {
	shown := fieldLabels
	suffix := ""
	if len(fieldLabels) > 3 {
		shown = fieldLabels[:3]
		suffix = fmt.Sprintf(" (+%d more)", len(fieldLabels)-3)
	}
	msg := fmt.Sprintf("Flagged: %s%s", strings.Join(shown, ", "), suffix)
	if len(remarks) == 1 {
		msg += "\n" + remarks[0].Body
	}
	return truncateBytes(msg, domain.MaxChatBodyBytes)
}

// fieldLabels resolves each of names to its human-friendly schema label
// (schema.Field.Label — e.g. "Main Engine HFO consumption" instead of
// "ME_Consumption_HFO"), for schemaName's latest published version.
// 18.07.26 manual-test item 13: remarkChatSummary used to join raw
// backend field keys directly, which read as internal jargon in Chat —
// the Remarks & findings panel elsewhere already resolves labels the
// same way (client-side, against a schema it separately fetched), this
// is the equivalent for the one place that has to do it server-side.
// Falls back to the raw name for any field the schema load can't resolve
// (schema missing, unrecognized field name) — this is display text for a
// chat message, not worth failing remark creation over.
func fieldLabels(ctx context.Context, st *store.Store, schemaName string, names []string) []string {
	out := make([]string, len(names))
	copy(out, names)

	sv, err := st.LatestSchemaVersion(ctx, schemaName)
	if err != nil {
		return out
	}
	sch, err := schema.Parse(sv.Content)
	if err != nil {
		return out
	}
	byName := make(map[string]string, len(sch.Fields))
	for _, f := range sch.Fields {
		byName[f.Name] = f.Label
	}
	for i, name := range names {
		if label, ok := byName[name]; ok && label != "" {
			out[i] = label
		}
	}
	return out
}

// truncateBytes trims s to at most maxBytes bytes without splitting a
// multi-byte UTF-8 rune.
func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	b := s[:maxBytes]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRuneInString(b)
		if r != utf8.RuneError || size != 1 {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}

// handleListRemarks returns every remark ever sent for one report.
// Viewable by any authenticated office user.
func (s *Server) handleListRemarks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	remarks, err := s.st.ListRemarks(r.Context(), r.PathValue("vesselId"), r.PathValue("reportId"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toRemarkViews(remarks))
}

type setRemarkResolvedRequest struct {
	Resolved bool `json:"resolved"`
}

// handleSetRemarkResolved toggles one remark's resolved flag —
// Reviewer-only manual toggle (Phase 5 open question 2's resolved
// default: no auto-infer from a later synced value).
func (s *Server) handleSetRemarkResolved(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireReviewer(w, r); !ok {
		return
	}
	var req setRemarkResolvedRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.st.SetRemarkResolved(r.Context(), r.PathValue("id"), req.Resolved); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"resolved": req.Resolved})
}
