// SPDX-License-Identifier: AGPL-3.0-only

package syncproto

import (
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestReportVersionRoundTrip(t *testing.T) {
	r := &domain.Report{
		ReportID:    "0192f1c0-0000-7000-8000-000000000001",
		VersionNo:   2,
		SchemaName:  "commercial-period",
		EventType:   "Departure",
		EventTime:   time.Date(2026, 7, 11, 12, 30, 0, 0, time.UTC),
		Fields:      map[string]any{"IMO": float64(9074729), "Charterer": "Acme Shipping"},
		State:       domain.StateSubmitted,
		SubmittedAt: time.Date(2026, 7, 11, 12, 35, 0, 0, time.UTC),
	}

	pv, err := ReportVersionFromDomain(r)
	if err != nil {
		t.Fatalf("ReportVersionFromDomain: %v", err)
	}
	if pv.GetReportId() != r.ReportID {
		t.Errorf("ReportId = %q, want %q", pv.GetReportId(), r.ReportID)
	}
	if pv.GetVersionNo() != int32(r.VersionNo) {
		t.Errorf("VersionNo = %d, want %d", pv.GetVersionNo(), r.VersionNo)
	}

	back, err := ReportVersionToDomain(pv)
	if err != nil {
		t.Fatalf("ReportVersionToDomain: %v", err)
	}
	if back.ReportID != r.ReportID || back.VersionNo != r.VersionNo || back.SchemaName != r.SchemaName ||
		back.EventType != r.EventType || back.State != r.State {
		t.Errorf("round-tripped report = %+v, want matching %+v", back, r)
	}
	if !back.EventTime.Equal(r.EventTime) {
		t.Errorf("EventTime = %v, want %v", back.EventTime, r.EventTime)
	}
	if !back.SubmittedAt.Equal(r.SubmittedAt) {
		t.Errorf("SubmittedAt = %v, want %v", back.SubmittedAt, r.SubmittedAt)
	}
	if back.Fields["Charterer"] != "Acme Shipping" {
		t.Errorf("Fields[Charterer] = %v, want %q", back.Fields["Charterer"], "Acme Shipping")
	}
	if back.Fields["IMO"] != float64(9074729) {
		t.Errorf("Fields[IMO] = %v, want %v", back.Fields["IMO"], float64(9074729))
	}
}

func TestReportVersionFromDomain_UnknownSchema(t *testing.T) {
	r := &domain.Report{SchemaName: "not-a-real-schema", State: domain.StateDraft}
	if _, err := ReportVersionFromDomain(r); err == nil {
		t.Fatal("ReportVersionFromDomain(unknown schema) = nil error, want an error")
	}
}

func TestReportAuditEventRoundTrip(t *testing.T) {
	e := domain.Event{
		ReportID:  "0192f1c0-0000-7000-8000-000000000001",
		VersionNo: 1,
		Type:      domain.EventSubmitted,
		At:        time.Date(2026, 7, 11, 12, 35, 0, 0, time.UTC),
		Actor:     "master",
	}

	pe, err := ReportAuditEventFromDomain(e)
	if err != nil {
		t.Fatalf("ReportAuditEventFromDomain: %v", err)
	}
	back, err := ReportAuditEventToDomain(pe)
	if err != nil {
		t.Fatalf("ReportAuditEventToDomain: %v", err)
	}
	if back.ReportID != e.ReportID || back.VersionNo != e.VersionNo || back.Type != e.Type || back.Actor != e.Actor {
		t.Errorf("round-tripped event = %+v, want matching %+v", back, e)
	}
	if !back.At.Equal(e.At) {
		t.Errorf("At = %v, want %v", back.At, e.At)
	}
}

func TestReportAuditEventRoundTrip_WithDetail(t *testing.T) {
	e := domain.Event{
		ReportID: "r1", VersionNo: 1, Type: domain.EventHealthCheckResult, At: time.Now().UTC(),
		Detail: map[string]any{"errors": float64(0), "warnings": float64(2), "info": float64(1)},
	}
	pe, err := ReportAuditEventFromDomain(e)
	if err != nil {
		t.Fatalf("ReportAuditEventFromDomain: %v", err)
	}
	back, err := ReportAuditEventToDomain(pe)
	if err != nil {
		t.Fatalf("ReportAuditEventToDomain: %v", err)
	}
	if back.Detail["warnings"] != float64(2) {
		t.Errorf("Detail[warnings] = %v, want 2", back.Detail["warnings"])
	}
}

func TestReportAuditEventRoundTrip_FindingAcknowledged(t *testing.T) {
	e := domain.Event{
		ReportID: "r1", VersionNo: 1, Type: domain.EventFindingAcknowledged, At: time.Now().UTC(), Actor: "chief.officer",
		Detail: map[string]any{"ruleId": "field.required", "field": "Wind_Force_Bft", "message": "Wind force is recommended but empty", "acknowledged": true},
	}
	pe, err := ReportAuditEventFromDomain(e)
	if err != nil {
		t.Fatalf("ReportAuditEventFromDomain: %v", err)
	}
	back, err := ReportAuditEventToDomain(pe)
	if err != nil {
		t.Fatalf("ReportAuditEventToDomain: %v", err)
	}
	if back.Type != domain.EventFindingAcknowledged {
		t.Errorf("Type = %v, want %v", back.Type, domain.EventFindingAcknowledged)
	}
	if back.Detail["ruleId"] != "field.required" || back.Detail["field"] != "Wind_Force_Bft" || back.Detail["acknowledged"] != true {
		t.Errorf("Detail = %+v, want ruleId/field/acknowledged preserved", back.Detail)
	}
}

func TestRemarkRoundTrip(t *testing.T) {
	r := domain.Remark{
		ID:        "remark-1",
		ReportID:  "0192f1c0-0000-7000-8000-000000000001",
		VersionNo: 1,
		FieldName: "Cargo_MT",
		Body:      "please double-check this figure against the BDN",
		Author:    "reviewer1",
		CreatedAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC),
		Resolved:  true,
	}

	pr := RemarkFromDomain(r)
	back := RemarkToDomain(pr)
	if back != r {
		t.Errorf("round-tripped remark = %+v, want %+v", back, r)
	}
}

