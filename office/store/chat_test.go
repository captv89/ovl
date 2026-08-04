// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_InsertAndListChatMessages(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 65)
	other := createTestVessel(t, st, 66)

	vesselMsg := domain.ChatMessage{
		ID: "chat-office-1", ReportID: "report-1", Sender: "master", Body: "corrected version pushed",
		SentAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC), Direction: domain.ChatFromVessel,
	}
	officeMsg := domain.ChatMessage{
		ID: "chat-office-2", ReportID: "report-1", Sender: "reviewer1", Body: "thanks, looks good",
		SentAt: time.Date(2026, 7, 12, 9, 5, 0, 0, time.UTC), Direction: domain.ChatFromOffice,
	}
	if err := st.InsertChatMessage(ctx, v.ID, vesselMsg); err != nil {
		t.Fatalf("InsertChatMessage (vessel): %v", err)
	}
	if err := st.InsertChatMessage(ctx, v.ID, officeMsg); err != nil {
		t.Fatalf("InsertChatMessage (office): %v", err)
	}
	// A different vessel's message on the same report_id string must not
	// leak into v's list.
	if err := st.InsertChatMessage(ctx, other.ID, domain.ChatMessage{
		ID: "chat-office-3", ReportID: "report-1", Sender: "master", Body: "unrelated vessel",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromVessel,
	}); err != nil {
		t.Fatalf("InsertChatMessage (other vessel): %v", err)
	}

	got, err := st.ListChatMessages(ctx, v.ID, "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != "chat-office-1" || got[1].ID != "chat-office-2" {
		t.Errorf("got = %+v, want [chat-office-1, chat-office-2] chronological", got)
	}
}

func TestStore_ListChatMessagesSince_OfficeDirectionOnly(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 67)

	if err := st.InsertChatMessage(ctx, v.ID, domain.ChatMessage{
		ID: "chat-since-1", ReportID: "report-1", Sender: "master", Body: "from vessel",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromVessel,
	}); err != nil {
		t.Fatalf("InsertChatMessage (vessel): %v", err)
	}
	if err := st.InsertChatMessage(ctx, v.ID, domain.ChatMessage{
		ID: "chat-since-2", ReportID: "report-1", Sender: "reviewer1", Body: "from office",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromOffice,
	}); err != nil {
		t.Fatalf("InsertChatMessage (office): %v", err)
	}

	items, err := st.ListChatMessagesSince(ctx, v.ID, 0)
	if err != nil {
		t.Fatalf("ListChatMessagesSince: %v", err)
	}
	if len(items) != 1 || items[0].Message.ID != "chat-since-2" {
		t.Fatalf("items = %+v, want exactly the office-direction message", items)
	}
	if items[0].Cursor == 0 {
		t.Error("Cursor is zero, want a real seq value")
	}

	// Nothing new since the returned cursor.
	again, err := st.ListChatMessagesSince(ctx, v.ID, items[0].Cursor)
	if err != nil {
		t.Fatalf("ListChatMessagesSince (again): %v", err)
	}
	if len(again) != 0 {
		t.Errorf("items after advancing past the cursor = %+v, want empty", again)
	}
}
