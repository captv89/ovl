// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"

	"github.com/captv89/ovl/internal/httpjson"
	"github.com/captv89/ovl/pkg/schema"
	ovlschemas "github.com/captv89/ovl/schemas"
)

// eventTypeView is the JSON shape for one pkg/schema.EventType.
type eventTypeView struct {
	Code   string  `json:"code"`
	Remark *string `json:"remark,omitempty"`
}

// handleListEventTypes serves the full OVD event-type enum (design handoff
// A3's "Other event…" full list, and the "event-types" enumRef the curated
// Event field points to — general enumRef resolution for arbitrary fields
// isn't built, see PROJECT.md; this endpoint is scoped to event types
// specifically since that's what A3 needs).
func (s *Server) handleListEventTypes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	types, err := schema.LoadEventTypes(ovlschemas.FS, "ovd-3.13/enums/event-types.json")
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]eventTypeView, len(types))
	for i, t := range types {
		views[i] = eventTypeView{Code: t.Code, Remark: t.Remark}
	}
	httpjson.WriteJSON(w, http.StatusOK, views)
}

// eventSuggestionView is the JSON shape for one pkg/schema.EventSuggestion.
type eventSuggestionView struct {
	After   string   `json:"after"`
	Suggest []string `json:"suggest"`
}

// handleListEventSuggestions serves the curated next-event suggestion
// state machine (architecture 9.4, design handoff A3's suggested-next-
// report card).
func (s *Server) handleListEventSuggestions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedUser(w, r); !ok {
		return
	}
	suggestions, err := schema.LoadEventSuggestions(ovlschemas.FS, "ovd-3.13/event-suggestions/event-types.json")
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]eventSuggestionView, len(suggestions))
	for i, sug := range suggestions {
		views[i] = eventSuggestionView{After: sug.After, Suggest: sug.Suggest}
	}
	httpjson.WriteJSON(w, http.StatusOK, views)
}
