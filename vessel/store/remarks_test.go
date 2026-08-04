// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_InsertAndListRemarks(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	r1 := domain.Remark{ID: "remark-1", ReportID: "report-1", VersionNo: 1, FieldName: "Cargo_Mt", Body: "double-check this", Author: "reviewer1", CreatedAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)}
	r2 := domain.Remark{ID: "remark-2", ReportID: "report-1", VersionNo: 1, FieldName: "HFO_ROB", Body: "looks off", Author: "reviewer1", CreatedAt: time.Date(2026, 7, 12, 9, 0, 1, 0, time.UTC)}
	if err := s.InsertRemark(ctx, r1); err != nil {
		t.Fatalf("InsertRemark: %v", err)
	}
	if err := s.InsertRemark(ctx, r2); err != nil {
		t.Fatalf("InsertRemark: %v", err)
	}
	// A different report's remark must not show up.
	if err := s.InsertRemark(ctx, domain.Remark{ID: "remark-3", ReportID: "report-2", VersionNo: 1, FieldName: "Cargo_Mt", Body: "x", Author: "reviewer1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("InsertRemark (other report): %v", err)
	}

	got, err := s.ListRemarks(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListRemarks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].FieldName != "Cargo_Mt" || got[1].FieldName != "HFO_ROB" {
		t.Errorf("got = %+v, want Cargo_Mt then HFO_ROB (chronological)", got)
	}
}

func TestStore_InsertRemark_IsIdempotentOnPKConflict(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := domain.Remark{ID: "remark-dup", ReportID: "report-1", VersionNo: 1, FieldName: "Cargo_Mt", Body: "x", Author: "reviewer1", CreatedAt: time.Now().UTC()}
	if err := s.InsertRemark(ctx, r); err != nil {
		t.Fatalf("InsertRemark (first): %v", err)
	}
	if err := s.InsertRemark(ctx, r); err != nil {
		t.Fatalf("InsertRemark (duplicate id): %v", err)
	}
	got, err := s.ListRemarks(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListRemarks: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (no duplicate row)", len(got))
	}
}
