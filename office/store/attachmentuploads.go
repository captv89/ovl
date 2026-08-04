// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AttachmentUpload is the declared shape (total size, chunk size,
// content type) of an in-progress attachment upload — architecture 15's
// "chunked and resumable over sync."
type AttachmentUpload struct {
	ContentHash string
	TotalSize   int64
	ChunkSize   int32
	ContentType string
	StartedAt   time.Time
}

// GetOrCreateAttachmentUpload returns the existing upload row for
// meta.ContentHash if one is already in progress (a vessel re-querying
// after a dropped link should see the same total_size/chunk_size it
// declared the first time, not silently different values from a second
// caller), or creates one from meta if this is the first time this hash
// has been seen.
func (s *Store) GetOrCreateAttachmentUpload(ctx context.Context, meta AttachmentUpload) (*AttachmentUpload, error) {
	if meta.StartedAt.IsZero() {
		meta.StartedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attachment_uploads (content_hash, total_size, chunk_size, content_type, started_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (content_hash) DO NOTHING
	`, meta.ContentHash, meta.TotalSize, meta.ChunkSize, meta.ContentType, meta.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("get or create attachment upload %s: %w", meta.ContentHash, err)
	}
	return s.GetAttachmentUpload(ctx, meta.ContentHash)
}

// GetAttachmentUpload returns the in-progress upload row for
// contentHash. Returns ErrNotFound if none exists (the attachment is
// either already complete/promoted, or QueryMissingAttachmentChunks was
// never called for it first).
func (s *Store) GetAttachmentUpload(ctx context.Context, contentHash string) (*AttachmentUpload, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT content_hash, total_size, chunk_size, content_type, started_at
		FROM attachment_uploads WHERE content_hash = $1
	`, contentHash)
	var u AttachmentUpload
	err := row.Scan(&u.ContentHash, &u.TotalSize, &u.ChunkSize, &u.ContentType, &u.StartedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan attachment upload: %w", err)
	}
	return &u, nil
}

// RecordAttachmentChunkReceived marks chunkIndex as received for
// contentHash — idempotent: re-recording an already-received chunk
// (e.g. an ack that never reached the vessel before a retry) is a
// no-op, not an error.
func (s *Store) RecordAttachmentChunkReceived(ctx context.Context, contentHash string, chunkIndex int32) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attachment_upload_chunks (content_hash, chunk_index, received_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (content_hash, chunk_index) DO NOTHING
	`, contentHash, chunkIndex, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("record attachment chunk %d for %s: %w", chunkIndex, contentHash, err)
	}
	return nil
}

// ListReceivedChunkIndices returns every chunk index received so far for
// contentHash — QueryMissingAttachmentChunks' resumability answer.
func (s *Store) ListReceivedChunkIndices(ctx context.Context, contentHash string) ([]int32, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT chunk_index FROM attachment_upload_chunks WHERE content_hash = $1
	`, contentHash)
	if err != nil {
		return nil, fmt.Errorf("list received chunks for %s: %w", contentHash, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []int32
	for rows.Next() {
		var idx int32
		if err := rows.Scan(&idx); err != nil {
			return nil, fmt.Errorf("scan received chunk index: %w", err)
		}
		out = append(out, idx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate received chunks for %s: %w", contentHash, err)
	}
	return out, nil
}

// DeleteAttachmentUpload removes the in-progress upload row for
// contentHash (cascading to its chunk-received rows) — called once the
// attachment is verified and promoted into the final content-addressed
// store, or when a corrupted assembly is rejected and staging needs a
// clean slate for a retry.
func (s *Store) DeleteAttachmentUpload(ctx context.Context, contentHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM attachment_uploads WHERE content_hash = $1`, contentHash); err != nil {
		return fmt.Errorf("delete attachment upload %s: %w", contentHash, err)
	}
	return nil
}
