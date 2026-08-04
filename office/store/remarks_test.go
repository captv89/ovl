// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_InsertRemarkSetAndListRemarks(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 96)

	now := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	remarks := []domain.Remark{
		{ID: "remark-1", ReportID: "report-r1", VersionNo: 1, FieldName: "Cargo_Mt", Body: "double-check this", Author: "reviewer1", CreatedAt: now},
		{ID: "remark-2", ReportID: "report-r1", VersionNo: 1, FieldName: "HFO_ROB", Body: "looks off", Author: "reviewer1", CreatedAt: now},
	}
	if err := st.InsertRemarkSet(ctx, v.ID, remarks, "set-1"); err != nil {
		t.Fatalf("InsertRemarkSet: %v", err)
	}

	got, err := st.ListRemarks(ctx, v.ID, "report-r1")
	if err != nil {
		t.Fatalf("ListRemarks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].FieldName != "Cargo_Mt" || got[1].FieldName != "HFO_ROB" {
		t.Errorf("got = %+v, want Cargo_Mt then HFO_ROB", got)
	}
}

func TestStore_SetRemarkResolved(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 97)

	remark := domain.Remark{ID: "remark-3", ReportID: "report-r2", VersionNo: 1, FieldName: "Cargo_Mt", Body: "check", Author: "reviewer1", CreatedAt: time.Now().UTC()}
	if err := st.InsertRemarkSet(ctx, v.ID, []domain.Remark{remark}, "set-2"); err != nil {
		t.Fatalf("InsertRemarkSet: %v", err)
	}

	if err := st.SetRemarkResolved(ctx, "remark-3", true); err != nil {
		t.Fatalf("SetRemarkResolved: %v", err)
	}
	got, err := st.ListRemarks(ctx, v.ID, "report-r2")
	if err != nil {
		t.Fatalf("ListRemarks: %v", err)
	}
	if len(got) != 1 || !got[0].Resolved {
		t.Errorf("got = %+v, want Resolved=true", got)
	}
}

func TestStore_ListRemarksSince_VesselScoped(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 98)
	other := createTestVessel(t, st, 99)

	remark := domain.Remark{ID: "remark-4", ReportID: "report-r3", VersionNo: 1, FieldName: "Cargo_Mt", Body: "check", Author: "reviewer1", CreatedAt: time.Now().UTC()}
	if err := st.InsertRemarkSet(ctx, v.ID, []domain.Remark{remark}, "set-3"); err != nil {
		t.Fatalf("InsertRemarkSet: %v", err)
	}
	otherRemark := domain.Remark{ID: "remark-5", ReportID: "report-r4", VersionNo: 1, FieldName: "Cargo_Mt", Body: "check", Author: "reviewer1", CreatedAt: time.Now().UTC()}
	if err := st.InsertRemarkSet(ctx, other.ID, []domain.Remark{otherRemark}, "set-4"); err != nil {
		t.Fatalf("InsertRemarkSet (other vessel): %v", err)
	}

	items, err := st.ListRemarksSince(ctx, v.ID, 0)
	if err != nil {
		t.Fatalf("ListRemarksSince: %v", err)
	}
	if len(items) != 1 || items[0].Remark.ID != "remark-4" {
		t.Fatalf("items = %+v, want exactly remark-4", items)
	}
	if items[0].Cursor == 0 {
		t.Error("Cursor is zero, want a real seq value")
	}

	again, err := st.ListRemarksSince(ctx, v.ID, items[0].Cursor)
	if err != nil {
		t.Fatalf("ListRemarksSince (again): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("items after advancing past the cursor = %+v, want empty", again)
	}
}
