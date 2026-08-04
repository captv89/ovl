// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/domain"
)

func TestHandleListAndDownloadReportAttachment(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 90)
	landTestReport(t, s, v.ID, "report-attach-1", 1, time.Now().UTC(), domain.StateSubmitted)

	hash, err := s.attachments.Put(strings.NewReader("bunker delivery note bytes"))
	if err != nil {
		t.Fatalf("attachments.Put: %v", err)
	}
	if err := s.st.UpsertReportAttachment(context.Background(), store.ReportAttachment{
		ID: "attach-1", VesselID: v.ID, ReportID: "report-attach-1", VersionNo: 1,
		FieldName: "Attachments", Filename: "bdn.pdf", ContentType: "application/pdf",
		ContentHash: hash, SizeBytes: int64(len("bunker delivery note bytes")), ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertReportAttachment: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-attach-1/attachments", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d, body %s", rec.Code, rec.Body)
	}
	listed := decodeBody[[]reportAttachmentView](t, rec)
	if len(listed) != 1 || listed[0].Filename != "bdn.pdf" {
		t.Fatalf("listed = %+v, want one bdn.pdf attachment", listed)
	}

	rec = c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-attach-1/attachments/"+listed[0].ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: status %d, body %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != "bunker delivery note bytes" {
		t.Errorf("downloaded body = %q, want the original content", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
}

func TestHandleDownloadReportAttachment_CrossReportIsNotFound(t *testing.T) {
	s := newTestServer(t)
	c := newTestClient(t, s)
	viewer := createTestUser(t, s, auth.Roles{auth.RoleViewer}, "correct horse battery staple")
	loginAs(t, c, viewer, "correct horse battery staple")
	v := createTestVesselForReports(t, s, 91)
	landTestReport(t, s, v.ID, "report-attach-2", 1, time.Now().UTC(), domain.StateSubmitted)
	landTestReport(t, s, v.ID, "report-attach-3", 1, time.Now().UTC(), domain.StateSubmitted)

	hash, err := s.attachments.Put(strings.NewReader("bytes"))
	if err != nil {
		t.Fatalf("attachments.Put: %v", err)
	}
	if err := s.st.UpsertReportAttachment(context.Background(), store.ReportAttachment{
		ID: "attach-2", VesselID: v.ID, ReportID: "report-attach-2", VersionNo: 1,
		FieldName: "Attachments", Filename: "bdn.jpg", ContentType: "image/jpeg",
		ContentHash: hash, SizeBytes: 5, ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertReportAttachment: %v", err)
	}

	rec := c.do(http.MethodGet, "/api/reports/"+v.ID+"/report-attach-3/attachments/attach-2", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an attachment fetched under the wrong report", rec.Code)
	}
}
