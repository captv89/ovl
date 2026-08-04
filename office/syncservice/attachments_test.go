// SPDX-License-Identifier: AGPL-3.0-only

package syncservice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
)

// splitIntoChunks divides content into chunkSize-sized pieces (the last
// one possibly shorter), matching how a real vessel would chunk a file.
func splitIntoChunks(content []byte, chunkSize int) [][]byte {
	var chunks [][]byte
	for i := 0; i < len(content); i += chunkSize {
		end := min(i+chunkSize, len(content))
		chunks = append(chunks, content[i:end])
	}
	return chunks
}

func hashOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestAttachmentUpload_FullCycle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 85)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	content := bytes.Repeat([]byte("a bunker delivery note scan, pretend PDF bytes. "), 100)
	chunks := splitIntoChunks(content, 200)
	hash := hashOf(content)

	meta := &syncv1.AttachmentMeta{
		ContentHash: hash, ReportId: "report-1", VersionNo: 1, FieldName: "BDN_Scan",
		TotalSize: int64(len(content)), ChunkSize: 200, ContentType: "application/pdf",
	}

	queryResp, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta}))
	if err != nil {
		t.Fatalf("QueryMissingAttachmentChunks: %v", err)
	}
	if queryResp.Msg.GetAlreadyComplete() {
		t.Fatal("AlreadyComplete = true before any chunks were uploaded")
	}
	if len(queryResp.Msg.GetMissingChunkIndices()) != len(chunks) {
		t.Fatalf("MissingChunkIndices = %v, want %d entries", queryResp.Msg.GetMissingChunkIndices(), len(chunks))
	}

	var lastResp *connect.Response[syncv1.UploadAttachmentChunkResponse]
	for i, chunk := range chunks {
		lastResp, err = client.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{
			ContentHash: hash, ChunkIndex: int32(i), Data: chunk,
		}))
		if err != nil {
			t.Fatalf("UploadAttachmentChunk(%d): %v", i, err)
		}
	}
	if !lastResp.Msg.GetComplete() {
		t.Error("last UploadAttachmentChunk response: Complete = false, want true")
	}

	// A subsequent query reports already_complete.
	queryResp2, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta}))
	if err != nil {
		t.Fatalf("QueryMissingAttachmentChunks (after complete): %v", err)
	}
	if !queryResp2.Msg.GetAlreadyComplete() {
		t.Error("AlreadyComplete = false after all chunks uploaded, want true")
	}
	if len(queryResp2.Msg.GetMissingChunkIndices()) != 0 {
		t.Errorf("MissingChunkIndices after complete = %v, want empty", queryResp2.Msg.GetMissingChunkIndices())
	}
}

// TestAttachmentUpload_RecordsReportAttachment covers Phase 6's
// office-side reversal of 00017_attachment_uploads.sql's own former
// "no permanent attachments table" decision: QueryMissingAttachmentChunks
// must persist the report association (report_id/version_no/field_name/
// filename) the first time it sees an AttachmentMeta, before any chunk
// has even been uploaded, and a resumed call for the same content must
// not duplicate the row.
func TestAttachmentUpload_RecordsReportAttachment(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 87)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	content := bytes.Repeat([]byte("bunker delivery note bytes "), 20)
	hash := hashOf(content)
	meta := &syncv1.AttachmentMeta{
		ContentHash: hash, ReportId: "report-attach-1", VersionNo: 1, FieldName: "Attachments",
		Filename: "bdn.pdf", TotalSize: int64(len(content)), ChunkSize: 200, ContentType: "application/pdf",
	}

	if _, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta})); err != nil {
		t.Fatalf("QueryMissingAttachmentChunks: %v", err)
	}
	// A resumed call (e.g. a dropped link retried) must not duplicate the row.
	if _, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta})); err != nil {
		t.Fatalf("QueryMissingAttachmentChunks (resumed): %v", err)
	}

	attachments, err := st.ListReportAttachments(ctx, v.ID, "report-attach-1", 1)
	if err != nil {
		t.Fatalf("ListReportAttachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("len(attachments) = %d, want 1 (no duplicate from the resumed call)", len(attachments))
	}
	got := attachments[0]
	if got.Filename != "bdn.pdf" || got.ContentType != "application/pdf" || got.ContentHash != hash || got.SizeBytes != int64(len(content)) {
		t.Errorf("attachment = %+v, want matching filename/contentType/hash/size", got)
	}
}

