// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// distinctHash gives each test its own fake content hash, so parallel/
// repeated runs against the same shared Postgres don't collide.
func distinctHash(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	return id.String()
}

func TestStore_GetOrCreateAttachmentUpload(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	hash := distinctHash(t)
	t.Cleanup(func() { _ = st.DeleteAttachmentUpload(ctx, hash) })

	u, err := st.GetOrCreateAttachmentUpload(ctx, AttachmentUpload{ContentHash: hash, TotalSize: 1000, ChunkSize: 100, ContentType: "application/pdf"})
	if err != nil {
		t.Fatalf("GetOrCreateAttachmentUpload: %v", err)
	}
	if u.TotalSize != 1000 || u.ChunkSize != 100 || u.ContentType != "application/pdf" {
		t.Errorf("u = %+v, want TotalSize=1000 ChunkSize=100 ContentType=application/pdf", u)
	}

	// A second call with different declared values must not overwrite
	// the first registration — same upload, re-queried.
	u2, err := st.GetOrCreateAttachmentUpload(ctx, AttachmentUpload{ContentHash: hash, TotalSize: 999, ChunkSize: 50, ContentType: "image/webp"})
	if err != nil {
		t.Fatalf("GetOrCreateAttachmentUpload (second): %v", err)
	}
	if u2.TotalSize != 1000 || u2.ChunkSize != 100 || u2.ContentType != "application/pdf" {
		t.Errorf("u2 = %+v, want the original values preserved", u2)
	}
}

func TestStore_GetAttachmentUpload_NotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetAttachmentUpload(context.Background(), distinctHash(t)); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAttachmentUpload(unregistered) error = %v, want ErrNotFound", err)
	}
}

func TestStore_RecordAndListReceivedChunks(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	hash := distinctHash(t)
	t.Cleanup(func() { _ = st.DeleteAttachmentUpload(ctx, hash) })

	if _, err := st.GetOrCreateAttachmentUpload(ctx, AttachmentUpload{ContentHash: hash, TotalSize: 300, ChunkSize: 100}); err != nil {
		t.Fatalf("GetOrCreateAttachmentUpload: %v", err)
	}

	for _, idx := range []int32{0, 2} {
		if err := st.RecordAttachmentChunkReceived(ctx, hash, idx); err != nil {
			t.Fatalf("RecordAttachmentChunkReceived(%d): %v", idx, err)
		}
	}
	// Re-recording an already-received chunk is a no-op.
	if err := st.RecordAttachmentChunkReceived(ctx, hash, 0); err != nil {
		t.Fatalf("RecordAttachmentChunkReceived(duplicate): %v", err)
	}

	received, err := st.ListReceivedChunkIndices(ctx, hash)
	if err != nil {
		t.Fatalf("ListReceivedChunkIndices: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("len(received) = %d, want 2", len(received))
	}
	seen := map[int32]bool{}
	for _, idx := range received {
		seen[idx] = true
	}
	if !seen[0] || !seen[2] {
		t.Errorf("received = %v, want {0, 2}", received)
	}
}

func TestStore_DeleteAttachmentUpload_CascadesChunks(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	hash := distinctHash(t)

	if _, err := st.GetOrCreateAttachmentUpload(ctx, AttachmentUpload{ContentHash: hash, TotalSize: 100, ChunkSize: 100}); err != nil {
		t.Fatalf("GetOrCreateAttachmentUpload: %v", err)
	}
	if err := st.RecordAttachmentChunkReceived(ctx, hash, 0); err != nil {
		t.Fatalf("RecordAttachmentChunkReceived: %v", err)
	}

	if err := st.DeleteAttachmentUpload(ctx, hash); err != nil {
		t.Fatalf("DeleteAttachmentUpload: %v", err)
	}

	if _, err := st.GetAttachmentUpload(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAttachmentUpload after delete: err = %v, want ErrNotFound", err)
	}
	received, err := st.ListReceivedChunkIndices(ctx, hash)
	if err != nil {
		t.Fatalf("ListReceivedChunkIndices after delete: %v", err)
	}
	if len(received) != 0 {
		t.Errorf("received chunks after delete = %v, want empty (cascade)", received)
	}
}
