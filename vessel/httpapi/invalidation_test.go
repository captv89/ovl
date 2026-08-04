// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestHandleListInvalidationNotices(t *testing.T) {
	s, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	n := domain.InvalidationNotice{
		ReportID: created.ReportID, VersionNo: 1, BrokenRules: []string{"continuity.timeChain"},
		ComputedAt: time.Now().UTC(),
	}
	if err := s.storeOrNil().InsertInvalidationNotice(t.Context(), n, time.Now().UTC()); err != nil {
		t.Fatalf("InsertInvalidationNotice: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/invalidation-notices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET invalidation-notices: status %d, body %s", rec.Code, rec.Body)
	}
	notices := decodeBody[[]invalidationNoticeView](t, rec)
	if len(notices) != 1 || len(notices[0].BrokenRules) != 1 || notices[0].BrokenRules[0] != "continuity.timeChain" {
		t.Errorf("notices = %+v, want one matching n", notices)
	}
}

func TestHandleListInvalidationNotices_EmptyForCleanReport(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/invalidation-notices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET invalidation-notices: status %d, body %s", rec.Code, rec.Body)
	}
	notices := decodeBody[[]invalidationNoticeView](t, rec)
	if len(notices) != 0 {
		t.Errorf("notices = %+v, want empty", notices)
	}
}
