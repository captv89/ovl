// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
)

type markReviewedItem struct {
	VesselID string `json:"vesselId"`
	ReportID string `json:"reportId"`
}

type markReviewedRequest struct {
	Items []markReviewedItem `json:"items"`
}

// handleBulkMarkReviewed is design handoff B3's bulk "mark reviewed"
// action (Phase 5 open question 3's resolved default): Reviewer-only,
// office-only triage bookkeeping that never touches the vessel-visible
// lifecycle.
func (s *Server) handleBulkMarkReviewed(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireReviewer(w, r)
	if !ok {
		return
	}
	var req markReviewedRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Items) == 0 {
		httpjson.WriteError(w, http.StatusBadRequest, "at least one item is required")
		return
	}
	for _, item := range req.Items {
		if err := s.st.MarkReviewed(r.Context(), item.VesselID, item.ReportID, user.Username); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"marked": len(req.Items)})
}
