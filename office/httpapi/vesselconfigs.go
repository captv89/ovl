// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/configbundle"
	"github.com/captv89/ovl/office/store"
)

// vesselConfigView is one row of the fleet-wide "Vessel configs" screen
// (Configuration > Profiles & bundles, 2026-07-25 redesign) — the same
// applied-vs-assigned comparison B2's per-vessel Config tab already
// makes (resolveBundleAssignment + its appliedStatus logic), just across
// every vessel at once so an admin can spot outliers without opening
// each vessel individually.
type vesselConfigView struct {
	VesselID   string `json:"vesselId"`
	VesselName string `json:"vesselName"`
	IMO        string `json:"imo"`
	// Status is one of "synced" (running the assigned bundle), "outOfDate"
	// (running a different bundle than the one assigned), "pendingSync"
	// (a bundle is assigned but the vessel hasn't reported applying any
	// bundle yet), or "unassigned" (no bundle assigned to this vessel or
	// any of its groups).
	Status              string     `json:"status"`
	AssignedBundleLabel string     `json:"assignedBundleLabel,omitempty"`
	ActiveSince         *time.Time `json:"activeSince,omitempty"`
}

// handleListVesselConfigs serves the fleet-wide "Vessel configs" tab.
// Resolves every vessel's assigned bundle against one shared
// ListBundleAssignments call rather than N+1 queries — the same
// resolution resolveBundleAssignment already does one vessel at a time,
// batched here since the whole point of this screen is the fleet view.
func (s *Server) handleListVesselConfigs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	list, err := s.st.ListVessels(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	assignments, err := s.st.ListBundleAssignments(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]vesselConfigView, 0, len(list))
	for _, v := range list {
		view := vesselConfigView{VesselID: v.ID, VesselName: v.Name, IMO: v.IMO}

		match := configbundle.Resolve(assignments, v.ID, v.Groups)
		if match == nil {
			view.Status = "unassigned"
			out = append(out, view)
			continue
		}
		bundle, err := s.st.GetConfigBundle(r.Context(), match.BundleID)
		if errors.Is(err, store.ErrNotFound) {
			view.Status = "unassigned"
			out = append(out, view)
			continue
		}
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view.AssignedBundleLabel = bundle.Label
		published := bundle.PublishedAt
		view.ActiveSince = &published

		var appliedID string
		if sync := s.syncStatusFor(r.Context(), v.ID); sync != nil {
			appliedID = sync.AppliedBundleID
		}
		switch appliedID {
		case "":
			view.Status = "pendingSync"
		case bundle.ID:
			view.Status = "synced"
		default:
			view.Status = "outOfDate"
		}
		out = append(out, view)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
