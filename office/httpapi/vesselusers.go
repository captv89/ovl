// SPDX-License-Identifier: AGPL-3.0-only

// Package httpapi (this file): architecture 9.3/12.4's remote vessel-
// user administration (2026-07-21) — B2's vessel Users tab. An Admin's
// action here queues a store.UserCommand (office/store/usercommands.go);
// the vessel picks it up on its own next sync cycle (PullInbox's
// user_commands stream) and applies it locally. Office has no other way
// to reach into a vessel (sync is vessel-initiated only), so every
// action here is "ask the vessel to do this next time it calls in," not
// an immediate remote mutation — same shape as the DR-bundle push path
// (office/httpapi/restorebundle.go), and every guardrail vessel/httpapi/
// users.go's own local handlers enforce (no second Master created with
// no ceremony, Master cannot be deactivated) is enforced here too, at
// queue time, not just re-checked vessel-side — an Admin should see a
// clear rejection immediately, not a silently-dropped command.
package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/store"
	"github.com/captv89/ovl/pkg/authcrypto"
	"github.com/captv89/ovl/pkg/syncproto"
)

type vesselUserView struct {
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CanSubmit bool      `json:"canSubmit"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toVesselUserView(u store.VesselUser) vesselUserView {
	return vesselUserView{Username: u.Username, Role: u.Role, Active: u.Active, CanSubmit: u.CanSubmit, UpdatedAt: u.UpdatedAt}
}

// userCommandView is one queued remote action's status (B2's Users tab)
// — TemporaryPassword never appears here, same "shown once, in the
// response that created it, never again" rule as every other one-time
// secret in this project.
type userCommandView struct {
	ID        string     `json:"id"`
	Action    string     `json:"action"`
	Username  string     `json:"username"`
	Role      string     `json:"role,omitempty"`
	IssuedBy  string     `json:"issuedBy"`
	IssuedAt  time.Time  `json:"issuedAt"`
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	AppliedAt *time.Time `json:"appliedAt,omitempty"`
}

func toUserCommandView(c store.UserCommand) userCommandView {
	return userCommandView{
		ID: c.ID, Action: c.Action, Username: c.Username, Role: c.Role,
		IssuedBy: c.IssuedBy, IssuedAt: c.IssuedAt, FetchedAt: c.FetchedAt, AppliedAt: c.AppliedAt,
	}
}

// requireVesselExists is this file's shared 404 check — every handler
// below needs it and nothing more (unlike DR's requireVesselWithDRKey,
// no encryption key precondition applies to user commands).
func (s *Server) requireVesselExists(w http.ResponseWriter, r *http.Request, vesselID string) bool {
	if _, err := s.st.GetVessel(r.Context(), vesselID); err != nil {
		httpjson.WriteError(w, http.StatusNotFound, "vessel not found")
		return false
	}
	return true
}

// findVesselUser looks up username in vesselID's roster mirror — used
// by the guardrail checks below (e.g. "is this the Master") — a
// best-effort, possibly-slightly-stale read (the mirror only updates on
// the vessel's own sync check-ins), which is exactly why the vessel
// re-checks every guardrail itself at apply time too.
func (s *Server) findVesselUser(r *http.Request, vesselID, username string) (store.VesselUser, bool) {
	roster, err := s.st.ListVesselUsers(r.Context(), vesselID)
	if err != nil {
		return store.VesselUser{}, false
	}
	for _, u := range roster {
		if u.Username == username {
			return u, true
		}
	}
	return store.VesselUser{}, false
}

type createVesselUserRequest struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type queuedUserCommandResponse struct {
	Command           userCommandView `json:"command"`
	TemporaryPassword string          `json:"temporaryPassword,omitempty"`
}

// handleCreateVesselUser queues a new crew account (design handoff B2's
// vessel Users tab "add user"). Admin-only. Mirrors vessel/httpapi's own
// handleCreateUser's "the Master account is created during first-run
// setup" rule — a second Master must never be created with no ceremony,
// remotely least of all.
func (s *Server) handleCreateVesselUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	vesselID := r.PathValue("id")
	if !s.requireVesselExists(w, r, vesselID) {
		return
	}
	var req createVesselUserRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Role == syncproto.VesselRoleMaster {
		httpjson.WriteError(w, http.StatusBadRequest, "the Master account is created during the vessel's own first-run setup, not remotely")
		return
	}
	if !syncproto.IsValidVesselRole(req.Role) {
		httpjson.WriteError(w, http.StatusBadRequest, "unknown role")
		return
	}
	if _, exists := s.findVesselUser(r, vesselID, username); exists {
		httpjson.WriteError(w, http.StatusConflict, "that username already exists on this vessel as of its last sync")
		return
	}

	temporaryPassword, err := authcrypto.RandomToken(12)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cmd, err := s.queueUserCommand(r, vesselID, admin.Username, syncproto.UserCommandActionCreate, username, req.Role, temporaryPassword, false, false)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, queuedUserCommandResponse{Command: toUserCommandView(*cmd), TemporaryPassword: temporaryPassword})
}

// handleResetVesselUserPassword queues a password reset for an existing
// crew account (design handoff B2). Admin-only. This is also the Master-
// forgot-their-password recovery path — deliberately no exclusion for
// role=master here, unlike deactivation below: resetting the Master's
// own password is exactly the recovery this feature exists for.
func (s *Server) handleResetVesselUserPassword(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	vesselID := r.PathValue("id")
	if !s.requireVesselExists(w, r, vesselID) {
		return
	}
	username := r.PathValue("username")

	temporaryPassword, err := authcrypto.RandomToken(12)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cmd, err := s.queueUserCommand(r, vesselID, admin.Username, syncproto.UserCommandActionResetPassword, username, "", temporaryPassword, false, false)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, queuedUserCommandResponse{Command: toUserCommandView(*cmd), TemporaryPassword: temporaryPassword})
}

type setVesselUserRoleRequest struct {
	Role string `json:"role"`
}

// handleSetVesselUserRole queues a role change for an existing crew
// account. Admin-only. Promoting anyone to Master is refused here — same
// "no second Master with no ceremony" reasoning as create, applied to a
// role change trying to reach the same destination by a side door.
func (s *Server) handleSetVesselUserRole(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	vesselID := r.PathValue("id")
	if !s.requireVesselExists(w, r, vesselID) {
		return
	}
	username := r.PathValue("username")
	var req setVesselUserRoleRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role == syncproto.VesselRoleMaster {
		httpjson.WriteError(w, http.StatusBadRequest, "promoting a crew member to Master must be done locally on the vessel, not remotely")
		return
	}
	if !syncproto.IsValidVesselRole(req.Role) {
		httpjson.WriteError(w, http.StatusBadRequest, "unknown role")
		return
	}
	cmd, err := s.queueUserCommand(r, vesselID, admin.Username, syncproto.UserCommandActionSetRole, username, req.Role, "", false, false)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, queuedUserCommandResponse{Command: toUserCommandView(*cmd)})
}

type setVesselUserCanSubmitRequest struct {
	CanSubmit bool `json:"canSubmit"`
}

// handleSetVesselUserCanSubmit queues a canSubmit toggle. Admin-only.
func (s *Server) handleSetVesselUserCanSubmit(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	vesselID := r.PathValue("id")
	if !s.requireVesselExists(w, r, vesselID) {
		return
	}
	username := r.PathValue("username")
	var req setVesselUserCanSubmitRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cmd, err := s.queueUserCommand(r, vesselID, admin.Username, syncproto.UserCommandActionSetCanSubmit, username, "", "", req.CanSubmit, false)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, queuedUserCommandResponse{Command: toUserCommandView(*cmd)})
}

// handleSetVesselUserActive queues an activate/deactivate action.
// Admin-only. Deactivating the Master remotely is refused — same rule
// vessel/httpapi's own handleUpdateUser already enforces locally
// ("the Master account cannot be deactivated"), so office resetting the
// Master's password (the recovery path) can never be paired with also
// locking the vessel out entirely.
func (s *Server) handleSetVesselUserActive(w http.ResponseWriter, r *http.Request, active bool) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	vesselID := r.PathValue("id")
	if !s.requireVesselExists(w, r, vesselID) {
		return
	}
	username := r.PathValue("username")
	if !active {
		if u, exists := s.findVesselUser(r, vesselID, username); exists && u.Role == syncproto.VesselRoleMaster {
			httpjson.WriteError(w, http.StatusBadRequest, "the Master account cannot be deactivated")
			return
		}
	}
	cmd, err := s.queueUserCommand(r, vesselID, admin.Username, syncproto.UserCommandActionSetActive, username, "", "", false, active)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, queuedUserCommandResponse{Command: toUserCommandView(*cmd)})
}

func (s *Server) handleDeactivateVesselUser(w http.ResponseWriter, r *http.Request) {
	s.handleSetVesselUserActive(w, r, false)
}

func (s *Server) handleReactivateVesselUser(w http.ResponseWriter, r *http.Request) {
	s.handleSetVesselUserActive(w, r, true)
}

// queueUserCommand mints a command id and persists the queued command —
// shared tail end of every handler above.
func (s *Server) queueUserCommand(r *http.Request, vesselID, issuedBy, action, username, role, temporaryPassword string, canSubmit, active bool) (*store.UserCommand, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	cmd := &store.UserCommand{
		ID: id.String(), VesselID: vesselID, Action: action, Username: username, Role: role,
		TemporaryPassword: temporaryPassword, CanSubmit: canSubmit, Active: active,
		IssuedBy: issuedBy, IssuedAt: time.Now().UTC(),
	}
	if err := s.st.QueueUserCommand(r.Context(), cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}
