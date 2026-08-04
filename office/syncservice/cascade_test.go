// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
)

// landReport lands a "commercial-period" report at eventTime.
// EventType is "Other event" (not "Departure"/"Arrival") so tests
// exercising a single continuity rule in isolation don't also
// accidentally trip EvaluateEventOrdering — same choice
// vessel/httpapi's own createTestReport test helper makes.
func landReport(t *testing.T, st *store.Store, vesselID, reportID string, eventTime time.Time) *domain.Report {
	t.Helper()
	r := &domain.Report{
		ReportID: reportID, VersionNo: 1, SchemaName: "commercial-period", EventType: "Other event",
		EventTime: eventTime, Fields: map[string]any{"Period_Id": reportID}, State: domain.StateSubmitted,
	}
	if err := st.UpsertReportVersion(context.Background(), vesselID, r, "3.13", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}
	return r
}

func TestRunCascade_TimestampCollisionInvalidatesBoth(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 90)

	sameMinute := time.Date(2026, 7, 12, 12, 0, 30, 0, time.UTC)
	landReport(t, st, v.ID, "cascade-report-1", sameMinute)
	landReport(t, st, v.ID, "cascade-report-2", sameMinute.Add(10*time.Second))

	if err := runCascade(ctx, st, v.ID, "commercial-period"); err != nil {
		t.Fatalf("runCascade: %v", err)
	}

	for _, reportID := range []string{"cascade-report-1", "cascade-report-2"} {
		got, err := st.GetReportVersion(ctx, v.ID, reportID, 1)
		if err != nil {
			t.Fatalf("GetReportVersion(%s): %v", reportID, err)
		}
		if got.State != domain.StateInvalidated {
			t.Errorf("%s.State = %q, want %q", reportID, got.State, domain.StateInvalidated)
		}

		events, err := st.ListReportAuditEvents(ctx, v.ID, reportID)
		if err != nil {
			t.Fatalf("ListReportAuditEvents(%s): %v", reportID, err)
		}
		if len(events) != 1 || events[0].Type != domain.EventInvalidated {
			t.Fatalf("%s events = %+v, want exactly one invalidated event", reportID, events)
		}

		notice, err := st.GetLatestInvalidationNotice(ctx, v.ID, reportID, 1)
		if err != nil {
			t.Fatalf("GetLatestInvalidationNotice(%s): %v", reportID, err)
		}
		if len(notice.BrokenRules) != 1 || notice.BrokenRules[0] != "continuity.timestampUniqueness" {
			t.Errorf("%s notice.BrokenRules = %v, want [continuity.timestampUniqueness]", reportID, notice.BrokenRules)
		}
	}

	// Re-running cascade with nothing changed must not spam duplicate
	// audit events or notices (architecture 8.3's own dedup requirement).
	if err := runCascade(ctx, st, v.ID, "commercial-period"); err != nil {
		t.Fatalf("runCascade (second run): %v", err)
	}
	events, err := st.ListReportAuditEvents(ctx, v.ID, "cascade-report-1")
	if err != nil {
		t.Fatalf("ListReportAuditEvents: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("events after second cascade run = %d, want still 1 (no duplicate)", len(events))
	}
}

func TestRunCascade_NoBrokenRulesIsANoOp(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 91)
	landReport(t, st, v.ID, "cascade-report-3", time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC))

	if err := runCascade(ctx, st, v.ID, "commercial-period"); err != nil {
		t.Fatalf("runCascade: %v", err)
	}
	got, err := st.GetReportVersion(ctx, v.ID, "cascade-report-3", 1)
	if err != nil {
		t.Fatalf("GetReportVersion: %v", err)
	}
	if got.State != domain.StateSubmitted {
		t.Errorf("State = %q, want unchanged %q", got.State, domain.StateSubmitted)
	}
}
