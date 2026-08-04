// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ReportAttachment is one vessel-uploaded Bunker/EDN report attachment,
// as office knows it (architecture 15's "inline preview on vessel and
// office"). The file bytes live in office's own content-addressed
// pkg/attachmentstore, keyed by ContentHash — this row is what makes
// that blob discoverable per report.
type ReportAttachment struct {
	ID          string
	VesselID    string
	ReportID    string
	VersionNo   int
	FieldName   string
	Filename    string
	ContentType string
	ContentHash string
	SizeBytes   int64
	ReceivedAt  time.Time
}

// UpsertReportAttachment records one attachment's report association.
// Called from QueryMissingAttachmentChunks (the only RPC carrying the
// full AttachmentMeta context) regardless of whether the content itself
// is already complete (dedup) or still needs uploading — ON CONFLICT DO
// NOTHING makes a resumed sync's repeat call a harmless no-op rather
// than a duplicate row.
func (s *Store) UpsertReportAttachment(ctx context.Context, a ReportAttachment) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO report_attachments (id, vessel_id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (vessel_id, report_id, version_no, content_hash) DO NOTHING
	`, a.ID, a.VesselID, a.ReportID, a.VersionNo, a.FieldName, a.Filename, a.ContentType, a.ContentHash, a.SizeBytes, a.ReceivedAt)
	if err != nil {
		return fmt.Errorf("upsert report attachment %s for report %s, vessel %s: %w", a.ID, a.ReportID, a.VesselID, err)
	}
	return nil
}

// ListReportAttachments returns every attachment received for one
// report version, oldest first.
func (s *Store) ListReportAttachments(ctx context.Context, vesselID, reportID string, versionNo int) ([]ReportAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vessel_id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, received_at
		FROM report_attachments WHERE vessel_id = $1 AND report_id = $2 AND version_no = $3 ORDER BY received_at ASC
	`, vesselID, reportID, versionNo)
	if err != nil {
		return nil, fmt.Errorf("list report attachments for report %s v%d, vessel %s: %w", reportID, versionNo, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []ReportAttachment
	for rows.Next() {
		a, err := scanReportAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report attachments: %w", err)
	}
	return out, nil
}

// GetReportAttachment returns one attachment by id. Returns ErrNotFound
// if id doesn't exist.
func (s *Store) GetReportAttachment(ctx context.Context, id string) (ReportAttachment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, vessel_id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, received_at
		FROM report_attachments WHERE id = $1
	`, id)
	a, err := scanReportAttachment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ReportAttachment{}, ErrNotFound
	}
	return a, err
}

func scanReportAttachment(row rowScanner) (ReportAttachment, error) {
	var a ReportAttachment
	if err := row.Scan(&a.ID, &a.VesselID, &a.ReportID, &a.VersionNo, &a.FieldName, &a.Filename, &a.ContentType, &a.ContentHash, &a.SizeBytes, &a.ReceivedAt); err != nil {
		return ReportAttachment{}, fmt.Errorf("scan report attachment: %w", err)
	}
	return a, nil
}
