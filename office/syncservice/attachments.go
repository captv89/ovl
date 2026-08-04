// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/office/store"
)

// validContentHash matches attachmentstore's own hex.EncodeToString(sha256(...))
// output exactly (64 lowercase hex chars) — every content_hash this
// package builds a filesystem path from (chunkStagingPath, cleanupStaging,
// assembleAndPromote's assembledPath) comes straight off the wire from
// req.Msg/meta, a vessel-controlled value this server has no other reason
// to trust. Without this check a crafted "../../../etc/x"-shaped hash
// could escape stagingDir (CWE-22) — os.RemoveAll(cleanupStaging) makes
// that a delete primitive, not just a read one.
var validContentHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// QueryMissingAttachmentChunks reports whether an attachment is already
// stored, or which chunk indices are still missing (architecture 15:
// "chunked and resumable over sync"). Registers the upload's declared
// size/chunk-size on first call for a given content hash — subsequent
// calls (e.g. a resumed sync after a dropped link) reuse that
// registration rather than trusting whatever the caller re-declares.
func (s *Server) QueryMissingAttachmentChunks(ctx context.Context, req *connect.Request[syncv1.QueryMissingAttachmentChunksRequest]) (*connect.Response[syncv1.QueryMissingAttachmentChunksResponse], error) {
	vesselID, ok := VesselIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errUnauthenticatedContext)
	}
	meta := req.Msg.GetAttachment()
	hash := meta.GetContentHash()
	if !validContentHash.MatchString(hash) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("content_hash must be a 64-character lowercase hex sha256 digest"))
	}

	// This is the only RPC carrying the full AttachmentMeta context
	// (report_id/version_no/field_name/filename) — UploadAttachmentChunk
	// only ever carries a bare content_hash — so the report-association
	// record is upserted here regardless of whether the content itself
	// turns out already-complete (dedup) or still needs uploading.
	if err := s.upsertReportAttachment(ctx, vesselID, meta); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Content addressing is intentionally global, not per-vessel: identical
	// bytes from any vessel dedup to the same stored object (architecture
	// 15: "content-addressed by sha256, deduplicated").
	if s.attachments.Has(hash) {
		return connect.NewResponse(&syncv1.QueryMissingAttachmentChunksResponse{AlreadyComplete: true}), nil
	}

	upload, err := s.st.GetOrCreateAttachmentUpload(ctx, store.AttachmentUpload{
		ContentHash: hash, TotalSize: meta.GetTotalSize(), ChunkSize: meta.GetChunkSize(), ContentType: meta.GetContentType(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	missing, err := s.missingChunkIndices(ctx, upload)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&syncv1.QueryMissingAttachmentChunksResponse{MissingChunkIndices: missing}), nil
}

// upsertReportAttachment records vesselID's report-attachment
// association from meta — see report_attachments' own migration comment
// on why this table exists and why it's populated from this call site.
func (s *Server) upsertReportAttachment(ctx context.Context, vesselID string, meta *syncv1.AttachmentMeta) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate report attachment id: %w", err)
	}
	return s.st.UpsertReportAttachment(ctx, store.ReportAttachment{
		ID: id.String(), VesselID: vesselID, ReportID: meta.GetReportId(), VersionNo: int(meta.GetVersionNo()),
		FieldName: meta.GetFieldName(), Filename: meta.GetFilename(), ContentType: meta.GetContentType(),
		ContentHash: meta.GetContentHash(), SizeBytes: meta.GetTotalSize(), ReceivedAt: time.Now().UTC(),
	})
}

// UploadAttachmentChunk stages one chunk and, once every chunk for its
// content hash has arrived, assembles them, verifies the sha256 against
// the declared hash, and promotes the result into the final
// content-addressed store. A hash mismatch (corruption) is rejected —
// the assembled file is discarded and staging is cleared so the next
// QueryMissingAttachmentChunks call starts the upload over from chunk 0,
// rather than being told to resume from a set of chunks that produced
// bad output once already.
func (s *Server) UploadAttachmentChunk(ctx context.Context, req *connect.Request[syncv1.UploadAttachmentChunkRequest]) (*connect.Response[syncv1.UploadAttachmentChunkResponse], error) {
	if _, ok := VesselIDFromContext(ctx); !ok {
		return nil, connect.NewError(connect.CodeInternal, errUnauthenticatedContext)
	}
	hash := req.Msg.GetContentHash()
	idx := req.Msg.GetChunkIndex()
	if !validContentHash.MatchString(hash) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("content_hash must be a 64-character lowercase hex sha256 digest"))
	}
	if idx < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("chunk_index must be non-negative"))
	}

	if s.attachments.Has(hash) {
		return connect.NewResponse(&syncv1.UploadAttachmentChunkResponse{Complete: true}), nil // already promoted; nothing to do
	}

	upload, err := s.st.GetAttachmentUpload(ctx, hash)
	if errors.Is(err, store.ErrNotFound) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("call QueryMissingAttachmentChunks before uploading chunks"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	chunkPath := s.chunkStagingPath(hash, idx)
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0o750); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := os.WriteFile(chunkPath, req.Msg.GetData(), 0o600); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.st.RecordAttachmentChunkReceived(ctx, hash, idx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	missing, err := s.missingChunkIndices(ctx, upload)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(missing) > 0 {
		return connect.NewResponse(&syncv1.UploadAttachmentChunkResponse{Complete: false}), nil
	}

	if err := s.assembleAndPromote(ctx, upload); err != nil {
		return nil, connect.NewError(connect.CodeDataLoss, err)
	}
	return connect.NewResponse(&syncv1.UploadAttachmentChunkResponse{Complete: true}), nil
}