func TestAttachmentUpload_ResumesFromMissingChunksOnly(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 86)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	content := bytes.Repeat([]byte("resumable upload test content "), 50)
	chunks := splitIntoChunks(content, 150)
	hash := hashOf(content)
	meta := &syncv1.AttachmentMeta{ContentHash: hash, TotalSize: int64(len(content)), ChunkSize: 150}

	if _, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta})); err != nil {
		t.Fatalf("QueryMissingAttachmentChunks: %v", err)
	}

	// Simulate a dropped link after sending only the first half.
	half := len(chunks) / 2
	for i := range half {
		if _, err := client.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{
			ContentHash: hash, ChunkIndex: int32(i), Data: chunks[i],
		})); err != nil {
			t.Fatalf("UploadAttachmentChunk(%d): %v", i, err)
		}
	}

	// Resume: query again, expect exactly the remaining indices, nothing
	// already sent.
	resumeResp, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta}))
	if err != nil {
		t.Fatalf("QueryMissingAttachmentChunks (resume): %v", err)
	}
	missing := resumeResp.Msg.GetMissingChunkIndices()
	if len(missing) != len(chunks)-half {
		t.Fatalf("missing after resume = %v, want %d entries", missing, len(chunks)-half)
	}
	seen := map[int32]bool{}
	for _, idx := range missing {
		seen[idx] = true
		if idx < int32(half) {
			t.Errorf("resume reported chunk %d as missing, but it was already uploaded", idx)
		}
	}

	// Send only the missing ones — the resumability contract.
	var lastResp *connect.Response[syncv1.UploadAttachmentChunkResponse]
	for _, idx := range missing {
		lastResp, err = client.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{
			ContentHash: hash, ChunkIndex: idx, Data: chunks[idx],
		}))
		if err != nil {
			t.Fatalf("UploadAttachmentChunk(%d): %v", idx, err)
		}
	}
	if !lastResp.Msg.GetComplete() {
		t.Error("Complete = false after uploading all remaining chunks")
	}
}

func TestAttachmentUpload_UploadWithoutQueryFirstIsRejected(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 87)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	content := []byte("small file, one chunk")
	hash := hashOf(content)

	_, err := client.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{ContentHash: hash, ChunkIndex: 0, Data: content}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("UploadAttachmentChunk without a prior QueryMissingAttachmentChunks: error = %v, want CodeFailedPrecondition", err)
	}
}

func TestAttachmentUpload_QueryAfterCompleteReportsZeroChunksNeeded(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 88)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	content := []byte("dedup test content")
	hash := hashOf(content)
	meta := &syncv1.AttachmentMeta{ContentHash: hash, TotalSize: int64(len(content)), ChunkSize: int32(len(content))}

	if _, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta})); err != nil {
		t.Fatalf("QueryMissingAttachmentChunks: %v", err)
	}
	if _, err := client.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{ContentHash: hash, ChunkIndex: 0, Data: content})); err != nil {
		t.Fatalf("UploadAttachmentChunk: %v", err)
	}

	// Re-running the whole thing (e.g. a second vessel with the exact
	// same file, or the same vessel re-syncing) must send zero chunks —
	// the dedup contract.
	resp, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta}))
	if err != nil {
		t.Fatalf("QueryMissingAttachmentChunks (dedup check): %v", err)
	}
	if !resp.Msg.GetAlreadyComplete() {
		t.Error("AlreadyComplete = false for content already fully stored, want true")
	}
	if len(resp.Msg.GetMissingChunkIndices()) != 0 {
		t.Errorf("MissingChunkIndices = %v, want empty (dedup: nothing to send)", resp.Msg.GetMissingChunkIndices())
	}
}

func TestAttachmentUpload_CorruptedChunkFailsVerificationAndIsNotPromoted(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 89)
	token := mintTestCredential(t, st, v)
	srv := newTestServer(t, st)
	client := newTestClient(srv, token)

	realContent := []byte("the real content that should be uploaded")
	hash := hashOf(realContent)
	meta := &syncv1.AttachmentMeta{ContentHash: hash, TotalSize: int64(len(realContent)), ChunkSize: int32(len(realContent))}

	if _, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta})); err != nil {
		t.Fatalf("QueryMissingAttachmentChunks: %v", err)
	}

	// Upload different (corrupted) bytes than what the hash promises.
	corrupted := []byte("this is NOT the content the hash was computed from!!")
	_, err := client.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{
		ContentHash: hash, ChunkIndex: 0, Data: corrupted,
	}))
	if err == nil {
		t.Fatal("UploadAttachmentChunk with corrupted content = nil error, want a verification failure")
	}
	if connect.CodeOf(err) != connect.CodeDataLoss {
		t.Errorf("error code = %v, want CodeDataLoss", connect.CodeOf(err))
	}

	// Not promoted: a fresh query must not report already_complete.
	resp, err := client.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta}))
	if err != nil {
		t.Fatalf("QueryMissingAttachmentChunks (after corruption): %v", err)
	}
	if resp.Msg.GetAlreadyComplete() {
		t.Error("AlreadyComplete = true after a corrupted upload, want false (corrupted content must not be promoted)")
	}
	if len(resp.Msg.GetMissingChunkIndices()) != 1 {
		t.Errorf("MissingChunkIndices = %v, want exactly 1 (retry from scratch)", resp.Msg.GetMissingChunkIndices())
	}
}

func TestAttachmentUpload_MissingCredential(t *testing.T) {
	st := openTestStore(t)
	srv := newTestServer(t, st)
	client := newTestClient(srv, "")

	_, err := client.QueryMissingAttachmentChunks(context.Background(), connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{
		Attachment: &syncv1.AttachmentMeta{ContentHash: "some-hash"},
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("QueryMissingAttachmentChunks (no credential) error = %v, want CodeUnauthenticated", err)
	}
}
