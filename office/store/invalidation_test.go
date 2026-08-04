// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStore_InsertAndGetLatestInvalidationNotice(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 70)

	if _, err := st.GetLatestInvalidationNotice(ctx, v.ID, "report-n", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLatestInvalidationNotice before any insert: err = %v, want ErrNotFound", err)
	}

	computedAt := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	if err := st.InsertInvalidationNotice(ctx, v.ID, "report-n", 1, []string{"continuity.timeChain"}, computedAt); err != nil {
		t.Fatalf("InsertInvalidationNotice: %v", err)
	}

	notice, err := st.GetLatestInvalidationNotice(ctx, v.ID, "report-n", 1)
	if err != nil {
		t.Fatalf("GetLatestInvalidationNotice: %v", err)
	}
	if notice.ReportID != "report-n" || notice.VersionNo != 1 {
		t.Errorf("notice = %+v, want ReportID=report-n VersionNo=1", notice)
	}
	if len(notice.BrokenRules) != 1 || notice.BrokenRules[0] != "continuity.timeChain" {
		t.Errorf("notice.BrokenRules = %v, want [continuity.timeChain]", notice.BrokenRules)
	}
	if !notice.ComputedAt.Equal(computedAt) {
		t.Errorf("notice.ComputedAt = %v, want %v", notice.ComputedAt, computedAt)
	}

	// A second insert with different broken rules is a fresh row — get
	// latest now returns the newer one, not the first.
	later := computedAt.Add(time.Hour)
	if err := st.InsertInvalidationNotice(ctx, v.ID, "report-n", 1, []string{"continuity.timeChain", "continuity.robContinuity"}, later); err != nil {
		t.Fatalf("InsertInvalidationNotice (second): %v", err)
	}
	notice, err = st.GetLatestInvalidationNotice(ctx, v.ID, "report-n", 1)
	if err != nil {
		t.Fatalf("GetLatestInvalidationNotice (after second insert): %v", err)
	}
	if len(notice.BrokenRules) != 2 {
		t.Errorf("notice.BrokenRules = %v, want the latest (2 rules)", notice.BrokenRules)
	}
}

func TestStore_ListInvalidationNoticesSince(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 71)
	other := createTestVessel(t, st, 72)

	if err := st.InsertInvalidationNotice(ctx, v.ID, "report-x", 1, []string{"continuity.timeChain"}, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice: %v", err)
	}
	// A different vessel's notice must not leak into v's stream.
	if err := st.InsertInvalidationNotice(ctx, other.ID, "report-y", 1, []string{"continuity.timeChain"}, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice (other vessel): %v", err)
	}

	items, err := st.ListInvalidationNoticesSince(ctx, v.ID, 0)
	if err != nil {
		t.Fatalf("ListInvalidationNoticesSince: %v", err)
	}
	if len(items) != 1 || items[0].Notice.ReportID != "report-x" {
		t.Fatalf("items = %+v, want exactly report-x's notice", items)
	}
	if items[0].Cursor == 0 {
		t.Error("Cursor is zero, want a real seq value")
	}

	// Nothing new since the returned cursor.
	again, err := st.ListInvalidationNoticesSince(ctx, v.ID, items[0].Cursor)
	if err != nil {
		t.Fatalf("ListInvalidationNoticesSince (again): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("items after advancing past the cursor = %+v, want empty", again)
	}
}
