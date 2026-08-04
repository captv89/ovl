// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
)

// syncIntervalMinSeconds/MaxSeconds bound architecture 11.2's
// "configurable interval": 30s as a practical floor (near real-time when
// a vessel has good connectivity, per the 2026-07-20 manual-test
// feedback that prompted this) without turning into a busy-poll, and 24h
// as a ceiling — beyond that a vessel isn't really "syncing on an
// interval" anymore, it should just use manual Sync now.
const (
	syncIntervalMinSeconds = 30
	syncIntervalMaxSeconds = 24 * 60 * 60
)

type syncSettingsView struct {
	SyncIntervalSeconds int `json:"syncIntervalSeconds"`
}

// handleGetSyncSettings returns the vessel's configured sync interval.
// Any authenticated user may view it (Settings' Sync section is visible
// to every role); only Master may change it (handleSaveSyncSettings).
func (s *Server) handleGetSyncSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	seconds, err := st.GetSyncIntervalSeconds(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, syncSettingsView{SyncIntervalSeconds: seconds})
}

type saveSyncSettingsRequest struct {
	SyncIntervalSeconds int `json:"syncIntervalSeconds"`
}

// handleSaveSyncSettings updates the configured sync interval. Master-
// only, same gate as backup/restore and the sensor source config — this
// is vessel-wide operational config, not a per-user preference. Takes
// effect on the next scheduler wait, not the currently in-flight one —
// see vessel/main.go's runSyncScheduler, which re-reads this setting
// before every wait rather than owning a fixed time.Ticker period.
func (s *Server) handleSaveSyncSettings(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var req saveSyncSettingsRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SyncIntervalSeconds < syncIntervalMinSeconds || req.SyncIntervalSeconds > syncIntervalMaxSeconds {
		httpjson.WriteError(w, http.StatusBadRequest, "syncIntervalSeconds must be between 30 and 86400")
		return
	}
	if err := st.SetSyncIntervalSeconds(r.Context(), req.SyncIntervalSeconds); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, syncSettingsView{SyncIntervalSeconds: req.SyncIntervalSeconds})
}
