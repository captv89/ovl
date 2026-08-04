// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/vessel/auth"
	"github.com/captv89/ovl/vessel/store"
)

// adminUserView is userView plus the fields only a Master's user
// management screen needs (design handoff A9) — never the password
// hash.
type adminUserView struct {
	userView
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toAdminUserView(u *auth.User) adminUserView {
	return adminUserView{
		userView:  toUserView(u),
		Active:    u.Active,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// handleListUsers is design handoff A9's "user list with role, canSubmit
// flag" — Master only.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	users, err := st.ListUsers(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]adminUserView, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUserView(u))
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type createUserRequest struct {
	Username string    `json:"username"`
	Role     auth.Role `json:"role"`
}

type createUserResponse struct {
	User              adminUserView `json:"user"`
	TemporaryPassword string        `json:"temporaryPassword"`
}

// errMasterCreationForbidden and errUsernameTaken are createLocalUser's
// sentinel errors — both handleCreateUser (Master, local, HTTP status
// codes) and applyCreateUserCommand (office, remote, sync.go's "log and
// move on" handling) need to distinguish these from a generic internal
// error, just for different reasons.
var (
	errMasterCreationForbidden = errors.New("the Master account is created during first-run setup, not created afterward")
	errUsernameTaken           = errors.New("that username is already taken")
)

// createLocalUser is the shared core of handleCreateUser (Master, local)
// and applyCreateUserCommand (office, remote — vessel/httpapi/
// usercommands.go, architecture 9.3/12.4's remote user administration)
// — the same "no second Master with no ceremony" rule applies
// identically to both, enforced in exactly one place rather than two
// copies that could drift.
func createLocalUser(ctx context.Context, st *store.Store, username string, role auth.Role, temporaryPassword string) (*auth.User, error) {
	if role == auth.RoleMaster {
		return nil, errMasterCreationForbidden
	}
	if !role.Valid() {
		return nil, fmt.Errorf("auth: %q is not a valid role", role)
	}
	if _, err := st.GetUserByUsername(ctx, username); err == nil {
		return nil, errUsernameTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	u, err := auth.NewUser(username, temporaryPassword, role)
	if err != nil {
		return nil, err
	}
	if err := st.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// handleCreateUser is design handoff A9's "add user (from the default
// role set)": the vessel, not the Master, chooses the temporary
// password (architecture 9.2's "no fleet-wide default passwords ever")
// and returns it exactly once in this response — the UI must show it to
// the Master now, since there is no way to retrieve it again.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var req createUserRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	temporaryPassword, err := auth.GenerateTemporaryPassword()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := createLocalUser(r.Context(), st, req.Username, req.Role, temporaryPassword)
	switch {
	case errors.Is(err, errMasterCreationForbidden):
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, errUsernameTaken):
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, createUserResponse{User: toAdminUserView(u), TemporaryPassword: temporaryPassword})
}

type resetPasswordResponse struct {
	TemporaryPassword string `json:"temporaryPassword"`
}

// handleResetUserPassword is design handoff A9's "reset password
// (generates a temporary one shown once)".
func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	u, err := st.GetUser(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	temporaryPassword, err := auth.GenerateTemporaryPassword()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := u.ResetPassword(temporaryPassword); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := st.UpdateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, resetPasswordResponse{TemporaryPassword: temporaryPassword})
}

type updateUserRequest struct {
	CanSubmit *bool `json:"canSubmit"`
	Active    *bool `json:"active"`
}

// handleUpdateUser is design handoff A9's "toggle canSubmit" and
// "deactivate" actions, folded into one partial-update endpoint since
// both operate on the same row and neither needs its own HTTP verb.
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	caller, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	st, ok := s.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	u, err := st.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req updateUserRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Active != nil && !*req.Active {
		if u.ID == caller.ID {
			httpjson.WriteError(w, http.StatusBadRequest, "you cannot deactivate your own account")
			return
		}
		if u.IsSuperAdmin() {
			httpjson.WriteError(w, http.StatusBadRequest, "the Master account cannot be deactivated")
			return
		}
	}
	if req.CanSubmit != nil {
		u.SetCanSubmit(*req.CanSubmit)
	}
	if req.Active != nil {
		u.SetActive(*req.Active)
	}
	if err := st.UpdateUser(r.Context(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toAdminUserView(u))
}
