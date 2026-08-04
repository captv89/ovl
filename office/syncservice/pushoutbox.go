// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/pkg/domain"
	"github.com/captv89/ovl/pkg/syncproto"
)

var errUnknownOutboxPayload = errors.New("syncservice: outbox item has no recognized payload")

// PushOutbox lands report versions and audit events pushed by the
// calling vessel (architecture 11.2 step 1), upserting idempotently by
// item id so retransmission after a dropped link (proto's OutboxItem
// doc comment) is harmless — every item in the batch gets its own
// accept/reject Ack rather than the whole call failing together, so one
// malformed item can't block the rest of a batch.
func (s *Server) PushOutbox(ctx context.Context, req *connect.Request[syncv1.PushOutboxRequest]) (*connect.Response[syncv1.PushOutboxResponse], error) {
	vesselID, ok := VesselIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errUnauthenticatedContext)
	}

	items := req.Msg.GetItems()
	acks := make([]*syncv1.ItemAck, 0, len(items))
	for _, item := range items {
		acks = append(acks, s.applyOutboxItem(ctx, vesselID, item))
	}
	return connect.NewResponse(&syncv1.PushOutboxResponse{Acks: acks}), nil
}

// applyOutboxItem lands one item and records its receipt, or returns why
// it couldn't. A previously-received item_id is recognized before doing
// any work and acked again as a no-op — the idempotency guarantee itself.
func (s *Server) applyOutboxItem(ctx context.Context, vesselID string, item *syncv1.OutboxItem) *syncv1.ItemAck {
	itemID := item.GetItemId()

	already, err := s.st.HasOutboxReceipt(ctx, vesselID, itemID)
	if err != nil {
		return rejectedAck(itemID, err)
	}
	if already {
		return &syncv1.ItemAck{ItemId: itemID, Accepted: true}
	}

	now := time.Now().UTC()
	switch payload := item.GetPayload().(type) {
	case *syncv1.OutboxItem_ReportVersion:
		r, err := syncproto.ReportVersionToDomain(payload.ReportVersion)
		if err != nil {
			return rejectedAck(itemID, err)
		}
		if err := s.st.UpsertReportVersion(ctx, vesselID, r, payload.ReportVersion.GetSchemaVersion(), now); err != nil {
			return rejectedAck(itemID, err)
		}
		// Cascade revalidation (architecture 8.3) runs synchronously right
		// after landing (Phase 5 open question 5's resolved default) so a
		// dependent later report's invalidation is visible within this same
		// push call. A cascade failure must not reject an item that already
		// landed successfully — log and continue, matching outbox
		// semantics' own "the item is accepted" contract.
		if err := runCascade(ctx, s.st, vesselID, r.SchemaName); err != nil {
			slog.Error("cascade revalidation failed", "vesselId", vesselID, "schema", r.SchemaName, "error", err)
		}
	case *syncv1.OutboxItem_AuditEvent:
		e, err := syncproto.ReportAuditEventToDomain(payload.AuditEvent)
		if err != nil {
			return rejectedAck(itemID, err)
		}
		if err := s.st.AppendReportAuditEvent(ctx, vesselID, e, now, "vessel"); err != nil {
			return rejectedAck(itemID, err)
		}
	case *syncv1.OutboxItem_ChatMessage:
		// direction is always vessel here — a chat message pushed via
		// PushOutbox was, by construction, authored on the vessel side (see
		// domain.ChatMessage's own doc comment on how direction is inferred
		// structurally rather than carried on the wire).
		m := syncproto.ChatMessageToDomain(payload.ChatMessage, domain.ChatFromVessel)
		if err := s.st.InsertChatMessage(ctx, vesselID, m); err != nil {
			return rejectedAck(itemID, err)
		}
	default:
		return rejectedAck(itemID, errUnknownOutboxPayload)
	}

	if err := s.st.RecordOutboxReceipt(ctx, vesselID, itemID, item.GetSequenceNo(), now); err != nil {
		return rejectedAck(itemID, err)
	}
	return &syncv1.ItemAck{ItemId: itemID, Accepted: true}
}

func rejectedAck(itemID string, err error) *syncv1.ItemAck {
	return &syncv1.ItemAck{ItemId: itemID, Accepted: false, Error: err.Error()}
}
