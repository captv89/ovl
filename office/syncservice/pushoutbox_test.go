// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/captv89/ovl/pkg/domain"
	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
)

func reportVersionItem(itemID string, seq int64, reportID string, versionNo int32) *syncv1.OutboxItem {
	fields, err := structpb.NewStruct(map[string]any{"Charterer": "Acme Shipping"})
	if err != nil {
		panic(err)
	}
	return &syncv1.OutboxItem{
		ItemId:     itemID,
		SequenceNo: seq,
		Payload: &syncv1.OutboxItem_ReportVersion{
			ReportVersion: &syncv1.ReportVersion{
				ReportId:   reportID,
				VersionNo:  versionNo,
				SchemaKind: syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_COMMERCIAL_PERIOD,
				EventType:  "Departure",
				State:      syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_SUBMITTED,
				EventTime:  timestamppb.New(time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)),
				Fields:     fields,
			},
		},
	}
}

// reportVersionItemAt is reportVersionItem with a caller-chosen
// eventTime/eventType, for cascade tests that need to construct a
// specific chain shape (e.g. two reports colliding at the same UTC
// minute).
func reportVersionItemAt(itemID string, seq int64, reportID string, versionNo int32, eventTime time.Time, eventType string) *syncv1.OutboxItem {
	fields, err := structpb.NewStruct(map[string]any{"Period_Id": reportID})
	if err != nil {
		panic(err)
	}
	return &syncv1.OutboxItem{
		ItemId:     itemID,
		SequenceNo: seq,
		Payload: &syncv1.OutboxItem_ReportVersion{
			ReportVersion: &syncv1.ReportVersion{
				ReportId:   reportID,
				VersionNo:  versionNo,
				SchemaKind: syncv1.ReportSchemaKind_REPORT_SCHEMA_KIND_COMMERCIAL_PERIOD,
				EventType:  eventType,
				State:      syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_SUBMITTED,
				EventTime:  timestamppb.New(eventTime),
				Fields:     fields,
			},
		},
	}
}

// reportVersionItemState is reportVersionItem with a caller-chosen
// lifecycle state, for the cascade re-sync test that re-pushes an
// already-landed version after its state flipped.
func reportVersionItemState(itemID string, seq int64, reportID string, versionNo int32, state syncv1.ReportLifecycleState) *syncv1.OutboxItem {
	item := reportVersionItem(itemID, seq, reportID, versionNo)
	item.GetReportVersion().State = state
	return item
}

func auditEventItem(itemID string, seq int64, reportID string, versionNo int32) *syncv1.OutboxItem {
	return &syncv1.OutboxItem{
		ItemId:     itemID,
		SequenceNo: seq,
		Payload: &syncv1.OutboxItem_AuditEvent{
			AuditEvent: &syncv1.ReportAuditEvent{
				ReportId:   reportID,
				VersionNo:  versionNo,
				EventType:  syncv1.AuditEventType_AUDIT_EVENT_TYPE_SUBMITTED,
				Actor:      "master",
				OccurredAt: timestamppb.New(time.Now().UTC()),
			},
		},
	}
}

func TestPushOutbox_LandsReportVersionAndAuditEvent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 82)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	resp, err := client.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{
		Items: []*syncv1.OutboxItem{
			reportVersionItem("item-1", 1, "report-1", 1),
			auditEventItem("item-2", 2, "report-1", 1),
		},
	}))
	if err != nil {
		t.Fatalf("PushOutbox: %v", err)
	}
	if len(resp.Msg.GetAcks()) != 2 {
		t.Fatalf("len(Acks) = %d, want 2", len(resp.Msg.GetAcks()))
	}
	for _, ack := range resp.Msg.GetAcks() {
		if !ack.GetAccepted() {
			t.Errorf("ack for %s: accepted=false, error=%q", ack.GetItemId(), ack.GetError())
		}
	}

	landed, err := st.GetReportVersion(ctx, v.ID, "report-1", 1)
	if err != nil {
		t.Fatalf("GetReportVersion: %v", err)
	}
	if landed.SchemaName != "commercial-period" || landed.Fields["Charterer"] != "Acme Shipping" {
		t.Errorf("landed = %+v, want SchemaName=commercial-period Fields[Charterer]=Acme Shipping", landed)
	}
}

