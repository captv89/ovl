// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/domain"
)

func TestHandleBulkMarkReviewed(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 75)
	landTestReport(t, s, v.ID, "report-review-1", 1, time.Now().UTC(), domain.StateSubmitted)

	rec := c.do(http.MethodPost, "/api/reports/mark-reviewed", markReviewedRequest{
		Items: []markReviewedItem{{VesselID: v.ID, ReportID: "report-review-1"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST mark-reviewed: status %d, body %s", rec.Code, rec.Body)
	}

	rows := decodeBody[[]reportListItemView](t, c.do(http.MethodGet, "/api/reports?vesselId="+v.ID, nil))
	found := false
	for _, row := range rows {
		if row.ReportID == "report-review-1" {
			found = true
			if !row.Reviewed {
				t.Errorf("report-review-1.Reviewed = false, want true")
			}
		}
	}
	if !found {
		t.Fatal("report-review-1 not found in list")
	}
}

func TestHandleBulkMarkReviewed_RequiresReviewer(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")

	rec := c.do(http.MethodPost, "/api/reports/mark-reviewed", markReviewedRequest{
		Items: []markReviewedItem{{VesselID: "some-vessel", ReportID: "some-report"}},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST mark-reviewed as viewer: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleListReports_HasRemarksFilter(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	reviewer := createTestUser(t, s, auth.Roles{auth.RoleReviewer}, "correct horse battery staple")
	loginAs(t, c, reviewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 76)
	landTestReport(t, s, v.ID, "report-review-2", 1, time.Now().UTC(), domain.StateSubmitted)
	landTestReport(t, s, v.ID, "report-review-3", 1, time.Now().UTC(), domain.StateSubmitted)

	c.do(http.MethodPost, "/api/reports/"+v.ID+"/report-review-2/remarks", createRemarkSetRequest{
		Remarks: []remarkFieldInput{{FieldName: "Cargo_Mt", Body: "check"}},
	})

	rows := decodeBody[[]reportListItemView](t, c.do(http.MethodGet, "/api/reports?vesselId="+v.ID+"&hasRemarks=true", nil))
	if len(rows) != 1 || rows[0].ReportID != "report-review-2" {
		t.Errorf("rows (hasRemarks=true) = %+v, want just report-review-2", rows)
	}
}
