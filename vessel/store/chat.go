// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// InsertChatMessage stores one chat message, either authored locally
// (direction vessel) or pulled from the office (direction office).
// ON CONFLICT DO NOTHING makes this idempotent on id — a re-pull of an
// already-applied office message (Slice S4's ApplyInboxPull extension)
// is a harmless no-op, matching schema_versions/config_bundles' own
// insert-if-absent idiom.
func (s *Store) InsertChatMessage(ctx context.Context, m domain.ChatMessage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_messages (id, report_id, sender, body, sent_at, direction)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`, m.ID, m.ReportID, m.Sender, m.Body, m.SentAt.UTC().Format(timeLayout), string(m.Direction))
	if err != nil {
		return fmt.Errorf("insert chat message %s for report %s: %w", m.ID, m.ReportID, err)
	}
	return nil
}

// ListChatMessages returns reportID's chat wall, chronological (design
// handoff A8: "text-only... both directions").
func (s *Store) ListChatMessages(ctx context.Context, reportID string) ([]domain.ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, sender, body, sent_at, direction
		FROM chat_messages WHERE report_id = ? ORDER BY sent_at ASC
	`, reportID)
	if err != nil {
		return nil, fmt.Errorf("list chat messages for report %s: %w", reportID, err)
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
		return nil, fmt.Errorf("iterate chat messages for report %s: %w", reportID, err)
	}
	return out, nil
}

// GetChatMessage returns one chat message by id — used to hydrate a
// chatMessage outbox item's full content before converting it to the
// proto wire type (mirroring GetEventByID's role for audit events).
// Returns ErrNotFound if id doesn't exist.
func (s *Store) GetChatMessage(ctx context.Context, id string) (domain.ChatMessage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, report_id, sender, body, sent_at, direction FROM chat_messages WHERE id = ?
	`, id)
	m, err := scanChatMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChatMessage{}, ErrNotFound
	}
	return m, err
}

func scanChatMessage(row rowScanner) (domain.ChatMessage, error) {
	var (
		m         domain.ChatMessage
		sentAt    string
		direction string
	)
	if err := row.Scan(&m.ID, &m.ReportID, &m.Sender, &m.Body, &sentAt, &direction); err != nil {
		return domain.ChatMessage{}, fmt.Errorf("scan chat message: %w", err)
	}
	var err error
	if m.SentAt, err = time.Parse(timeLayout, sentAt); err != nil {
		return domain.ChatMessage{}, fmt.Errorf("parse chat message sent_at: %w", err)
	}
	m.Direction = domain.ChatDirection(direction)
	return m, nil
}
