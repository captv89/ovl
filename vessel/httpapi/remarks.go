// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
)

// remarkView is the JSON shape for one pkg/domain.Remark — design
// handoff A7's Remarks tab.
type remarkView struct {
	ID        string    `json:"id"`
	ReportID  string    `json:"reportId"`
	VersionNo int       `json:"versionNo"`
	FieldName string    `json:"fieldName"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	Resolved  bool      `json:"resolved"`
}

func toRemarkView(r domain.Remark) remarkView {
	return remarkView{
		ID: r.ID, ReportID: r.ReportID, VersionNo: r.VersionNo, FieldName: r.FieldName,
		Body: r.Body, Author: r.Author, CreatedAt: r.CreatedAt, Resolved: r.Resolved,
	}
}

// handleListRemarks returns every remark ever received for a report (all
// versions), chronological.
func (s *Server) handleListRemarks(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	remarks, err := st.ListRemarks(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]remarkView, len(remarks))
	for i, rm := range remarks {
		out[i] = toRemarkView(rm)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
