// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
)

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// handleHealth reports whether the API and its Postgres connection are
// up. No auth required — it's meant for container/orchestrator health
// checks (architecture 12.1's Docker Compose deployment).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.st.Ping(r.Context()); err != nil {
		httpjson.WriteJSON(w, http.StatusServiceUnavailable, healthResponse{
			Status:   "unavailable",
			Database: err.Error(),
		})
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, healthResponse{Status: "ok", Database: "ok"})
}