func TestPushOutbox_RetransmissionIsIdempotent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 83)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	batch := &syncv1.PushOutboxRequest{Items: []*syncv1.OutboxItem{reportVersionItem("item-1", 1, "report-1", 1)}}

	if _, err := client.PushOutbox(ctx, connect.NewRequest(batch)); err != nil {
		t.Fatalf("PushOutbox (first): %v", err)
	}
	// Simulate a dropped link: the same batch is sent again. The
	// underlying no-duplicate-row guarantee is verified directly at the
	// store layer (office/store's TestStore_OutboxReceipts); this only
	// needs to confirm the RPC surfaces it as a harmless re-ack.
	resp, err := client.PushOutbox(ctx, connect.NewRequest(batch))
	if err != nil {
		t.Fatalf("PushOutbox (retransmit): %v", err)
	}
	if !resp.Msg.GetAcks()[0].GetAccepted() {
		t.Errorf("retransmitted item not accepted: %q", resp.Msg.GetAcks()[0].GetError())
	}

	has, err := st.HasOutboxReceipt(ctx, v.ID, "item-1")
	if err != nil {
		t.Fatalf("HasOutboxReceipt: %v", err)
	}
	if !has {
		t.Error("HasOutboxReceipt = false after two pushes, want true")
	}
}

func TestPushOutbox_PartialBatchFailureDoesNotBlockOthers(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 84)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	badItem := &syncv1.OutboxItem{ItemId: "item-bad", SequenceNo: 1} // no payload set
	goodItem := reportVersionItem("item-good", 2, "report-1", 1)

	resp, err := client.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{Items: []*syncv1.OutboxItem{badItem, goodItem}}))
	if err != nil {
		t.Fatalf("PushOutbox: %v", err)
	}
	acks := resp.Msg.GetAcks()
	if len(acks) != 2 {
		t.Fatalf("len(Acks) = %d, want 2", len(acks))
	}
	if acks[0].GetAccepted() {
		t.Error("ack for item with no payload: accepted=true, want false")
	}
	if !acks[1].GetAccepted() {
		t.Errorf("ack for well-formed item: accepted=false, error=%q", acks[1].GetError())
	}
	if _, err := st.GetReportVersion(ctx, v.ID, "report-1", 1); err != nil {
		t.Errorf("GetReportVersion for the good item: %v", err)
	}
}

// TestPushOutbox_CascadeRevalidatesChainAfterLanding covers T2.3: landing
// a report version must trigger cascade revalidation for its
// (vessel, schema) chain within the same PushOutbox call, not as a
// separate later step.
func TestPushOutbox_CascadeRevalidatesChainAfterLanding(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 85)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	sameMinute := time.Date(2026, 7, 12, 12, 0, 30, 0, time.UTC)
	item1 := reportVersionItemAt("item-a", 1, "cascade-report-a", 1, sameMinute, "Other event")
	item2 := reportVersionItemAt("item-b", 2, "cascade-report-b", 1, sameMinute.Add(10*time.Second), "Other event")

	if _, err := client.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{Items: []*syncv1.OutboxItem{item1, item2}})); err != nil {
		t.Fatalf("PushOutbox: %v", err)
	}

	for _, reportID := range []string{"cascade-report-a", "cascade-report-b"} {
		got, err := st.GetReportVersion(ctx, v.ID, reportID, 1)
		if err != nil {
			t.Fatalf("GetReportVersion(%s): %v", reportID, err)
		}
		if got.State != domain.StateInvalidated {
			t.Errorf("%s.State = %q, want invalidated (cascade should have run within the push call)", reportID, got.State)
		}
	}
}

