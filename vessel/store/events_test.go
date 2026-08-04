// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_AppendAndListEvents(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	reportID := "r1"

	events := []domain.Event{
		{ReportID: reportID, VersionNo: 1, Type: domain.EventCreated, At: time.Now().UTC(), Actor: "master"},
		{ReportID: reportID, VersionNo: 1, Type: domain.EventSectionSaved, At: time.Now().UTC().Add(time.Minute), Actor: "2/O",
			Detail: map[string]any{"section": "distanceAndSpeed"}},
	}
	var lastID int64
	for _, e := range events {
		id, err := s.AppendEvent(ctx, e)
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		if id <= lastID {
			t.Errorf("AppendEvent id = %d, want increasing (previous %d)", id, lastID)
		}
		lastID = id
	}

	got, err := s.ListEvents(ctx, reportID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListEvents returned %d events, want 2", len(got))
	}
	if got[0].Type != domain.EventCreated || got[1].Type != domain.EventSectionSaved {
		t.Errorf("ListEvents order = [%q, %q], want chronological [created, section_saved]", got[0].Type, got[1].Type)
	}
	if got[1].Detail["section"] != "distanceAndSpeed" {
		t.Errorf("Detail[section] = %v, want distanceAndSpeed", got[1].Detail["section"])
	}

	none, err := s.ListEvents(ctx, "no-such-report")
	if err != nil {
		t.Fatalf("ListEvents(no-such-report): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListEvents(no-such-report) = %v, want empty", none)
	}
}
