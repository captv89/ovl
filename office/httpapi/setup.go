// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/auth"
)

// setupStatusResponse is what the login screen checks on load to decide
// whether to show "create the first Admin account" or the normal login
// form.
type setupStatusResponse struct {
	HasAnyUser bool `json:"hasAnyUser"`
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	hasAnyUser, err := s.st.HasAnyUser(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, setupStatusResponse{HasAnyUser: hasAnyUser})
}

type setupAdminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetupAdmin creates office's first local account, an Admin — there
// is no OIDC-provisioned or enrollment-issued user yet (architecture
// 12.2's OIDC integration is still blocked on a DNV-issued client, see
// PROJECT.md's Phase 3 checklist), so like vessel/httpapi's
// handleSetupMaster, the very first account has to be created by
// whoever stands up the office instance, choosing their own password
// directly. Rejected once any user already exists — this is a one-time
// bootstrap step, not a general "create user" endpoint.
func (s *Server) handleSetupAdmin(w http.ResponseWriter, r *http.Request) {
	hasAnyUser, err := s.st.HasAnyUser(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hasAnyUser {
		httpjson.WriteError(w, http.StatusConflict, "a user already exists; use the login screen instead")
		return
	}

	var req setupAdminRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// The bootstrap account is the only user in existence at this point —
	// no one else could grant it additional roles later, so it starts as
	// a full superuser (every role, not just Admin) rather than leaving
	// e.g. remarking (RoleReviewer-gated) or commercial data entry
	// (RoleCommercialEditor-gated) unreachable until the admin remembers
	// to self-assign them via Administration > Users. Deliberately not
	// "Admin only": that left a genuinely unusable first-run experience
	// (confirmed by manual-test review — the office side remark button
	// never renders for a fresh install's own admin account).
	u, err := auth.NewUser(req.Username, req.Password, auth.Roles(auth.AllRoles()))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	u.MustChangePassword = false // chosen directly by the operator, not a temporary handoff password

	if err := s.st.CreateUser(context.Background(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.startSession(w, u.ID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toUserView(u))
}
