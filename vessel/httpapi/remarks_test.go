// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestHandleListRemarks(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	r := domain.Remark{
		ID: "remark-1", ReportID: created.ReportID, VersionNo: 1, FieldName: "Charterer",
		Body: "please double-check this", Author: "reviewer1", CreatedAt: time.Now().UTC(),
	}
	if err := s.storeOrNil().InsertRemark(t.Context(), r); err != nil {
		t.Fatalf("InsertRemark: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/remarks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET remarks: status %d, body %s", rec.Code, rec.Body)
	}
	remarks := decodeBody[[]remarkView](t, rec)
	if len(remarks) != 1 || remarks[0].FieldName != "Charterer" {
		t.Errorf("remarks = %+v, want one Charterer remark", remarks)
	}
}

func TestHandleListRemarks_EmptyForCleanReport(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/remarks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET remarks: status %d, body %s", rec.Code, rec.Body)
	}
	remarks := decodeBody[[]remarkView](t, rec)
	if len(remarks) != 0 {
		t.Errorf("remarks = %+v, want empty", remarks)
	}
}
