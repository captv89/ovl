// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/captv89/ovl/office/vessels"
	"github.com/captv89/ovl/pkg/domain"
)

// createTestVesselWithGroups is createTestVessel with group tags, for
// ListReports' group-filter tests.
func createTestVesselWithGroups(t *testing.T, st *Store, first int, groups []string) *vessels.Vessel {
	t.Helper()
	v := newTestVessel(t, first, groups)
	if err := st.CreateVessel(context.Background(), v); err != nil {
		t.Fatalf("CreateVessel: %v", err)
	}
	t.Cleanup(func() { deleteTestVessel(t, st, v.ID) })
	return v
}

func TestStore_UpsertAndGetReportVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 40)

	r := &domain.Report{
		ReportID: "report-1", VersionNo: 1, SchemaName: "commercial-period", EventType: "Departure",
		EventTime: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Fields:    map[string]any{"Charterer": "Acme Shipping"},
		State:     domain.StateSubmitted, SubmittedAt: time.Date(2026, 7, 11, 12, 5, 0, 0, time.UTC),
	}
	receivedAt := time.Now().UTC()
	if err := st.UpsertReportVersion(ctx, v.ID, r, "3.13", receivedAt); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}

	got, err := st.GetReportVersion(ctx, v.ID, "report-1", 1)
	if err != nil {
		t.Fatalf("GetReportVersion: %v", err)
	}
	if got.ReportID != "report-1" || got.SchemaName != "commercial-period" || got.State != domain.StateSubmitted {
		t.Errorf("got = %+v, want ReportID=report-1 SchemaName=commercial-period State=submitted", got)
	}
	if got.Fields["Charterer"] != "Acme Shipping" {
		t.Errorf("Fields[Charterer] = %v, want Acme Shipping", got.Fields["Charterer"])
	}
	if !got.SubmittedAt.Equal(r.SubmittedAt) {
		t.Errorf("SubmittedAt = %v, want %v", got.SubmittedAt, r.SubmittedAt)
	}
}

func TestStore_GetReportVersion_NotFound(t *testing.T) {
	st := openTestStore(t)
	v := createTestVessel(t, st, 41)
	if _, err := st.GetReportVersion(context.Background(), v.ID, "no-such-report", 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetReportVersion(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_UpsertReportVersion_IsIdempotent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 42)

	r := &domain.Report{
		ReportID: "report-1", VersionNo: 1, SchemaName: "commercial-period", EventType: "Departure",
		EventTime: time.Now().UTC(), Fields: map[string]any{"a": "b"}, State: domain.StateSubmitted,
	}
	if err := st.UpsertReportVersion(ctx, v.ID, r, "3.13", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion (first): %v", err)
	}
	// Retransmission after a dropped link: same content, upserted again.
	if err := st.UpsertReportVersion(ctx, v.ID, r, "3.13", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion (retransmit): %v", err)
	}

	got, err := st.GetReportVersion(ctx, v.ID, "report-1", 1)
	if err != nil {
		t.Fatalf("GetReportVersion: %v", err)
	}
	if got.Fields["a"] != "b" {
		t.Errorf("Fields[a] = %v, want b", got.Fields["a"])
	}
}

func TestStore_AppendReportAuditEvent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 43)

	e := domain.Event{ReportID: "report-1", VersionNo: 1, Type: domain.EventSubmitted, Actor: "master", At: time.Now().UTC()}
	if err := st.AppendReportAuditEvent(ctx, v.ID, e, time.Now().UTC(), "vessel"); err != nil {
		t.Fatalf("AppendReportAuditEvent: %v", err)
	}
	// Appending twice (e.g. two distinct outbox items) just appends
	// twice — idempotency for retransmission is outbox_receipts' job,
	// not this table's.
	if err := st.AppendReportAuditEvent(ctx, v.ID, e, time.Now().UTC(), "vessel"); err != nil {
		t.Fatalf("AppendReportAuditEvent (second): %v", err)
	}
}

