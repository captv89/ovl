// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"context"
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/authcrypto"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin mirrors vessel/httpapi's handleLogin: the failure response
// is identical whether the username doesn't exist or the password is
// wrong, both in message and in timing (VerifyPassword always runs,
// against authcrypto.DummyHash() when there is no real user), so a
// failed attempt can't be used to enumerate valid usernames.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	u, err := s.st.GetUserByUsername(r.Context(), req.Username)
	hash := authcrypto.DummyHash()
	if err == nil {
		hash = u.PasswordHash
	}
	match, verr := authcrypto.VerifyPassword(req.Password, hash)
	if verr != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, verr.Error())
		return
	}
	if err != nil || !match || !u.Active {
		httpjson.WriteError(w, http.StatusUnauthorized, "incorrect username or password")
		return
	}

	if err := s.startSession(w, u.ID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toUserView(u))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.revoke(cookie.Value)
	}
	http.SetCookie(w, s.sessionCookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toUserView(u))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// handleChangePassword is the general-purpose self-service password
// change, matching vessel/httpapi's handleChangePassword.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	var req changePasswordRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	match, err := u.Authenticate(req.CurrentPassword)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !match {
		httpjson.WriteError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if err := u.ChangePassword(req.NewPassword); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateUser(context.Background(), u); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toUserView(u))
}
