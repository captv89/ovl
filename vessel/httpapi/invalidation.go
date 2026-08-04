// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
)

// invalidationNoticeView is the JSON shape for one pkg/domain.
// InvalidationNotice — design handoff A7's notices strip (Phase 5 T4.3).
type invalidationNoticeView struct {
	VersionNo   int       `json:"versionNo"`
	BrokenRules []string  `json:"brokenRules"`
	ComputedAt  time.Time `json:"computedAt"`
}

// handleListInvalidationNotices returns every invalidation notice ever
// applied for a report (all versions), oldest first.
func (s *Server) handleListInvalidationNotices(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	notices, err := st.ListInvalidationNotices(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]invalidationNoticeView, len(notices))
	for i, n := range notices {
		out[i] = invalidationNoticeView{VersionNo: n.VersionNo, BrokenRules: n.BrokenRules, ComputedAt: n.ComputedAt}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
