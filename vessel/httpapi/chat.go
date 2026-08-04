// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/vessel/store"
)

// chatMessageView is the JSON shape for one pkg/domain.ChatMessage
// (design handoff A8's per-report chat wall).
type chatMessageView struct {
	ID        string    `json:"id"`
	ReportID  string    `json:"reportId"`
	Sender    string    `json:"sender"`
	Body      string    `json:"body"`
	SentAt    time.Time `json:"sentAt"`
	Direction string    `json:"direction"`
}

func toChatMessageView(m domain.ChatMessage) chatMessageView {
	return chatMessageView{
		ID: m.ID, ReportID: m.ReportID, Sender: m.Sender, Body: m.Body,
		SentAt: m.SentAt, Direction: string(m.Direction),
	}
}

// handleListChat returns one report's chat wall, chronological.
func (s *Server) handleListChat(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	reportID := r.PathValue("id")
	if _, err := st.GetLatestVersion(r.Context(), reportID); errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	messages, err := st.ListChatMessages(r.Context(), reportID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]chatMessageView, len(messages))
	for i, m := range messages {
		out[i] = toChatMessageView(m)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

type postChatRequest struct {
	Body string `json:"body"`
}

// handlePostChat sends a new chat message from this vessel (design
// handoff A8): stores a local direction=vessel row and enqueues it to
// the outbox in the same call, so it's queued for push without waiting
// for a separate step.
func (s *Server) handlePostChat(w http.ResponseWriter, r *http.Request) {
	st, ok := s.requireStore(w)
	if !ok {
		return
	}
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	reportID := r.PathValue("id")
	if _, err := st.GetLatestVersion(r.Context(), reportID); errors.Is(err, store.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, "report not found")
		return
	} else if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req postChatRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := domain.NewChatMessage(reportID, user.Username, req.Body, domain.ChatFromVessel)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := st.InsertChatMessage(r.Context(), m); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := st.EnqueueChatMessage(r.Context(), reportID, m.ID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toChatMessageView(m))
}