// TestPushOutbox_ReSyncsCascadeInvalidatedVersion closes the second half
// of the 2026-07-16 cascade-invalidation sync finding (codebase audit
// 2026-07-22 §3.2): after a vessel cascade-invalidates an already-
// submitted report, vessel/httpapi.runCascade re-enqueues that version and
// pushes it again — with a fresh outbox item id and state=invalidated. The
// office must land the state flip, not silently keep the stale submitted
// row (UpsertReportVersion's ON CONFLICT DO UPDATE covers this; the worry
// was that it was never confirmed end to end). A single report with no
// colliding chain isolates the landing from office's own cascade.
func TestPushOutbox_ReSyncsCascadeInvalidatedVersion(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 96)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	// First push: the report lands submitted.
	if _, err := client.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{
		Items: []*syncv1.OutboxItem{reportVersionItem("item-submitted", 1, "resync-report", 1)},
	})); err != nil {
		t.Fatalf("PushOutbox (submitted): %v", err)
	}
	got, err := st.GetReportVersion(ctx, v.ID, "resync-report", 1)
	if err != nil {
		t.Fatalf("GetReportVersion (after submit): %v", err)
	}
	if got.State != domain.StateSubmitted {
		t.Fatalf("state after first push = %q, want submitted", got.State)
	}

	// Vessel cascade-invalidated it and re-enqueued: same (report, version),
	// fresh item id, state now invalidated.
	if _, err := client.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{
		Items: []*syncv1.OutboxItem{
			reportVersionItemState("item-invalidated", 2, "resync-report", 1, syncv1.ReportLifecycleState_REPORT_LIFECYCLE_STATE_INVALIDATED),
		},
	})); err != nil {
		t.Fatalf("PushOutbox (invalidated re-sync): %v", err)
	}

	got, err = st.GetReportVersion(ctx, v.ID, "resync-report", 1)
	if err != nil {
		t.Fatalf("GetReportVersion (after re-sync): %v", err)
	}
	if got.State != domain.StateInvalidated {
		t.Errorf("state after re-sync = %q, want invalidated (the flip must land, not stay stale)", got.State)
	}
}

// chatMessageItem builds an OutboxItem_ChatMessage payload for tests.
func chatMessageItem(itemID string, seq int64, chatID, reportID string, sentAt time.Time) *syncv1.OutboxItem {
	return &syncv1.OutboxItem{
		ItemId:     itemID,
		SequenceNo: seq,
		Payload: &syncv1.OutboxItem_ChatMessage{
			ChatMessage: &syncv1.ChatMessage{
				Id:       chatID,
				ReportId: reportID,
				Sender:   "master",
				Body:     "corrected version pushed",
				SentAt:   timestamppb.New(sentAt),
			},
		},
	}
}

// TestPushOutbox_LandsChatMessage covers T3.3: a pushed chat message
// must actually land in chat_messages (direction=vessel), not just be
// acked-and-discarded (the Phase 4 no-op this replaces).
func TestPushOutbox_LandsChatMessage(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 86)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	sentAt := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	resp, err := client.PushOutbox(ctx, connect.NewRequest(&syncv1.PushOutboxRequest{
		Items: []*syncv1.OutboxItem{chatMessageItem("item-chat-1", 1, "chat-1", "report-1", sentAt)},
	}))
	if err != nil {
		t.Fatalf("PushOutbox: %v", err)
	}
	if !resp.Msg.GetAcks()[0].GetAccepted() {
		t.Fatalf("ack not accepted: %q", resp.Msg.GetAcks()[0].GetError())
	}

	landed, err := st.ListChatMessages(ctx, v.ID, "report-1")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(landed) != 1 || landed[0].ID != "chat-1" || landed[0].Direction != domain.ChatFromVessel {
		t.Errorf("landed = %+v, want one message id=chat-1 direction=vessel", landed)
	}
}

func TestPushOutbox_MissingCredential(t *testing.T) {
	st := openTestStore(t)
	srv := newTestServer(t, st)
	client := newTestClient(srv, "")

	_, err := client.PushOutbox(context.Background(), connect.NewRequest(&syncv1.PushOutboxRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("PushOutbox (no credential) error = %v, want CodeUnauthenticated", err)
	}
}
