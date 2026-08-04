// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// uploadAttachment issues a multipart POST to reportID's attachments
// endpoint — testClient.do only knows how to marshal JSON bodies, so
// attachment tests build the multipart request directly.
func uploadAttachment(t *testing.T, c *testClient, reportID, filename, contentType string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/reports/"+reportID+"/attachments", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	for _, ck := range c.jar {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.server.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHandleUploadAttachment(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := uploadAttachment(t, c, created.ReportID, "bdn.jpg", "image/jpeg", []byte("fake-jpeg-bytes"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: status %d, body %s", rec.Code, rec.Body)
	}
	uploaded := decodeBody[attachmentView](t, rec)
	if uploaded.Filename != "bdn.jpg" || uploaded.ContentType != "image/jpeg" || uploaded.SizeBytes != int64(len("fake-jpeg-bytes")) {
		t.Errorf("uploaded = %+v, want matching filename/contentType/size", uploaded)
	}
	if uploaded.FieldName != "Attachments" {
		t.Errorf("FieldName = %q, want default %q", uploaded.FieldName, "Attachments")
	}
	if uploaded.Synced {
		t.Error("Synced = true for a freshly-uploaded attachment, want false (not yet pushed)")
	}

	rec = c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/attachments", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d, body %s", rec.Code, rec.Body)
	}
	listed := decodeBody[[]attachmentView](t, rec)
	if len(listed) != 1 || listed[0].ID != uploaded.ID {
		t.Errorf("listed = %+v, want one attachment matching %q", listed, uploaded.ID)
	}
}

func TestHandleUploadAttachment_RejectsDisallowedContentType(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())

	rec := uploadAttachment(t, c, created.ReportID, "malware.exe", "application/x-msdownload", []byte("nope"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a disallowed content type", rec.Code)
	}
}

func TestHandleUploadAttachment_RejectsOnLockedReport(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/check", nil); rec.Code != http.StatusOK {
		t.Fatalf("check: status %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/reports/"+created.ReportID+"/submit", nil); rec.Code != http.StatusOK {
		t.Fatalf("submit: status %d", rec.Code)
	}

	rec := uploadAttachment(t, c, created.ReportID, "late.jpg", "image/jpeg", []byte("too-late"))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for an upload on a submitted report", rec.Code)
	}
}

func TestHandleDownloadAndDeleteAttachment(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	created := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	uploaded := decodeBody[attachmentView](t, uploadAttachment(t, c, created.ReportID, "bdn.pdf", "application/pdf", []byte("%PDF-1.4 fake")))

	rec := c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/attachments/"+uploaded.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: status %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "fake") {
		t.Errorf("downloaded body = %q, want it to contain the uploaded content", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}

	rec = c.do(http.MethodDelete, "/api/reports/"+created.ReportID+"/attachments/"+uploaded.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: status %d, body %s", rec.Code, rec.Body)
	}
	rec = c.do(http.MethodGet, "/api/reports/"+created.ReportID+"/attachments", nil)
	listed := decodeBody[[]attachmentView](t, rec)
	if len(listed) != 0 {
		t.Errorf("listed after delete = %+v, want empty", listed)
	}
}

func TestHandleDownloadAttachment_CrossReportIsNotFound(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	a := createTestReport(t, c, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	b := createTestReport(t, c, time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC), commercialPeriodFields())
	uploaded := decodeBody[attachmentView](t, uploadAttachment(t, c, a.ReportID, "bdn.jpg", "image/jpeg", []byte("bytes")))

	rec := c.do(http.MethodGet, "/api/reports/"+b.ReportID+"/attachments/"+uploaded.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an attachment fetched under the wrong report", rec.Code)
	}
}
