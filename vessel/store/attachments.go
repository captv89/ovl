// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Attachment is one Bunker/EDN report attachment's local metadata
// (architecture 15) — the file bytes live in pkg/attachmentstore, keyed
// by ContentHash; this is what a report's Attachments section lists,
// previews, and deletes by. Store-only, not pkg/domain: unlike reports/
// remarks/chat, an attachment has no lifecycle events or cross-side
// domain rules beyond "exists, belongs to a report version, gets synced."
type Attachment struct {
	ID          string
	ReportID    string
	VersionNo   int
	FieldName   string
	Filename    string
	ContentType string
	ContentHash string
	SizeBytes   int64
	UploadedAt  time.Time
	UploadedBy  string
	SyncedAt    *time.Time
}

// InsertAttachment stores one newly-uploaded attachment's metadata.
func (s *Store) InsertAttachment(ctx context.Context, a Attachment) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attachments (id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, uploaded_at, uploaded_by, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, a.ID, a.ReportID, a.VersionNo, a.FieldName, a.Filename, a.ContentType, a.ContentHash, a.SizeBytes, a.UploadedAt.UTC().Format(timeLayout), a.UploadedBy)
	if err != nil {
		return fmt.Errorf("insert attachment %s for report %s: %w", a.ID, a.ReportID, err)
	}
	return nil
}

// ListAttachments returns every attachment for reportID's given version,
// oldest first.
func (s *Store) ListAttachments(ctx context.Context, reportID string, versionNo int) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, uploaded_at, uploaded_by, synced_at
		FROM attachments WHERE report_id = ? AND version_no = ? ORDER BY uploaded_at ASC
	`, reportID, versionNo)
	if err != nil {
		return nil, fmt.Errorf("list attachments for report %s v%d: %w", reportID, versionNo, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachments for report %s v%d: %w", reportID, versionNo, err)
	}
	return out, nil
}

// GetAttachment returns one attachment by id. Returns ErrNotFound if id
// doesn't exist.
func (s *Store) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, uploaded_at, uploaded_by, synced_at
		FROM attachments WHERE id = ?
	`, id)
	a, err := scanAttachment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return a, err
}

// DeleteAttachment removes one attachment's metadata row. The underlying
// blob in pkg/attachmentstore is left in place — it's content-addressed
// and may be deduplicated with other attachments, so nothing else in
// this vessel is safe to delete it on this row's account alone; orphaned
// blob cleanup is not implemented (out of scope, matching the "no
// Delete on attachmentstore.Store" state that package already has).
func (s *Store) DeleteAttachment(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete attachment %s: %w", id, err)
	}
	return nil
}

// ListPendingAttachments returns every attachment not yet confirmed
// synced to the office, oldest first — the batch a sync cycle's
// attachment push phase (vessel/httpapi's RunSyncCycle) sends.
func (s *Store) ListPendingAttachments(ctx context.Context) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, report_id, version_no, field_name, filename, content_type, content_hash, size_bytes, uploaded_at, uploaded_by, synced_at
		FROM attachments WHERE synced_at IS NULL ORDER BY uploaded_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list pending attachments: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending attachments: %w", err)
	}
	return out, nil
}

// MarkAttachmentSynced records that the office has confirmed every chunk
// of this attachment is received.
func (s *Store) MarkAttachmentSynced(ctx context.Context, id string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE attachments SET synced_at = ? WHERE id = ?`, at.UTC().Format(timeLayout), id); err != nil {
		return fmt.Errorf("mark attachment %s synced: %w", id, err)
	}
	return nil
}

func scanAttachment(row rowScanner) (Attachment, error) {
	var (
		a          Attachment
		uploadedAt string
		syncedAt   *string
	)
	if err := row.Scan(&a.ID, &a.ReportID, &a.VersionNo, &a.FieldName, &a.Filename, &a.ContentType, &a.ContentHash, &a.SizeBytes, &uploadedAt, &a.UploadedBy, &syncedAt); err != nil {
		return Attachment{}, fmt.Errorf("scan attachment: %w", err)
	}
	var err error
	if a.UploadedAt, err = time.Parse(timeLayout, uploadedAt); err != nil {
		return Attachment{}, fmt.Errorf("parse attachment uploaded_at: %w", err)
	}
	if syncedAt != nil {
		t, err := time.Parse(timeLayout, *syncedAt)
		if err != nil {
			return Attachment{}, fmt.Errorf("parse attachment synced_at: %w", err)
		}
		a.SyncedAt = &t
	}
	return a, nil
}
