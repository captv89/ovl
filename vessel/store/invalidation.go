// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/captv89/ovl/pkg/domain"
)

// InsertInvalidationNotice stores one pulled invalidation notice, unless
// the latest existing notice for the same (report_id, version_no)
// already has identical broken rules — the dedup guard a retransmitted
// pull (same notice, cursor not yet advanced) needs, mirroring the
// office's own cascade dedup check.
func (s *Store) InsertInvalidationNotice(ctx context.Context, n domain.InvalidationNotice, appliedAt time.Time) error {
	latest, err := s.latestInvalidationNotice(ctx, n.ReportID, n.VersionNo)
	if err != nil {
		return err
	}
	if latest != nil && slices.Equal(latest.BrokenRules, n.BrokenRules) {
		return nil
	}
	rulesJSON, err := json.Marshal(n.BrokenRules)
	if err != nil {
		return fmt.Errorf("marshal broken rules for %s v%d: %w", n.ReportID, n.VersionNo, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO invalidation_notices (report_id, version_no, broken_rules, computed_at, applied_at)
		VALUES (?, ?, ?, ?, ?)
	`, n.ReportID, n.VersionNo, string(rulesJSON), n.ComputedAt.UTC().Format(timeLayout), appliedAt.UTC().Format(timeLayout))
	if err != nil {
		return fmt.Errorf("insert invalidation notice for %s v%d: %w", n.ReportID, n.VersionNo, err)
	}
	return nil
}

// latestInvalidationNotice returns the most recently inserted notice for
// (reportID, versionNo), or nil if none exists yet.
func (s *Store) latestInvalidationNotice(ctx context.Context, reportID string, versionNo int) (*domain.InvalidationNotice, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT report_id, version_no, broken_rules, computed_at
		FROM invalidation_notices WHERE report_id = ? AND version_no = ?
		ORDER BY id DESC LIMIT 1
	`, reportID, versionNo)
	n, err := scanInvalidationNotice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ListInvalidationNotices returns every notice ever applied for
// reportID (all versions), oldest first — design handoff A7's notices
// strip.
func (s *Store) ListInvalidationNotices(ctx context.Context, reportID string) ([]domain.InvalidationNotice, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT report_id, version_no, broken_rules, computed_at
		FROM invalidation_notices WHERE report_id = ? ORDER BY id ASC
	`, reportID)
	if err != nil {
		return nil, fmt.Errorf("list invalidation notices for report %s: %w", reportID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []domain.InvalidationNotice
	for rows.Next() {
		n, err := scanInvalidationNotice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invalidation notices for report %s: %w", reportID, err)
	}
	return out, nil
}

func scanInvalidationNotice(row rowScanner) (domain.InvalidationNotice, error) {
	var (
		n          domain.InvalidationNotice
		rulesJSON  string
		computedAt string
	)
	if err := row.Scan(&n.ReportID, &n.VersionNo, &rulesJSON, &computedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.InvalidationNotice{}, err
		}
		return domain.InvalidationNotice{}, fmt.Errorf("scan invalidation notice: %w", err)
	}
	if err := json.Unmarshal([]byte(rulesJSON), &n.BrokenRules); err != nil {
		return domain.InvalidationNotice{}, fmt.Errorf("unmarshal broken rules: %w", err)
	}
	var err error
	if n.ComputedAt, err = time.Parse(timeLayout, computedAt); err != nil {
		return domain.InvalidationNotice{}, fmt.Errorf("parse computed_at: %w", err)
	}
	return n, nil
}
