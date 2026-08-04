// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/attachmentstore"
)

// reportAttachmentView is the JSON shape returned for one attachment —
// architecture 15's "inline preview on vessel and office," mirroring
// vessel/httpapi's own attachmentView.
type reportAttachmentView struct {
	ID          string    `json:"id"`
	ReportID    string    `json:"reportId"`
	VersionNo   int       `json:"versionNo"`
	FieldName   string    `json:"fieldName"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

func toReportAttachmentView(a store.ReportAttachment) reportAttachmentView {
	return reportAttachmentView{
		ID: a.ID, ReportID: a.ReportID, VersionNo: a.VersionNo, FieldName: a.FieldName,
		Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.SizeBytes, ReceivedAt: a.ReceivedAt,
	}
}

// handleListReportAttachments lists one report's attachments for its
// latest version — viewable by any authenticated office user, same
// visibility as chat/remarks.
func (s *Server) handleListReportAttachments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	vesselID, reportID := r.PathValue("vesselId"), r.PathValue("reportId")
	versions, err := s.st.ListReportVersions(r.Context(), vesselID, reportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(versions) == 0 {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	latest := versions[len(versions)-1]
	attachments, err := s.st.ListReportAttachments(r.Context(), vesselID, reportID, latest.VersionNo)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]reportAttachmentView, len(attachments))
	for i, a := range attachments {
		out[i] = toReportAttachmentView(a)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// handleDownloadReportAttachment streams one attachment's bytes for
// inline preview. Confirms the attachment actually belongs to
// {vesselId}/{reportId} before serving — cheap IDOR hardening even
// within office's own data.
func (s *Server) handleDownloadReportAttachment(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	a, err := s.st.GetReportAttachment(r.Context(), r.PathValue("attachmentId"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "attachment not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a.VesselID != r.PathValue("vesselId") || a.ReportID != r.PathValue("reportId") {
		httpjson.WriteError(w, http.StatusNotFound, "attachment not found")
		return
	}

	f, err := s.attachments.Open(a.ContentHash)
	if errors.Is(err, attachmentstore.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "attachment content not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = f.Close() }()
	// Read fully rather than requiring pkg/attachmentstore.Open's
	// io.ReadCloser to also implement io.Seeker (http.ServeContent needs
	// a ReadSeeker for range requests) — matches vessel/httpapi's own
	// handleDownloadAttachment, same size ceiling reasoning.
	data, err := io.ReadAll(f)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q`, a.Filename))
	http.ServeContent(w, r, a.Filename, a.ReceivedAt, bytes.NewReader(data))
}
