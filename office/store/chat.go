// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"

	"github.com/captv89/ovl/pkg/domain"
)

// InsertChatMessage lands one chat message for vesselID, either landed
// from a vessel-authored PushOutbox item (direction vessel) or authored
// through office HTTP (direction office).
func (s *Store) InsertChatMessage(ctx context.Context, vesselID string, m domain.ChatMessage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_messages (id, vessel_id, report_id, sender, body, sent_at, direction)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, m.ID, vesselID, m.ReportID, m.Sender, m.Body, m.SentAt, string(m.Direction))
	if err != nil {
		return fmt.Errorf("insert chat message %s for report %s, vessel %s: %w", m.ID, m.ReportID, vesselID, err)
	}
	return nil
}

// ListChatMessages returns one report's full chat wall (both
// directions), chronological — design handoff B4's Chat tab.
func (s *Store) ListChatMessages(ctx context.Context, vesselID, reportID string) ([]domain.ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, sender, body, sent_at, direction
		FROM chat_messages WHERE vessel_id = $1 AND report_id = $2 ORDER BY sent_at ASC
	`, vesselID, reportID)
	if err != nil {
		return nil, fmt.Errorf("list chat messages for report %s, vessel %s: %w", reportID, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []domain.ChatMessage
	for rows.Next() {
		m, err := scanChatMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}
	return out, nil
}

// ChatMessageCursorItem pairs an office-authored chat message with its
// pull cursor value — mirrors SchemaVersionCursorItem's own shape.
type ChatMessageCursorItem struct {
	Message domain.ChatMessage
	Cursor  int64
}

// ListChatMessagesSince returns vesselID's office-authored chat messages
// (direction=office only — a vessel only ever pulls what the office
// sent it, never its own messages echoed back) with seq > sinceCursor,
// cursor ascending — PullInbox's own vessel-scoped stream.
func (s *Store) ListChatMessagesSince(ctx context.Context, vesselID string, sinceCursor int64) ([]ChatMessageCursorItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, sender, body, sent_at, direction, seq
		FROM chat_messages WHERE vessel_id = $1 AND direction = 'office' AND seq > $2 ORDER BY seq ASC
	`, vesselID, sinceCursor)
	if err != nil {
		return nil, fmt.Errorf("list chat messages since cursor %d, vessel %s: %w", sinceCursor, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []ChatMessageCursorItem
	for rows.Next() {
		var (
			m         domain.ChatMessage
			direction string
			cursor    int64
		)
		if err := rows.Scan(&m.ID, &m.ReportID, &m.Sender, &m.Body, &m.SentAt, &direction, &cursor); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		m.Direction = domain.ChatDirection(direction)
		out = append(out, ChatMessageCursorItem{Message: m, Cursor: cursor})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}
	return out, nil
}

func scanChatMessage(row rowScanner) (domain.ChatMessage, error) {
	var (
		m         domain.ChatMessage
		direction string
	)
	if err := row.Scan(&m.ID, &m.ReportID, &m.Sender, &m.Body, &m.SentAt, &direction); err != nil {
		return domain.ChatMessage{}, fmt.Errorf("scan chat message: %w", err)
	}
	m.Direction = domain.ChatDirection(direction)
	return m, nil
}
