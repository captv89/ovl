// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// InsertRemarkSet lands one B4 "send remark set" action as N rows
// sharing remarkSetID (office-only grouping — see the remarks table's
// own migration comment on why the wire Remark message doesn't carry
// this id).
func (s *Store) InsertRemarkSet(ctx context.Context, vesselID string, remarks []domain.Remark, remarkSetID string) error {
	for _, r := range remarks {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO remarks (id, remark_set_id, vessel_id, report_id, version_no, field_name, body, author, created_at, resolved)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, r.ID, remarkSetID, vesselID, r.ReportID, r.VersionNo, r.FieldName, r.Body, r.Author, r.CreatedAt, r.Resolved)
		if err != nil {
			return fmt.Errorf("insert remark %s for report %s, vessel %s: %w", r.ID, r.ReportID, vesselID, err)
		}
	}
	return nil
}

// ListRemarks returns every remark ever sent for one report (all
// versions), oldest first — design handoff B4/A7's Remarks tab.
func (s *Store) ListRemarks(ctx context.Context, vesselID, reportID string) ([]domain.Remark, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, version_no, field_name, body, author, created_at, resolved
		FROM remarks WHERE vessel_id = $1 AND report_id = $2 ORDER BY created_at ASC, id ASC
	`, vesselID, reportID)
	if err != nil {
		return nil, fmt.Errorf("list remarks for report %s, vessel %s: %w", reportID, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []domain.Remark
	for rows.Next() {
		r, err := scanRemark(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remarks: %w", err)
	}
	return out, nil
}

// SetRemarkResolved toggles one remark's resolved flag (design handoff
// B4: Reviewer-only manual toggle, per Phase 5 open question 2's
// resolved default — no auto-infer from a later synced value).
func (s *Store) SetRemarkResolved(ctx context.Context, id string, resolved bool) error {
	var resolvedAt any
	if resolved {
		resolvedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE remarks SET resolved = $1, resolved_at = $2 WHERE id = $3
	`, resolved, resolvedAt, id)
	if err != nil {
		return fmt.Errorf("set remark %s resolved=%v: %w", id, resolved, err)
	}
	return nil
}

// RemarkCursorItem pairs a remark with its pull cursor value — mirrors
// ChatMessageCursorItem/InvalidationNoticeCursorItem's own shape.
type RemarkCursorItem struct {
	Remark domain.Remark
	Cursor int64
}

// ListRemarksSince returns vesselID's remarks with seq > sinceCursor,
// cursor ascending — PullInbox's own vessel-scoped stream (Slice S5).
func (s *Store) ListRemarksSince(ctx context.Context, vesselID string, sinceCursor int64) ([]RemarkCursorItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, version_no, field_name, body, author, created_at, resolved, seq
		FROM remarks WHERE vessel_id = $1 AND seq > $2 ORDER BY seq ASC
	`, vesselID, sinceCursor)
	if err != nil {
		return nil, fmt.Errorf("list remarks since cursor %d, vessel %s: %w", sinceCursor, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []RemarkCursorItem
	for rows.Next() {
		var (
			r      domain.Remark
			cursor int64
		)
		if err := rows.Scan(&r.ID, &r.ReportID, &r.VersionNo, &r.FieldName, &r.Body, &r.Author, &r.CreatedAt, &r.Resolved, &cursor); err != nil {
			return nil, fmt.Errorf("scan remark: %w", err)
		}
		out = append(out, RemarkCursorItem{Remark: r, Cursor: cursor})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remarks: %w", err)
	}
	return out, nil
}

func scanRemark(row rowScanner) (domain.Remark, error) {
	var r domain.Remark
	if err := row.Scan(&r.ID, &r.ReportID, &r.VersionNo, &r.FieldName, &r.Body, &r.Author, &r.CreatedAt, &r.Resolved); err != nil {
		return domain.Remark{}, fmt.Errorf("scan remark: %w", err)
	}
	return r, nil
}
