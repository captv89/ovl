// SPDX-License-Identifier: AGPL-3.0-only

package httpapi

import (
	"net/http"
	"testing"
)

func TestHandleListEventTypes(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/event-types", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/event-types: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[[]eventTypeView](t, rec)
	if len(got) != 33 {
		t.Errorf("len(event types) = %d, want 33 (the full OVD 3.13 event-type enum)", len(got))
	}
}

func TestHandleListEventSuggestions(t *testing.T) {
	_, c := newLoggedInTestServer(t)
	rec := c.do(http.MethodGet, "/api/event-suggestions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/event-suggestions: status %d, body %s", rec.Code, rec.Body)
	}
	got := decodeBody[[]eventSuggestionView](t, rec)
	found := false
	for _, s := range got {
		if s.After == "Departure" {
			found = true
			if len(s.Suggest) == 0 {
				t.Error("suggestions after Departure is empty, want at least Noon-at-sea/EOSP/Arrival")
			}
		}
	}
	if !found {
		t.Error("no suggestion entry for \"Departure\"")
	}
}
