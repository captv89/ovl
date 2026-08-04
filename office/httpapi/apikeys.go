// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/office/apikey"
	"github.com/captv89/ovl/office/store"
)

// apiKeyView is the JSON shape for one API key. The plaintext token
// itself is never part of this view — see createAPIKeyResponse's own
// doc comment for the one place it's ever returned.
type apiKeyView struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	GroupID    *string    `json:"groupId"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func toAPIKeyView(k *apikey.APIKey) apiKeyView {
	return apiKeyView{
		ID:         k.ID,
		Label:      k.Label,
		GroupID:    k.GroupID,
		CreatedBy:  k.CreatedBy,
		CreatedAt:  k.CreatedAt,
		RevokedAt:  k.RevokedAt,
		LastUsedAt: k.LastUsedAt,
	}
}

// handleListAPIKeys serves Administration > API Access. Admin-only, same
// gate as user management — issuing a data-API credential is exactly as
// privileged an act as provisioning a local account.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	list, err := s.st.ListAPIKeys(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]apiKeyView, len(list))
	for i, k := range list {
		out[i] = toAPIKeyView(k)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type createAPIKeyRequest struct {
	Label   string  `json:"label"`
	GroupID *string `json:"groupId"`
}

// createAPIKeyResponse carries the one-time plaintext key (never stored,
// never recoverable afterward) — same "reveal once" contract as
// createUserResponse's own temporary password.
type createAPIKeyResponse struct {
	APIKey apiKeyView `json:"apiKey"`
	Token  string     `json:"token"`
}

// handleCreateAPIKey issues a new API key. Admin-only.
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	var req createAPIKeyRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := apikey.Mint(req.Label, user.Username, req.GroupID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.CreateAPIKey(r.Context(), result.APIKey); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.RecordAPIKeyEvent(r.Context(), result.APIKey.ID, "created", result.APIKey.CreatedAt); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, createAPIKeyResponse{APIKey: toAPIKeyView(result.APIKey), Token: result.Token})
}

// handleRevokeAPIKey revokes an API key immediately. Admin-only.
// Idempotent — revoking an already-revoked (or nonexistent) key is not
// an error, matching RevokeAPIKey's own no-op-on-no-match shape. A
// "revoked" event is still recorded even on that no-op path — Postgres
// doesn't tell us whether the UPDATE actually flipped a row, and a
// second recorded revoke attempt is harmless log noise, not a
// correctness problem.
func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	now := time.Now().UTC()
	if err := s.st.RevokeAPIKey(r.Context(), r.PathValue("id"), now); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.st.RecordAPIKeyEvent(r.Context(), r.PathValue("id"), "revoked", now); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// handleDeleteAPIKey permanently removes an API key and its activity
// log. Admin-only, and only once the key is already revoked (409
// otherwise) — deleting a live key would silently break whatever
// customer integration still holds it, with no warning and no way back,
// unlike revoke which at least shows up as an auth failure the customer
// can trace to a still-visible row.
func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	k, err := s.st.GetAPIKeyByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "no such API key")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if k.RevokedAt == nil {
		httpjson.WriteError(w, http.StatusConflict, "revoke this API key before deleting it")
		return
	}
	if err := s.st.DeleteAPIKey(r.Context(), id); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// apiKeyEventView is the JSON shape for one row of a key's activity log.
type apiKeyEventView struct {
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
}

// handleListAPIKeyEvents serves the per-key activity-log panel on
// Administration > API Access. Admin-only, same gate as the rest of this
// screen. 404s on an unknown key rather than silently returning an empty
// list, so the UI can tell "no activity yet" apart from "this key
// doesn't exist" (e.g. a stale id after another admin deleted it).
func (s *Server) handleListAPIKeyEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := s.st.GetAPIKeyByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "no such API key")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := s.st.ListAPIKeyEvents(r.Context(), id)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]apiKeyEventView, len(events))
	for i, e := range events {
		out[i] = apiKeyEventView{Kind: e.Kind, At: e.At}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}