// missingChunkIndices computes 0..totalChunks-1 minus whatever's already
// recorded as received for upload.ContentHash.
func (s *Server) missingChunkIndices(ctx context.Context, upload *store.AttachmentUpload) ([]int32, error) {
	received, err := s.st.ListReceivedChunkIndices(ctx, upload.ContentHash)
	if err != nil {
		return nil, fmt.Errorf("list received chunks: %w", err)
	}
	receivedSet := make(map[int32]bool, len(received))
	for _, idx := range received {
		receivedSet[idx] = true
	}
	total := chunkCount(upload.TotalSize, upload.ChunkSize)
	var missing []int32
	for i := range total {
		if !receivedSet[i] {
			missing = append(missing, i)
		}
	}
	return missing, nil
}

// chunkCount returns how many chunks of chunkSize it takes to cover
// totalSize bytes (ceiling division).
func chunkCount(totalSize int64, chunkSize int32) int32 {
	if chunkSize <= 0 {
		return 0
	}
	n := (totalSize + int64(chunkSize) - 1) / int64(chunkSize)
	return int32(n) // #nosec G115 -- an attachment's chunk count at any realistic chunkSize is nowhere near 2^31
}

// assembleAndPromote concatenates every staged chunk for upload (in
// index order) into one temp file, hands it to attachmentstore.PutFile
// for hash verification and promotion, and cleans up staging either way
// — on success because the content now lives in the final store, on
// failure so a retry doesn't resume from chunks that already produced
// corrupt output once.
func (s *Server) assembleAndPromote(ctx context.Context, upload *store.AttachmentUpload) error {
	total := chunkCount(upload.TotalSize, upload.ChunkSize)
	assembledPath := filepath.Join(s.stagingDir, upload.ContentHash+".assembling")
	if err := assembleChunkFile(assembledPath, func(i int32) string { return s.chunkStagingPath(upload.ContentHash, i) }, total); err != nil {
		_ = s.cleanupStaging(upload.ContentHash)
		return fmt.Errorf("assemble chunks for %s: %w", upload.ContentHash, err)
	}

	if _, err := s.attachments.PutFile(assembledPath, upload.ContentHash); err != nil {
		_ = os.Remove(assembledPath)
		_ = s.cleanupStaging(upload.ContentHash)
		_ = s.st.DeleteAttachmentUpload(ctx, upload.ContentHash)
		return fmt.Errorf("promote %s: %w", upload.ContentHash, err)
	}

	_ = s.cleanupStaging(upload.ContentHash)
	if err := s.st.DeleteAttachmentUpload(ctx, upload.ContentHash); err != nil {
		return fmt.Errorf("delete upload record for %s: %w", upload.ContentHash, err)
	}
	return nil
}

// assembleChunkFile writes chunks 0..total-1 (via chunkPath) into destPath
// in order. destPath/chunkPath are always built from a content_hash
// validContentHash has already checked (see the two RPC handlers above),
// never raw external input.
func assembleChunkFile(destPath string, chunkPath func(int32) string, total int32) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return err
	}
	out, err := os.Create(destPath) // #nosec G304 -- see doc comment
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	for i := range total {
		data, err := os.ReadFile(chunkPath(i)) // #nosec G304 -- see doc comment

		if err != nil {
			return fmt.Errorf("read chunk %d: %w", i, err)
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("write chunk %d: %w", i, err)
		}
	}
	return out.Close()
}

// chunkStagingPath is stagingDir/<content_hash>/<chunk_index> for one
// chunk still being assembled.
func (s *Server) chunkStagingPath(contentHash string, chunkIndex int32) string {
	return filepath.Join(s.stagingDir, contentHash, strconv.Itoa(int(chunkIndex)))
}

// cleanupStaging removes every staged chunk (and the assembling temp
// file, if any) for contentHash.
func (s *Server) cleanupStaging(contentHash string) error {
	_ = os.Remove(filepath.Join(s.stagingDir, contentHash+".assembling"))
	if err := os.RemoveAll(filepath.Join(s.stagingDir, contentHash)); err != nil {
		return fmt.Errorf("clean up staging for %s: %w", contentHash, err)
	}
	return nil
}
