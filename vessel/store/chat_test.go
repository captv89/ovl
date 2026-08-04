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

	vessel := domain.ChatMessage{
		ID: "chat-1", ReportID: "report-1", Sender: "master", Body: "corrected version pushed",
		SentAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC), Direction: domain.ChatFromVessel,
	}
	office := domain.ChatMessage{
		ID: "chat-2", ReportID: "report-1", Sender: "reviewer1", Body: "thanks, looks good now",
		SentAt: time.Date(2026, 7, 12, 9, 5, 0, 0, time.UTC), Direction: domain.ChatFromOffice,
	}
	if err := st.InsertChatMessage(ctx, vessel); err != nil {
		t.Fatalf("InsertChatMessage (vessel): %v", err)
	}
	if err := st.InsertChatMessage(ctx, office); err != nil {
		t.Fatalf("InsertChatMessage (office): %v", err)
	}
	// A different report's message must not show up in report-1's list.
	if err := st.InsertChatMessage(ctx, domain.ChatMessage{
		ID: "chat-3", ReportID: "report-2", Sender: "master", Body: "unrelated",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromVessel,
	}); err != nil {
		t.Fatalf("InsertChatMessage (other report): %v", err)
	}

	got, err := st.ListChatMessages(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != "chat-1" || got[0].Direction != domain.ChatFromVessel {
		t.Errorf("got[0] = %+v, want chat-1 from vessel (chronological)", got[0])
	}
	if got[1].ID != "chat-2" || got[1].Direction != domain.ChatFromOffice {
		t.Errorf("got[1] = %+v, want chat-2 from office", got[1])
	}
}

func TestStore_GetChatMessage(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	msg := domain.ChatMessage{
		ID: "chat-get-1", ReportID: "report-1", Sender: "master", Body: "hello",
		SentAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC), Direction: domain.ChatFromVessel,
	}
	if err := st.InsertChatMessage(ctx, msg); err != nil {
		t.Fatalf("InsertChatMessage: %v", err)
	}
	got, err := st.GetChatMessage(ctx, "chat-get-1")
	if err != nil {
		t.Fatalf("GetChatMessage: %v", err)
	}
	if got.Body != "hello" || got.Sender != "master" {
		t.Errorf("got = %+v, want Body=hello Sender=master", got)
	}
	if _, err := st.GetChatMessage(ctx, "no-such-id"); err != ErrNotFound {
		t.Errorf("GetChatMessage(missing) error = %v, want ErrNotFound", err)
	}
}

func TestStore_InsertChatMessage_IsIdempotentOnPKConflict(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	msg := domain.ChatMessage{
		ID: "chat-dup", ReportID: "report-1", Sender: "master", Body: "hello",
		SentAt: time.Now().UTC(), Direction: domain.ChatFromVessel,
	}
	if err := st.InsertChatMessage(ctx, msg); err != nil {
		t.Fatalf("InsertChatMessage (first): %v", err)
	}
	// Re-pull/re-apply of the same id must not error (idempotent apply).
	if err := st.InsertChatMessage(ctx, msg); err != nil {
		t.Fatalf("InsertChatMessage (duplicate id): %v", err)
	}
	got, err := st.ListChatMessages(ctx, "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (no duplicate row)", len(got))
	}
}
