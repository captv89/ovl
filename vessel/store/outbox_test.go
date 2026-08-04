// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"

	"github.com/captv89/ovl/pkg/domain"
)

func TestStore_EnqueueReportVersion_SequenceIsMonotonic(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	first, err := s.EnqueueReportVersion(ctx, "report-1", 1)
	if err != nil {
		t.Fatalf("EnqueueReportVersion: %v", err)
	}
	second, err := s.EnqueueReportVersion(ctx, "report-2", 1)
	if err != nil {
		t.Fatalf("EnqueueReportVersion: %v", err)
	}
	if second.SequenceNo <= first.SequenceNo {
		t.Errorf("second.SequenceNo = %d, want greater than first.SequenceNo = %d", second.SequenceNo, first.SequenceNo)
	}
	if first.ItemID == "" || second.ItemID == "" {
		t.Error("ItemID is empty")
	}
	if first.ItemID == second.ItemID {
		t.Error("two enqueued items got the same ItemID")
	}
}

func TestStore_EnqueueReportAuditEvent_ReferencesEvent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eventID, err := s.AppendEvent(ctx, domain.Event{ReportID: "report-1", VersionNo: 1, Type: domain.EventSubmitted})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	item, err := s.EnqueueReportAuditEvent(ctx, "report-1", 1, eventID)
	if err != nil {
		t.Fatalf("EnqueueReportAuditEvent: %v", err)
	}
	if item.Kind != OutboxItemKindReportAuditEvent {
		t.Errorf("Kind = %q, want %q", item.Kind, OutboxItemKindReportAuditEvent)
	}
	if item.ReportEventID == nil || *item.ReportEventID != eventID {
		t.Errorf("ReportEventID = %v, want %d", item.ReportEventID, eventID)
	}
}

func TestStore_EnqueueChatMessage_ReferencesChatMessageID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	item, err := s.EnqueueChatMessage(ctx, "report-1", "chat-1")
	if err != nil {
		t.Fatalf("EnqueueChatMessage: %v", err)
	}
	if item.Kind != OutboxItemKindChat {
		t.Errorf("Kind = %q, want %q", item.Kind, OutboxItemKindChat)
	}
	if item.ChatMessageID == nil || *item.ChatMessageID != "chat-1" {
		t.Errorf("ChatMessageID = %v, want chat-1", item.ChatMessageID)
	}
	if item.ReportID != "report-1" {
		t.Errorf("ReportID = %q, want report-1", item.ReportID)
	}

	items, err := s.ListOutboxItems(ctx)
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	if len(items) != 1 || items[0].Kind != OutboxItemKindChat {
		t.Errorf("ListOutboxItems = %+v, want one chat item", items)
	}
}

func TestStore_ListOutboxItems_OrderedBySequence(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.EnqueueReportVersion(ctx, "report-a", 1); err != nil {
		t.Fatalf("EnqueueReportVersion: %v", err)
	}
	if _, err := s.EnqueueReportVersion(ctx, "report-b", 1); err != nil {
		t.Fatalf("EnqueueReportVersion: %v", err)
	}

	items, err := s.ListOutboxItems(ctx)
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ReportID != "report-a" || items[1].ReportID != "report-b" {
		t.Errorf("order = [%s, %s], want [report-a, report-b] (enqueue order)", items[0].ReportID, items[1].ReportID)
	}
}

func TestStore_AckOutboxItem_RemovesIt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	item, err := s.EnqueueReportVersion(ctx, "report-1", 1)
	if err != nil {
		t.Fatalf("EnqueueReportVersion: %v", err)
	}
	if err := s.AckOutboxItem(ctx, item.ItemID); err != nil {
		t.Fatalf("AckOutboxItem: %v", err)
	}

	items, err := s.ListOutboxItems(ctx)
	if err != nil {
		t.Fatalf("ListOutboxItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("ListOutboxItems after ack = %v, want empty", items)
	}
}

func TestStore_GetEventByID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	id, err := s.AppendEvent(ctx, domain.Event{ReportID: "report-1", VersionNo: 1, Type: domain.EventSubmitted, Actor: "master"})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	got, err := s.GetEventByID(ctx, id)
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	if got.ReportID != "report-1" || got.Type != domain.EventSubmitted || got.Actor != "master" {
		t.Errorf("GetEventByID = %+v, want ReportID=report-1 Type=submitted Actor=master", got)
	}

	if _, err := s.GetEventByID(ctx, 999999); err != ErrNotFound {
		t.Errorf("GetEventByID(missing) error = %v, want ErrNotFound", err)
	}
}
