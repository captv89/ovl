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

// InsertInvalidationNotice records one report version newly found to
// violate a continuity rule (architecture 8.3), for a vessel to later
// pull (Slice S4). Always a fresh row, never an update-in-place — see
// the table's own migration comment on why "latest row per report
// version" (GetLatestInvalidationNotice) is the right query, not a
// upsert-and-overwrite.
func (s *Store) InsertInvalidationNotice(ctx context.Context, vesselID, reportID string, versionNo int, brokenRules []string, computedAt time.Time) error {
	rulesJSON, err := json.Marshal(brokenRules)
	if err != nil {
		return fmt.Errorf("marshal broken rules for %s v%d: %w", reportID, versionNo, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO invalidation_notices (vessel_id, report_id, version_no, broken_rules, computed_at)
		VALUES ($1, $2, $3, $4, $5)
	`, vesselID, reportID, versionNo, string(rulesJSON), computedAt)
	if err != nil {
		return fmt.Errorf("insert invalidation notice for %s v%d, vessel %s: %w", reportID, versionNo, vesselID, err)
	}
	return nil
}

// GetLatestInvalidationNotice returns the most recently computed
// invalidation notice for one report version, or ErrNotFound if none
// exists yet — cascade.go's own dedup guard (architecture 8.3: re-
// running cascade with the same broken rules must not spam duplicate
// notices/audit events).
func (s *Store) GetLatestInvalidationNotice(ctx context.Context, vesselID, reportID string, versionNo int) (*domain.InvalidationNotice, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT report_id, version_no, broken_rules, computed_at
		FROM invalidation_notices
		WHERE vessel_id = $1 AND report_id = $2 AND version_no = $3
		ORDER BY seq DESC LIMIT 1
	`, vesselID, reportID, versionNo)
	var (
		n         domain.InvalidationNotice
		rulesJSON string
	)
	err := row.Scan(&n.ReportID, &n.VersionNo, &rulesJSON, &n.ComputedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan invalidation notice: %w", err)
	}
	if err := json.Unmarshal([]byte(rulesJSON), &n.BrokenRules); err != nil {
		return nil, fmt.Errorf("unmarshal broken rules: %w", err)
	}
	return &n, nil
}

// InvalidationNoticeCursorItem pairs an invalidation notice with its
// pull cursor value — mirrors SchemaVersionCursorItem/
// ChatMessageCursorItem's own shape.
type InvalidationNoticeCursorItem struct {
	Notice domain.InvalidationNotice
	Cursor int64
}

// ListInvalidationNoticesSince returns vesselID's invalidation notices
// with seq > sinceCursor, cursor ascending — PullInbox's own vessel-
// scoped stream (Slice S4).
func (s *Store) ListInvalidationNoticesSince(ctx context.Context, vesselID string, sinceCursor int64) ([]InvalidationNoticeCursorItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT report_id, version_no, broken_rules, computed_at, seq
		FROM invalidation_notices WHERE vessel_id = $1 AND seq > $2 ORDER BY seq ASC
	`, vesselID, sinceCursor)
	if err != nil {
		return nil, fmt.Errorf("list invalidation notices since cursor %d, vessel %s: %w", sinceCursor, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []InvalidationNoticeCursorItem
	for rows.Next() {
		var (
			n         domain.InvalidationNotice
			rulesJSON string
			cursor    int64
		)
		if err := rows.Scan(&n.ReportID, &n.VersionNo, &rulesJSON, &n.ComputedAt, &cursor); err != nil {
			return nil, fmt.Errorf("scan invalidation notice: %w", err)
		}
		if err := json.Unmarshal([]byte(rulesJSON), &n.BrokenRules); err != nil {
			return nil, fmt.Errorf("unmarshal broken rules: %w", err)
		}
		out = append(out, InvalidationNoticeCursorItem{Notice: n, Cursor: cursor})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invalidation notices: %w", err)
	}
	return out, nil
}
