// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/auth"
	"github.com/captv89/ovl/pkg/authcrypto"
)

// handleListUsers serves design handoff B10's Users tab. Admin-only —
// same gate as vessel/enrollment management (architecture 12.2: "Admin
// manages users").
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	list, err := s.st.ListUsers(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]userView, len(list))
	for i, u := range list {
		out[i] = toUserView(u)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type createUserRequest struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// createUserResponse carries the one-time plaintext temporary password
// (never stored, never recoverable afterward) — same "reveal once"
// contract as enrollment's issueResultView, generated rather than
// admin-chosen so provisioning can't produce a weak shared default.
type createUserResponse struct {
	User              userView `json:"user"`
	TemporaryPassword string   `json:"temporaryPassword"`
}

// handleCreateUser provisions a new local account. Admin-only.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var req createUserRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	roles := make(auth.Roles, len(req.Roles))
	for i, roleName := range req.Roles {
		roles[i] = auth.Role(roleName)
	}
	temporaryPassword, err := authcrypto.RandomToken(12) // 96 bits, same as enrollment's own initial Master password
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := auth.NewUser(req.Username, temporaryPassword, roles)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.CreateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, createUserResponse{User: toUserView(u), TemporaryPassword: temporaryPassword})
}

type updateUserRolesRequest struct {
	Roles []string `json:"roles"`
}

// handleUpdateUserRoles reassigns a user's combinable role set. Admin-
// only. A user can change their own roles this way too — office has no
// separate "can't demote yourself" rule in either handoff doc, and
// unlike Deactivate, a role change can't lock the account out entirely
// on its own (there's always at least one other route back to Admin:
// the account can still log in and be re-promoted by any other Admin,
// or by itself if it left itself with configManager access to nothing
// self-service — this mirrors the deliberate lack of such a guard
// elsewhere in this project, e.g. bundle assignment).
func (s *Server) handleUpdateUserRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	u, err := s.st.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	var req updateUserRolesRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	roles := make(auth.Roles, len(req.Roles))
	for i, rl := range req.Roles {
		roles[i] = auth.Role(rl)
	}
	if err := u.SetRoles(roles); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toUserView(u))
}

// handleDeactivateUser blocks a user from logging in (design handoff
// B10). Admin-only. A deactivated Admin can still be reactivated by any
// other Admin — deactivating the last remaining Admin account is not
// guarded against here, same reasoning as handleUpdateUserRoles's own
// doc comment on why no "don't lock yourself out" check exists
// elsewhere in this project.
func (s *Server) handleDeactivateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	u, err := s.st.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	u.Deactivate()
	if err := s.st.UpdateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toUserView(u))
}

// handleReactivateUser reverses handleDeactivateUser. Admin-only.
func (s *Server) handleReactivateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	u, err := s.st.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	u.Reactivate()
	if err := s.st.UpdateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toUserView(u))
}

// handleResetUserPassword is the Admin-initiated counterpart to
// handleChangePassword — fulfills Login.tsx's own existing "Forgot
// password? Ask an Admin to reset it" copy, which had no backing
// endpoint until this phase. Same "reveal once" contract as
// handleCreateUser's own temporary password.
func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	u, err := s.st.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	temporaryPassword, err := authcrypto.RandomToken(12)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := u.ResetPassword(temporaryPassword); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, createUserResponse{User: toUserView(u), TemporaryPassword: temporaryPassword})
}
