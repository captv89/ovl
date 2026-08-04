// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func newTestReport(t *testing.T) *domain.Report {
	t.Helper()
	r, _, err := domain.NewReport("log-abstract", "Departure",
		time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC),
		map[string]any{"IMO": 9876543.0, "Distance": 12.0}, "master")
	if err != nil {
		t.Fatalf("domain.NewReport: %v", err)
	}
	return r
}

func TestStore_SaveAndGetReport(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := newTestReport(t)

	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	got, err := s.GetReport(ctx, r.ReportID, r.VersionNo)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if got.ReportID != r.ReportID || got.SchemaName != r.SchemaName || got.EventType != r.EventType {
		t.Errorf("GetReport() = %+v, want identity matching %+v", got, r)
	}
	if !got.EventTime.Equal(r.EventTime) {
		t.Errorf("EventTime = %v, want %v", got.EventTime, r.EventTime)
	}
	if got.Fields["Distance"] != 12.0 {
		t.Errorf("Fields[Distance] = %v, want 12.0", got.Fields["Distance"])
	}
	if got.State != domain.StateDraft {
		t.Errorf("State = %q, want %q", got.State, domain.StateDraft)
	}

	if _, err := s.GetReport(ctx, "does-not-exist", 1); err != ErrNotFound {
		t.Errorf("GetReport(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_SaveReport_UpdatesWhileEditable(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := newTestReport(t)
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (insert): %v", err)
	}

	r.SaveSection("distanceAndSpeed", map[string]any{"Distance": 20.0}, "2/O")
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (update while draft): %v", err)
	}

	got, err := s.GetReport(ctx, r.ReportID, r.VersionNo)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if got.Fields["Distance"] != 20.0 {
		t.Errorf("Fields[Distance] = %v after update, want 20.0", got.Fields["Distance"])
	}
}

func TestStore_SaveReport_LockedAfterSubmit(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := newTestReport(t)
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (insert): %v", err)
	}

	if _, err := r.MarkReady(nil); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := r.Submit("master"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (submit transition): %v", err)
	}

	// Lifecycle-only changes (e.g. cascade invalidation) must still be
	// allowed on a submitted report.
	r.Invalidate([]string{"continuity.robContinuity"}, time.Now())
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (lifecycle-only change after submit): %v", err)
	}

	// But mutating the locked report data directly must be rejected by
	// the reports_locked_after_submit trigger.
	r.Fields["Distance"] = 999.0
	if err := s.SaveReport(ctx, r); err == nil {
		t.Fatal("SaveReport with changed fields on a locked report: got nil error, want a trigger rejection")
	}
}

func TestStore_GetLatestVersion(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := newTestReport(t)
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport v1: %v", err)
	}
	if _, err := r.MarkReady(nil); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := r.Submit("master"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (submitted): %v", err)
	}

	next, _, err := r.NewCorrection("2/O")
	if err != nil {
		t.Fatalf("NewCorrection: %v", err)
	}
	if err := s.SaveReport(ctx, next); err != nil {
		t.Fatalf("SaveReport v2: %v", err)
	}

	got, err := s.GetLatestVersion(ctx, r.ReportID)
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if got.VersionNo != 2 {
		t.Errorf("VersionNo = %d, want 2", got.VersionNo)
	}

	// The original version must still be readable — corrections don't
	// delete history (architecture 8.2).
	v1, err := s.GetReport(ctx, r.ReportID, 1)
	if err != nil {
		t.Fatalf("GetReport v1: %v", err)
	}
	if v1.State != domain.StateSubmitted {
		t.Errorf("v1 State = %q, want unchanged %q", v1.State, domain.StateSubmitted)
	}
}

func TestStore_ListVersions(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := newTestReport(t)
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport v1: %v", err)
	}
	if _, err := r.MarkReady(nil); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := r.Submit("master"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport (submitted): %v", err)
	}
	next, _, err := r.NewCorrection("2/O")
	if err != nil {
		t.Fatalf("NewCorrection: %v", err)
	}
	if err := s.SaveReport(ctx, next); err != nil {
		t.Fatalf("SaveReport v2: %v", err)
	}

	versions, err := s.ListVersions(ctx, r.ReportID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(ListVersions) = %d, want 2", len(versions))
	}
	if versions[0].VersionNo != 1 || versions[1].VersionNo != 2 {
		t.Errorf("VersionNo order = [%d, %d], want [1, 2]", versions[0].VersionNo, versions[1].VersionNo)
	}

	if _, err := s.ListVersions(ctx, "does-not-exist"); err != nil {
		t.Errorf("ListVersions(missing) error = %v, want nil (empty slice)", err)
	}
}