func TestChatMessageRoundTrip(t *testing.T) {
	m := domain.ChatMessage{
		ID:       "chat-1",
		ReportID: "0192f1c0-0000-7000-8000-000000000001",
		Sender:   "master",
		Body:     "corrected version pushed",
		SentAt:   time.Date(2026, 7, 12, 9, 5, 0, 0, time.UTC),
	}

	pm := ChatMessageFromDomain(m)
	back := ChatMessageToDomain(pm, domain.ChatFromVessel)
	want := m
	want.Direction = domain.ChatFromVessel
	if back != want {
		t.Errorf("round-tripped chat message = %+v, want %+v", back, want)
	}
}

func TestInvalidationNoticeRoundTrip(t *testing.T) {
	n := domain.InvalidationNotice{
		ReportID:    "0192f1c0-0000-7000-8000-000000000001",
		VersionNo:   3,
		BrokenRules: []string{"continuity.robContinuity", "continuity.timeChain"},
		ComputedAt:  time.Date(2026, 7, 12, 9, 10, 0, 0, time.UTC),
	}

	pn := InvalidationNoticeFromDomain(n)
	back := InvalidationNoticeToDomain(pn)
	if back.ReportID != n.ReportID || back.VersionNo != n.VersionNo || !back.ComputedAt.Equal(n.ComputedAt) {
		t.Errorf("round-tripped notice = %+v, want matching %+v", back, n)
	}
	if len(back.BrokenRules) != len(n.BrokenRules) {
		t.Fatalf("BrokenRules = %v, want %v", back.BrokenRules, n.BrokenRules)
	}
	for i := range n.BrokenRules {
		if back.BrokenRules[i] != n.BrokenRules[i] {
			t.Errorf("BrokenRules[%d] = %q, want %q", i, back.BrokenRules[i], n.BrokenRules[i])
		}
	}
}

func TestSchemaKindFromName_AllCuratedSchemas(t *testing.T) {
	names := []string{"log-abstract", "bunker-report", "edn-report", "commercial-period", "cargo-nomination"}
	seen := map[string]bool{}
	for _, name := range names {
		kind, err := SchemaKindFromName(name)
		if err != nil {
			t.Errorf("SchemaKindFromName(%q): %v", name, err)
			continue
		}
		back, err := SchemaNameFromKind(kind)
		if err != nil {
			t.Errorf("SchemaNameFromKind(%v): %v", kind, err)
			continue
		}
		if back != name {
			t.Errorf("round trip for %q produced %q", name, back)
		}
		seen[kind.String()] = true
	}
	if len(seen) != len(names) {
		t.Errorf("got %d distinct proto kinds for %d schema names, want all distinct", len(seen), len(names))
	}
}
