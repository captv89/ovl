// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutboxItemKind discriminates which oneof field of the proto's
// OutboxItem message a row represents (proto/ovl/sync/v1/sync.proto).
type OutboxItemKind string

const (
	OutboxItemKindReportVersion    OutboxItemKind = "reportVersion"
	OutboxItemKindReportAuditEvent OutboxItemKind = "reportAuditEvent"
	OutboxItemKindChat             OutboxItemKind = "chatMessage"
)

// OutboxItem is one row pending push to the office (architecture 11.2
// step 1). ReportEventID is set only when Kind is
// OutboxItemKindReportAuditEvent; ChatMessageID is set only when Kind is
// OutboxItemKindChat.
type OutboxItem struct {
	ItemID        string
	SequenceNo    int64
	Kind          OutboxItemKind
	ReportID      string
	VersionNo     int
	ReportEventID *int64
	ChatMessageID *string
	CreatedAt     time.Time
}

// EnqueueReportVersion adds an outbox item referencing the given report
// version, to be looked up fresh from `reports` at push time (submitted
// versions are immutable, so there is nothing to duplicate here).
func (s *Store) EnqueueReportVersion(ctx context.Context, reportID string, versionNo int) (OutboxItem, error) {
	return s.enqueueOutboxItem(ctx, OutboxItemKindReportVersion, reportID, versionNo, nil, nil)
}

// EnqueueReportAuditEvent adds an outbox item referencing the given
// report_events row (its AUTOINCREMENT id, as returned by AppendEvent).
func (s *Store) EnqueueReportAuditEvent(ctx context.Context, reportID string, versionNo int, reportEventID int64) (OutboxItem, error) {
	return s.enqueueOutboxItem(ctx, OutboxItemKindReportAuditEvent, reportID, versionNo, &reportEventID, nil)
}

// EnqueueChatMessage adds an outbox item referencing the given
// chat_messages row by id, to be looked up fresh at push time. versionNo
// has no meaning for a chat message (unlike the other two kinds) but the
// column is NOT NULL, so this always enqueues 0.
func (s *Store) EnqueueChatMessage(ctx context.Context, reportID, chatMessageID string) (OutboxItem, error) {
	return s.enqueueOutboxItem(ctx, OutboxItemKindChat, reportID, 0, nil, &chatMessageID)
}

// enqueueOutboxItem assigns the next per-vessel sequence number and
// inserts the row in one transaction (architecture 11.2: "per-vessel
// monotonic sequence number") — see outbox_sequence's migration comment
// for why this reads-then-increments a counter row rather than deriving
// the next value from MAX(sequence_no).
func (s *Store) enqueueOutboxItem(ctx context.Context, kind OutboxItemKind, reportID string, versionNo int, reportEventID *int64, chatMessageID *string) (OutboxItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OutboxItem{}, fmt.Errorf("begin outbox enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT next_no FROM outbox_sequence WHERE id = 1`).Scan(&seq); err != nil {
		return OutboxItem{}, fmt.Errorf("read outbox sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE outbox_sequence SET next_no = ? WHERE id = 1`, seq+1); err != nil {
		return OutboxItem{}, fmt.Errorf("advance outbox sequence: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return OutboxItem{}, fmt.Errorf("generate outbox item id: %w", err)
	}
	item := OutboxItem{
		ItemID: id.String(), SequenceNo: seq, Kind: kind,
		ReportID: reportID, VersionNo: versionNo, ReportEventID: reportEventID, ChatMessageID: chatMessageID,
		CreatedAt: time.Now().UTC(),
	}
	var eventIDVal any
	if reportEventID != nil {
		eventIDVal = *reportEventID
	}
	var chatMessageIDVal any
	if chatMessageID != nil {
		chatMessageIDVal = *chatMessageID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox (item_id, sequence_no, kind, report_id, version_no, report_event_id, chat_message_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ItemID, item.SequenceNo, string(item.Kind), item.ReportID, item.VersionNo, eventIDVal, chatMessageIDVal, item.CreatedAt.UTC().Format(timeLayout)); err != nil {
		return OutboxItem{}, fmt.Errorf("insert outbox item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return OutboxItem{}, fmt.Errorf("commit outbox enqueue: %w", err)
	}
	return item, nil
}

// ListOutboxItems returns every pending (unacked) outbox item, oldest
// first — the batch RunSyncCycle's PushOutbox phase sends.
func (s *Store) ListOutboxItems(ctx context.Context) ([]OutboxItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT item_id, sequence_no, kind, report_id, version_no, report_event_id, chat_message_id, created_at
		FROM outbox ORDER BY sequence_no ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list outbox items: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []OutboxItem
	for rows.Next() {
		var (
			item            OutboxItem
			kind, createdAt string
			reportEventID   *int64
			chatMessageID   *string
		)
		if err := rows.Scan(&item.ItemID, &item.SequenceNo, &kind, &item.ReportID, &item.VersionNo, &reportEventID, &chatMessageID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan outbox item: %w", err)
		}
		item.Kind = OutboxItemKind(kind)
		item.ReportEventID = reportEventID
		item.ChatMessageID = chatMessageID
		if item.CreatedAt, err = time.Parse(timeLayout, createdAt); err != nil {
			return nil, fmt.Errorf("parse outbox item created_at: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox items: %w", err)
	}
	return out, nil
}

// AckOutboxItem deletes an acked item — there is nothing further to do
// with it once the office has confirmed receipt (PushOutboxResponse's
// ItemAck.accepted), and deleting keeps this table sized to only what's
// still pending, not a growing history (the office's own outbox_receipts
// is where "what has this vessel ever pushed" lives, if that's ever
// needed).
func (s *Store) AckOutboxItem(ctx context.Context, itemID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM outbox WHERE item_id = ?`, itemID); err != nil {
		return fmt.Errorf("ack outbox item %s: %w", itemID, err)
	}
	return nil
}