func TestStore_ListChainAndListLatestReports(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	times := []time.Time{
		time.Date(2026, 1, 12, 18, 0, 0, 0, time.UTC), // Arrival, saved first but latest in time
		time.Date(2026, 1, 10, 6, 0, 0, 0, time.UTC),  // Departure
		time.Date(2026, 1, 11, 12, 0, 0, 0, time.UTC), // Noon
	}
	eventTypes := []string{"Arrival", "Departure", "Noon (Position) - Sea passage"}
	for i, et := range eventTypes {
		r, _, err := domain.NewReport("log-abstract", et, times[i], map[string]any{"IMO": 1.0}, "master")
		if err != nil {
			t.Fatalf("NewReport: %v", err)
		}
		if err := s.SaveReport(ctx, r); err != nil {
			t.Fatalf("SaveReport %s: %v", et, err)
		}
	}
	// A different schema must not leak into log-abstract's chain/list.
	other, _, err := domain.NewReport("bunker-report", "", time.Now(), map[string]any{"IMO": 1.0}, "master")
	if err != nil {
		t.Fatalf("NewReport (other schema): %v", err)
	}
	if err := s.SaveReport(ctx, other); err != nil {
		t.Fatalf("SaveReport (other schema): %v", err)
	}

	chain, err := s.ListChain(ctx, "log-abstract")
	if err != nil {
		t.Fatalf("ListChain: %v", err)
	}
	if len(chain) != 3 {
		t.Fatalf("ListChain returned %d reports, want 3", len(chain))
	}
	wantAsc := []string{"Departure", "Noon (Position) - Sea passage", "Arrival"}
	for i, r := range chain {
		if r.EventType != wantAsc[i] {
			t.Errorf("ListChain[%d].EventType = %q, want %q (ascending event time)", i, r.EventType, wantAsc[i])
		}
	}

	latest, err := s.ListLatestReports(ctx, "log-abstract")
	if err != nil {
		t.Fatalf("ListLatestReports: %v", err)
	}
	if len(latest) != 3 || latest[0].EventType != "Arrival" {
		t.Errorf("ListLatestReports = %v, want 3 reports with Arrival (most recent) first", latest)
	}
}

func TestStore_DeleteReport(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := newTestReport(t)
	if err := s.SaveReport(ctx, r); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}
	if _, err := s.AppendEvent(ctx, domain.Event{ReportID: r.ReportID, VersionNo: r.VersionNo, Type: domain.EventCreated, At: time.Now()}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := s.InsertAttachment(ctx, Attachment{
		ID: "att-1", ReportID: r.ReportID, VersionNo: r.VersionNo, FieldName: "Attachments",
		Filename: "f.png", ContentType: "image/png", ContentHash: "hash1", SizeBytes: 10, UploadedAt: time.Now(), UploadedBy: "master",
	}); err != nil {
		t.Fatalf("InsertAttachment: %v", err)
	}
	if err := s.InsertChatMessage(ctx, domain.ChatMessage{ID: "chat-1", ReportID: r.ReportID, Sender: "master", Body: "hi", SentAt: time.Now(), Direction: domain.ChatFromVessel}); err != nil {
		t.Fatalf("InsertChatMessage: %v", err)
	}
	if err := s.InsertInvalidationNotice(ctx, domain.InvalidationNotice{ReportID: r.ReportID, VersionNo: r.VersionNo, BrokenRules: []string{"rob.continuity"}, ComputedAt: time.Now()}, time.Now()); err != nil {
		t.Fatalf("InsertInvalidationNotice: %v", err)
	}

	if err := s.DeleteReport(ctx, r.ReportID, "master"); err != nil {
		t.Fatalf("DeleteReport: %v", err)
	}

	if _, err := s.GetLatestVersion(ctx, r.ReportID); err != ErrNotFound {
		t.Errorf("GetLatestVersion after delete = %v, want ErrNotFound", err)
	}
	events, err := s.ListEvents(ctx, r.ReportID)
	if err != nil {
		t.Fatalf("ListEvents after delete: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ListEvents after delete = %v, want none", events)
	}
	if atts, err := s.ListAttachments(ctx, r.ReportID, r.VersionNo); err != nil || len(atts) != 0 {
		t.Errorf("ListAttachments after delete = %v, %v, want none", atts, err)
	}
	if notices, err := s.ListInvalidationNotices(ctx, r.ReportID); err != nil || len(notices) != 0 {
		t.Errorf("ListInvalidationNotices after delete = %v, %v, want none", notices, err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_messages WHERE report_id = ?`, r.ReportID).Scan(&count); err != nil {
		t.Fatalf("count chat_messages: %v", err)
	}
	if count != 0 {
		t.Errorf("chat_messages remaining after delete = %d, want 0", count)
	}

	var logCount int
	var loggedSchema, loggedState, loggedBy string
	if err := s.db.QueryRowContext(ctx, `SELECT schema_name, state, deleted_by FROM deleted_report_log WHERE report_id = ?`, r.ReportID).Scan(&loggedSchema, &loggedState, &loggedBy); err != nil {
		t.Fatalf("query deleted_report_log: %v", err)
	}
	if loggedSchema != "log-abstract" || loggedState != string(domain.StateDraft) || loggedBy != "master" {
		t.Errorf("deleted_report_log row = schema %q state %q deletedBy %q, want log-abstract/draft/master", loggedSchema, loggedState, loggedBy)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM deleted_report_log WHERE report_id = ?`, r.ReportID).Scan(&logCount); err != nil {
		t.Fatalf("count deleted_report_log: %v", err)
	}
	if logCount != 1 {
		t.Errorf("deleted_report_log rows = %d, want 1", logCount)
	}

	if err := s.DeleteReport(ctx, "does-not-exist", "master"); err != ErrNotFound {
		t.Errorf("DeleteReport(missing) error = %v, want ErrNotFound", err)
	}
}