func TestStore_ListReports(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	// Every query below is scoped to this test's own fixtures via a unique
	// group both vessels share (scopeGroup), so assertions about counts and
	// ordering hold regardless of whatever else lives in the shared dev
	// Postgres (demo data, other tests' rows) — the vessels self-clean via
	// createTestVesselWithGroups' t.Cleanup, so scopeGroup never accumulates
	// across runs either. Fixes the recurring TestStore_ListReports false
	// alarm (codebase audit 2026-07-22 §8). fleetA/fleetB are distinct
	// per-vessel groups for the group-filter subtest, also made unique so
	// they can't collide with demo data.
	scopeGroup := "listreports-scope"
	fleetA := "listreports-fleet-a"
	fleetB := "listreports-fleet-b"
	v1 := createTestVesselWithGroups(t, st, 50, []string{fleetA, scopeGroup})
	v2 := createTestVesselWithGroups(t, st, 51, []string{fleetB, scopeGroup})

	landReportVersion(t, st, v1.ID, "report-a", 1, "log-abstract", "Departure",
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	landReportVersion(t, st, v1.ID, "report-a", 2, "log-abstract", "Departure",
		time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	landReportVersion(t, st, v2.ID, "report-b", 1, "bunker-report", "Bunkering",
		time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), domain.StateInvalidated)

	t.Run("returns latest version of every report in scope, newest first", func(t *testing.T) {
		rows, err := st.ListReports(ctx, ReportFilter{GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		var a, b *ReportListRow
		for i := range rows {
			switch rows[i].ReportID {
			case "report-a":
				a = &rows[i]
			case "report-b":
				b = &rows[i]
			}
		}
		if a == nil || a.VersionNo != 2 {
			t.Fatalf("report-a = %+v, want the latest version (2)", a)
		}
		if b == nil || b.VersionNo != 1 {
			t.Fatalf("report-b = %+v, want version 1", b)
		}
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want exactly 2 (this test's own reports)", len(rows))
		}
		if rows[0].ReportID != "report-b" {
			t.Errorf("rows[0].ReportID = %q, want report-b (newest event_time first)", rows[0].ReportID)
		}
	})

	t.Run("filters by vessel", func(t *testing.T) {
		rows, err := st.ListReports(ctx, ReportFilter{VesselID: &v1.ID})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		for _, row := range rows {
			if row.VesselID != v1.ID {
				t.Errorf("row.VesselID = %q, want only %q", row.VesselID, v1.ID)
			}
		}
		if len(rows) != 1 {
			t.Errorf("len(rows) = %d, want 1", len(rows))
		}
	})

	t.Run("filters by group", func(t *testing.T) {
		rows, err := st.ListReports(ctx, ReportFilter{GroupID: &fleetB})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-b" {
			t.Errorf("rows = %+v, want just report-b", rows)
		}
	})

	t.Run("filters by state", func(t *testing.T) {
		state := string(domain.StateInvalidated)
		rows, err := st.ListReports(ctx, ReportFilter{State: &state, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-b" {
			t.Errorf("rows = %+v, want just report-b", rows)
		}
	})

	t.Run("InvalidatedOnly is equivalent to filtering state=invalidated", func(t *testing.T) {
		yes := true
		rows, err := st.ListReports(ctx, ReportFilter{InvalidatedOnly: &yes, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-b" {
			t.Errorf("rows = %+v, want just report-b", rows)
		}
	})

	t.Run("filters by event type and schema name", func(t *testing.T) {
		eventType := "Bunkering"
		rows, err := st.ListReports(ctx, ReportFilter{EventType: &eventType, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-b" {
			t.Errorf("rows (by event type) = %+v, want just report-b", rows)
		}

		schemaName := "log-abstract"
		rows, err = st.ListReports(ctx, ReportFilter{SchemaName: &schemaName, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-a" {
			t.Errorf("rows (by schema) = %+v, want just report-a", rows)
		}
	})

	t.Run("filters by has-remarks and rows carry Reviewed", func(t *testing.T) {
		if err := st.InsertRemarkSet(ctx, v1.ID, []domain.Remark{
			{ID: "list-remark-1", ReportID: "report-a", VersionNo: 2, FieldName: "Cargo_Mt", Body: "check", Author: "reviewer1", CreatedAt: time.Now().UTC()},
		}, "list-remark-set-1"); err != nil {
			t.Fatalf("InsertRemarkSet: %v", err)
		}
		hasRemarks := true
		rows, err := st.ListReports(ctx, ReportFilter{HasRemarks: &hasRemarks, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-a" {
			t.Errorf("rows (has-remarks) = %+v, want just report-a", rows)
		}
		if len(rows) == 1 && !rows[0].HasRemarks {
			t.Errorf("report-a.HasRemarks = false, want true (a remark was just inserted)")
		}

		allRows, err := st.ListReports(ctx, ReportFilter{GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		for _, row := range allRows {
			if row.ReportID == "report-b" && row.HasRemarks {
				t.Errorf("report-b.HasRemarks = true, want false (no remark inserted for it)")
			}
		}

		if err := st.MarkReviewed(ctx, v1.ID, "report-a", "reviewer1"); err != nil {
			t.Fatalf("MarkReviewed: %v", err)
		}
		rows, err = st.ListReports(ctx, ReportFilter{VesselID: &v1.ID})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		var found bool
		for _, row := range rows {
			if row.ReportID == "report-a" {
				found = true
				if !row.Reviewed {
					t.Errorf("report-a.Reviewed = false, want true after MarkReviewed")
				}
			}
			if row.ReportID != "report-a" && row.Reviewed {
				t.Errorf("%s.Reviewed = true, want false (never marked)", row.ReportID)
			}
		}
		if !found {
			t.Fatal("report-a not found in v1's rows")
		}
	})

	t.Run("filters by date range", func(t *testing.T) {
		from := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
		rows, err := st.ListReports(ctx, ReportFilter{DateFrom: &from, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-b" {
			t.Errorf("rows (DateFrom) = %+v, want just report-b", rows)
		}

		// DateTo filters on the latest version's event_time (the only
		// version ListReports ever surfaces per report) — report-a's
		// latest is v2 at 07-02, report-b's is 07-03, so a cutoff between
		// them keeps report-a and excludes report-b.
		to := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
		rows, err = st.ListReports(ctx, ReportFilter{DateTo: &to, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports: %v", err)
		}
		if len(rows) != 1 || rows[0].ReportID != "report-a" || rows[0].VersionNo != 2 {
			t.Errorf("rows (DateTo) = %+v, want just report-a v2", rows)
		}
	})

	t.Run("Limit and Offset bound and page the result set", func(t *testing.T) {
		// Added for the external data API (architecture 13.2) — a
		// GraphQL/CSV caller outside office's own UI must never be able
		// to trigger an unbounded full-table scan. Scoped to this test's
		// group so paging is asserted against exactly its own two reports,
		// not whatever else is in the shared dev DB.
		all, err := st.ListReports(ctx, ReportFilter{GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports (unbounded): %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("len(all) = %d, want exactly 2 (this test's own reports) to page", len(all))
		}

		one := 1
		firstPage, err := st.ListReports(ctx, ReportFilter{Limit: &one, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports (Limit=1): %v", err)
		}
		if len(firstPage) != 1 {
			t.Fatalf("len(firstPage) = %d, want 1", len(firstPage))
		}
		if firstPage[0].ReportID != all[0].ReportID || firstPage[0].VersionNo != all[0].VersionNo {
			t.Errorf("firstPage[0] = %+v, want %+v (same order as unbounded)", firstPage[0], all[0])
		}

		oneOffset := 1
		secondPage, err := st.ListReports(ctx, ReportFilter{Limit: &one, Offset: &oneOffset, GroupID: &scopeGroup})
		if err != nil {
			t.Fatalf("ListReports (Limit=1, Offset=1): %v", err)
		}
		if len(secondPage) != 1 {
			t.Fatalf("len(secondPage) = %d, want 1", len(secondPage))
		}
		if secondPage[0].ReportID != all[1].ReportID || secondPage[0].VersionNo != all[1].VersionNo {
			t.Errorf("secondPage[0] = %+v, want %+v (offset by one from the unbounded list)", secondPage[0], all[1])
		}
		if secondPage[0].ReportID == firstPage[0].ReportID && secondPage[0].VersionNo == firstPage[0].VersionNo {
			t.Error("firstPage and secondPage returned the same row, want Offset to actually advance")
		}
	})
}

func TestStore_ListChain(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 54)
	landReportVersion(t, st, v.ID, "report-e", 1, "commercial-period", "Departure", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	landReportVersion(t, st, v.ID, "report-e", 2, "commercial-period", "Departure", time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	landReportVersion(t, st, v.ID, "report-f", 1, "commercial-period", "Arrival", time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), domain.StateSubmitted)
	// Different schema — must not appear in the commercial-period chain.
	landReportVersion(t, st, v.ID, "report-g", 1, "bunker-report", "Bunkering", time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), domain.StateSubmitted)

	chain, err := st.ListChain(ctx, v.ID, "commercial-period")
	if err != nil {
		t.Fatalf("ListChain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("len(chain) = %d, want 2 (latest version of report-e and report-f)", len(chain))
	}
	// Ascending by event_time: report-e's latest (v2, 07-03)... wait,
	// report-f (07-02) sits before report-e's latest version (07-03).
	if chain[0].ReportID != "report-f" || chain[1].ReportID != "report-e" || chain[1].VersionNo != 2 {
		t.Errorf("chain = [%s v%d, %s v%d], want [report-f v1, report-e v2] ascending by event_time",
			chain[0].ReportID, chain[0].VersionNo, chain[1].ReportID, chain[1].VersionNo)
	}
}

func TestStore_UpdateReportVersionState(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 55)
	landReportVersion(t, st, v.ID, "report-h", 1, "log-abstract", "Departure", time.Now().UTC(), domain.StateSubmitted)

	if err := st.UpdateReportVersionState(ctx, v.ID, "report-h", 1, domain.StateInvalidated); err != nil {
		t.Fatalf("UpdateReportVersionState: %v", err)
	}
	got, err := st.GetReportVersion(ctx, v.ID, "report-h", 1)
	if err != nil {
		t.Fatalf("GetReportVersion: %v", err)
	}
	if got.State != domain.StateInvalidated {
		t.Errorf("State = %q, want %q", got.State, domain.StateInvalidated)
	}
	// Fields/schema untouched by a state-only update.
	if got.SchemaName != "log-abstract" {
		t.Errorf("SchemaName = %q, want unchanged log-abstract", got.SchemaName)
	}
}

func TestStore_ListReportVersions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 52)
	landReportVersion(t, st, v.ID, "report-c", 1, "log-abstract", "Departure", time.Now().UTC(), domain.StateSubmitted)
	landReportVersion(t, st, v.ID, "report-c", 2, "log-abstract", "Departure", time.Now().UTC(), domain.StateSynced)

	versions, err := st.ListReportVersions(ctx, v.ID, "report-c")
	if err != nil {
		t.Fatalf("ListReportVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0].VersionNo != 1 || versions[1].VersionNo != 2 {
		t.Errorf("versions = [%d, %d], want [1, 2] (ascending)", versions[0].VersionNo, versions[1].VersionNo)
	}
}

func TestStore_ListReportAuditEvents(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 53)
	e1 := domain.Event{ReportID: "report-d", VersionNo: 1, Type: domain.EventSubmitted, Actor: "master", At: time.Now().UTC()}
	e2 := domain.Event{ReportID: "report-d", VersionNo: 1, Type: domain.EventInvalidated, At: time.Now().UTC().Add(time.Minute)}
	if err := st.AppendReportAuditEvent(ctx, v.ID, e1, time.Now().UTC(), "vessel"); err != nil {
		t.Fatalf("AppendReportAuditEvent: %v", err)
	}
	if err := st.AppendReportAuditEvent(ctx, v.ID, e2, time.Now().UTC(), "office"); err != nil {
		t.Fatalf("AppendReportAuditEvent: %v", err)
	}

	events, err := st.ListReportAuditEvents(ctx, v.ID, "report-d")
	if err != nil {
		t.Fatalf("ListReportAuditEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Type != domain.EventSubmitted || events[1].Type != domain.EventInvalidated {
		t.Errorf("events = [%q, %q], want [submitted, invalidated] (chronological)", events[0].Type, events[1].Type)
	}
	if events[0].Origin != "vessel" || events[1].Origin != "office" {
		t.Errorf("origins = [%q, %q], want [vessel, office]", events[0].Origin, events[1].Origin)
	}
}

// landReportVersion is a small test helper landing one report version
// with the given identity/state, saving each ListReports test case from
// re-deriving a full domain.Report literal.
func landReportVersion(t *testing.T, st *Store, vesselID, reportID string, versionNo int, schemaName, eventType string, eventTime time.Time, state domain.State) {
	t.Helper()
	r := &domain.Report{
		ReportID: reportID, VersionNo: versionNo, SchemaName: schemaName, EventType: eventType,
		EventTime: eventTime, Fields: map[string]any{}, State: state,
	}
	if err := st.UpsertReportVersion(context.Background(), vesselID, r, "3.13", time.Now().UTC()); err != nil {
		t.Fatalf("UpsertReportVersion: %v", err)
	}
}

func TestStore_OutboxReceipts(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 44)

	has, err := st.HasOutboxReceipt(ctx, v.ID, "item-1")
	if err != nil {
		t.Fatalf("HasOutboxReceipt: %v", err)
	}
	if has {
		t.Error("HasOutboxReceipt = true before recording, want false")
	}

	if err := st.RecordOutboxReceipt(ctx, v.ID, "item-1", 1, time.Now().UTC()); err != nil {
		t.Fatalf("RecordOutboxReceipt: %v", err)
	}
	has, err = st.HasOutboxReceipt(ctx, v.ID, "item-1")
	if err != nil {
		t.Fatalf("HasOutboxReceipt: %v", err)
	}
	if !has {
		t.Error("HasOutboxReceipt = false after recording, want true")
	}

	// Recording the same item twice (retransmission) is a no-op, not an error.
	if err := st.RecordOutboxReceipt(ctx, v.ID, "item-1", 1, time.Now().UTC()); err != nil {
		t.Fatalf("RecordOutboxReceipt (duplicate): %v", err)
	}
}
