// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/attachmentstore"
	"github.com/captv89/ovl/vessel/store"
)

// maxAttachmentBytes is the server-side hard cap on any one attachment
// (architecture 15's PDF ceiling — images are expected to already be
// well under this after client-side downscaling, but the server enforces
// the same ceiling for both rather than trusting client-side processing
// alone). attachmentUploadSlack covers multipart's own field/boundary
// overhead so a file exactly at the limit doesn't get rejected for
// reasons unrelated to the file itself.
const (
	maxAttachmentBytes    = 5 << 20 // 5 MB
	attachmentUploadSlack = 64 << 10
)

// allowedAttachmentContentType matches architecture 15's stated scope:
// "images... PDFs." Anything else is rejected before it ever reaches
// pkg/attachmentstore.
func allowedAttachmentContentType(ct string) bool {
	return strings.HasPrefix(ct, "image/") || ct == "application/pdf"
}

// attachmentStore opens (creating if needed) this vessel's content-
// addressed attachment store, rooted at the same attachmentsDir
// backup.go's snapshot routine already knows about.
func (s *Server) attachmentStore() (*attachmentstore.Store, error) {
	return attachmentstore.New(attachmentsDir(s.config().DataDir))
}

// attachmentView is the JSON shape returned for one attachment.
type attachmentView struct {
	ID          string    `json:"id"`
	ReportID    string    `json:"reportId"`
	VersionNo   int       `json:"versionNo"`
	FieldName   string    `json:"fieldName"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	UploadedAt  time.Time `json:"uploadedAt"`
	UploadedBy  string    `json:"uploadedBy"`
	Synced      bool      `json:"synced"`
}

func toAttachmentView(a store.Attachment) attachmentView {
	return attachmentView{
		ID: a.ID, ReportID: a.ReportID, VersionNo: a.VersionNo, FieldName: a.FieldName,
		Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.SizeBytes,
		UploadedAt: a.UploadedAt, UploadedBy: a.UploadedBy, Synced: a.SyncedAt != nil,
	}
}

// handleUploadAttachment stores a new Bunker/EDN report attachment
// (design handoff A5·B's Attachments section, architecture 15). Gated to
// draft/ready like every other report mutation — an attachment on a
// locked report would have nowhere consistent to attribute a version to,
// and a correction re-opens the section same as any other field.
func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	reportID := r.PathValue("id")
	report, ok := s.loadEditableReport(w, r, st, reportID)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+attachmentUploadSlack)
	if err := r.ParseMultipartForm(1 << 20); err != nil { // #nosec G120 -- r.Body is already capped by MaxBytesReader above; the 1MiB here is just the in-memory/disk-spill threshold, not an unbounded read
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment exceeds the %d byte limit or is malformed", maxAttachmentBytes))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer func() { _ = file.Close() }()

	contentType := header.Header.Get("Content-Type")
	if !allowedAttachmentContentType(contentType) {
		httpjson.WriteError(w, http.StatusBadRequest, "only images and PDFs are accepted")
		return
	}

	astore, err := s.attachmentStore()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hash, err := astore.Put(file)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	fieldName := r.FormValue("fieldName")
	if fieldName == "" {
		fieldName = "Attachments"
	}
	id, err := uuid.NewV7()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a := store.Attachment{
		ID: id.String(), ReportID: reportID, VersionNo: report.VersionNo, FieldName: fieldName,
		Filename: header.Filename, ContentType: contentType, ContentHash: hash, SizeBytes: header.Size,
		UploadedAt: time.Now().UTC(), UploadedBy: user.Username,
	}
	if err := st.InsertAttachment(r.Context(), a); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toAttachmentView(a))
}

// handleListAttachments lists reportID's attachments for its latest
// version — viewable by any authenticated user, submitted or not (design
// handoff A7's read-only Attachments preview reuses this same list).
func (s *Server) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	reportID := r.PathValue("id")
	report, err := st.GetLatestVersion(r.Context(), reportID)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	attachments, err := st.ListAttachments(r.Context(), reportID, report.VersionNo)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]attachmentView, len(attachments))
	for i, a := range attachments {
		out[i] = toAttachmentView(a)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// handleDownloadAttachment streams one attachment's bytes for inline
// preview (design handoff A5·B/A7: "image lightbox, PDF viewer").
func (s *Server) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	a, ok := s.loadOwnedAttachment(w, r, st)
	if !ok {
		return
	}
	astore, err := s.attachmentStore()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	f, err := astore.Open(a.ContentHash)
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
	// a ReadSeeker for range requests) — attachments are capped at
	// maxAttachmentBytes, so buffering one in memory to serve is cheap at
	// this app's scale.
	data, err := io.ReadAll(f)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q`, a.Filename))
	http.ServeContent(w, r, a.Filename, a.UploadedAt, bytes.NewReader(data))
}

// handleDeleteAttachment removes one attachment — gated to draft/ready
// and to the report's currently-editable version, matching every other
// field's immutability-after-submit rule.
func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	reportID := r.PathValue("id")
	report, ok := s.loadEditableReport(w, r, st, reportID)
	if !ok {
		return
	}
	a, ok := s.loadOwnedAttachment(w, r, st)
	if !ok {
		return
	}
	if a.VersionNo != report.VersionNo {
		httpjson.WriteError(w, http.StatusConflict, "attachment does not belong to the current editable version")
		return
	}
	if err := st.DeleteAttachment(r.Context(), a.ID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// loadOwnedAttachment loads {attachmentId} and confirms it actually
// belongs to {id} (the path's reportID) — cheap IDOR hardening even
// within one vessel's own local data.
func (s *Server) loadOwnedAttachment(w http.ResponseWriter, r *http.Request, st *store.Store) (store.Attachment, bool) {
	a, err := st.GetAttachment(r.Context(), r.PathValue("attachmentId"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "attachment not found")
		return store.Attachment{}, false
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return store.Attachment{}, false
	}
	if a.ReportID != r.PathValue("id") {
		httpjson.WriteError(w, http.StatusNotFound, "attachment not found")
		return store.Attachment{}, false
	}
	return a, true
}
