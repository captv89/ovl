// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"slices"

	"github.com/captv89/ovl/internal/httpjson"
)

// Vessel groups are free-form JSONB tags on vessels.groups (architecture
// 12.4), not a first-class entity — this phase deliberately doesn't
// introduce a redundant vessel_groups table. Rename/delete below are
// layered directly on that existing model: load every vessel currently
// carrying the tag, mutate
// each one's Groups in place via the same Vessel.UpdateProfile every
// other profile edit already uses, and persist one vessel at a time
// (not one cross-table transaction) — an acceptable failure mode for a
// fleet tag rename (a partial failure just means "some vessels renamed,
// retry"), not financial or audit-critical data.

type renameVesselGroupRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type vesselGroupMutationResponse struct {
	VesselsUpdated int `json:"vesselsUpdated"`
}

// handleRenameVesselGroup renames a group tag across every vessel that
// carries it (design handoff B10's group management). Admin-only.
func (s *Server) handleRenameVesselGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var req renameVesselGroupRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.From == "" || req.To == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "from and to are both required")
		return
	}
	list, err := s.st.ListVesselsByGroup(r.Context(), req.From)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, v := range list {
		groups := slices.Clone(v.Groups)
		for i, g := range groups {
			if g == req.From {
				groups[i] = req.To
			}
		}
		if err := v.UpdateProfile(v.Name, v.Type, groups); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.st.UpdateVessel(r.Context(), v); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	httpjson.WriteJSON(w, http.StatusOK, vesselGroupMutationResponse{VesselsUpdated: len(list)})
}

type deleteVesselGroupRequest struct {
	Group string `json:"group"`
}

// handleDeleteVesselGroup removes a group tag from every vessel that
// carries it. Admin-only.
func (s *Server) handleDeleteVesselGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var req deleteVesselGroupRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Group == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "group is required")
		return
	}
	list, err := s.st.ListVesselsByGroup(r.Context(), req.Group)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, v := range list {
		groups := make([]string, 0, len(v.Groups))
		for _, g := range v.Groups {
			if g != req.Group {
				groups = append(groups, g)
			}
		}
		if err := v.UpdateProfile(v.Name, v.Type, groups); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.st.UpdateVessel(r.Context(), v); err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	httpjson.WriteJSON(w, http.StatusOK, vesselGroupMutationResponse{VesselsUpdated: len(list)})
}
