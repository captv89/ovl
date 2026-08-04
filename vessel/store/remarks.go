// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// InsertRemark stores one pulled remark. ON CONFLICT DO NOTHING makes
// this idempotent on id — a re-pull of an already-applied remark
// (cursor not yet advanced) is a harmless no-op.
func (s *Store) InsertRemark(ctx context.Context, r domain.Remark) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO remarks (id, report_id, version_no, field_name, body, author, created_at, resolved)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`, r.ID, r.ReportID, r.VersionNo, r.FieldName, r.Body, r.Author, r.CreatedAt.UTC().Format(timeLayout), r.Resolved)
	if err != nil {
		return fmt.Errorf("insert remark %s for report %s: %w", r.ID, r.ReportID, err)
	}
	return nil
}

// ListRemarks returns every remark ever received for reportID (all
// versions), chronological — design handoff A7's Remarks tab.
func (s *Store) ListRemarks(ctx context.Context, reportID string) ([]domain.Remark, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, version_no, field_name, body, author, created_at, resolved
		FROM remarks WHERE report_id = ? ORDER BY created_at ASC, id ASC
	`, reportID)
	if err != nil {
		return nil, fmt.Errorf("list remarks for report %s: %w", reportID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []domain.Remark
	for rows.Next() {
		var (
			r         domain.Remark
			createdAt string
		)
		if err := rows.Scan(&r.ID, &r.ReportID, &r.VersionNo, &r.FieldName, &r.Body, &r.Author, &createdAt, &r.Resolved); err != nil {
			return nil, fmt.Errorf("scan remark: %w", err)
		}
		if r.CreatedAt, err = time.Parse(timeLayout, createdAt); err != nil {
			return nil, fmt.Errorf("parse remark created_at: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remarks for report %s: %w", reportID, err)
	}
	return out, nil
}
