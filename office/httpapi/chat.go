// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"time"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/domain"
)

// chatMessageView is the JSON shape for one pkg/domain.ChatMessage —
// design handoff B4's Chat tab.
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

// handleListChat returns one report's full chat wall (both directions),
// chronological. Viewable by any authenticated office user — chatting is
// not a value-editing action (unlike remarks, which are reviewer-gated).
func (s *Server) handleListChat(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	messages, err := s.st.ListChatMessages(r.Context(), r.PathValue("vesselId"), r.PathValue("reportId"))
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

// handlePostChat sends a new chat message from the office (design
// handoff B4/A8). Any authenticated role may chat.
func (s *Server) handlePostChat(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticatedUser(w, r)
	if !ok {
		return
	}
	var req postChatRequest
	if err := httpjson.DecodeJSON(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := domain.NewChatMessage(r.PathValue("reportId"), user.Username, req.Body, domain.ChatFromOffice)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.InsertChatMessage(r.Context(), r.PathValue("vesselId"), m); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toChatMessageView(m))
}
