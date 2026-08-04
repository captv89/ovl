// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// AppendEvent adds one row to the append-only audit trail (architecture
// 14) and returns its id — report_events' own AUTOINCREMENT primary key,
// needed by callers that go on to reference this exact row (e.g.
// EnqueueReportAuditEvent, Phase 4's outbox). There is no update/delete
// counterpart by design.
func (s *Store) AppendEvent(ctx context.Context, e domain.Event) (int64, error) {
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return 0, fmt.Errorf("marshal event detail: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO report_events (report_id, version_no, type, at, actor, detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`, e.ReportID, e.VersionNo, string(e.Type), e.At.UTC().Format(timeLayout), e.Actor, string(detailJSON))
	if err != nil {
		return 0, fmt.Errorf("append event for report %s: %w", e.ReportID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get id of appended event for report %s: %w", e.ReportID, err)
	}
	return id, nil
}

// ListEvents returns every audit event for reportID (all versions),
// oldest first — the chronological per-report screen architecture 14
// describes.
func (s *Store) ListEvents(ctx context.Context, reportID string) ([]domain.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT report_id, version_no, type, at, actor, detail
		FROM report_events
		WHERE report_id = ?
		ORDER BY at ASC, id ASC
	`, reportID)
	if err != nil {
		return nil, fmt.Errorf("list events for report %s: %w", reportID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []domain.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events for report %s: %w", reportID, err)
	}
	return out, nil
}

// GetEventByID returns one specific report_events row by its
// AUTOINCREMENT id (as returned by AppendEvent) — used to hydrate a
// reportAuditEvent outbox item's full Event before converting it to the
// proto wire type (Phase 4's PushOutbox). Returns ErrNotFound if id
// doesn't exist.
func (s *Store) GetEventByID(ctx context.Context, id int64) (domain.Event, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT report_id, version_no, type, at, actor, detail FROM report_events WHERE id = ?
	`, id)
	e, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Event{}, ErrNotFound
	}
	return e, err
}

func scanEvent(row rowScanner) (domain.Event, error) {
	var (
		e          domain.Event
		eventType  string
		at         string
		detailJSON string
	)
	if err := row.Scan(&e.ReportID, &e.VersionNo, &eventType, &at, &e.Actor, &detailJSON); err != nil {
		return domain.Event{}, fmt.Errorf("scan event: %w", err)
	}
	e.Type = domain.EventType(eventType)
	var err error
	if e.At, err = time.Parse(timeLayout, at); err != nil {
		return domain.Event{}, fmt.Errorf("parse event at: %w", err)
	}
	if err := json.Unmarshal([]byte(detailJSON), &e.Detail); err != nil {
		return domain.Event{}, fmt.Errorf("unmarshal event detail: %w", err)
	}
	return e, nil
}
