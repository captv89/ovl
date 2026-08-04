// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_InsertAndListInvalidationNotices(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	n := domain.InvalidationNotice{
		ReportID: "report-1", VersionNo: 1, BrokenRules: []string{"continuity.timeChain"},
		ComputedAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC),
	}
	if err := s.InsertInvalidationNotice(ctx, n, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice: %v", err)
	}

	got, err := s.ListInvalidationNotices(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListInvalidationNotices: %v", err)
	}
	if len(got) != 1 || got[0].VersionNo != 1 || len(got[0].BrokenRules) != 1 || got[0].BrokenRules[0] != "continuity.timeChain" {
		t.Errorf("got = %+v, want one notice matching n", got)
	}
	if !got[0].ComputedAt.Equal(n.ComputedAt) {
		t.Errorf("ComputedAt = %v, want %v", got[0].ComputedAt, n.ComputedAt)
	}
}

func TestStore_InsertInvalidationNotice_SkipsDuplicateBrokenRules(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	n := domain.InvalidationNotice{
		ReportID: "report-2", VersionNo: 1, BrokenRules: []string{"continuity.timeChain"},
		ComputedAt: time.Now().UTC(),
	}
	if err := s.InsertInvalidationNotice(ctx, n, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice (first): %v", err)
	}
	// Retransmission after a dropped link: same report/version/broken
	// rules again — must not accumulate a duplicate row.
	if err := s.InsertInvalidationNotice(ctx, n, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice (retransmit): %v", err)
	}
	got, err := s.ListInvalidationNotices(ctx, "report-2")
	if err != nil {
		t.Fatalf("ListInvalidationNotices: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (no duplicate)", len(got))
	}

	// A later notice with genuinely different broken rules is a new row.
	n2 := domain.InvalidationNotice{
		ReportID: "report-2", VersionNo: 1, BrokenRules: []string{"continuity.timeChain", "continuity.robContinuity"},
		ComputedAt: time.Now().UTC(),
	}
	if err := s.InsertInvalidationNotice(ctx, n2, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice (different rules): %v", err)
	}
	got, err = s.ListInvalidationNotices(ctx, "report-2")
	if err != nil {
		t.Fatalf("ListInvalidationNotices: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2 (genuinely new broken rules)", len(got))
	}
}
