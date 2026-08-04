// SPDX-License-Identifier: AGPL-3.0-only

package sync

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"
	"github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1/syncv1connect"

	"github.com/captv89/ovl/pkg/attachmentstore"
	"github.com/captv89/ovl/pkg/syncproto"
)

// fakeAttachmentOffice is a minimal, in-memory stand-in for
// office/syncservice's real resumable-chunk logic (already covered by
// that package's own tests) — this only needs to prove
// vessel/sync.Client.UploadAttachment drives the query-then-send-missing
// loop correctly against a protocol-correct office.
type fakeAttachmentOffice struct {
	syncv1connect.UnimplementedSyncServiceHandler
	mu           sync.Mutex
	received     map[string]map[int32][]byte // content_hash -> chunk_index -> data
	complete     map[string]bool
	uploadCalls  int
	chunkSizeFor map[string]int32
	totalSizeFor map[string]int64
}

func newFakeAttachmentOffice() *fakeAttachmentOffice {
	return &fakeAttachmentOffice{
		received:     make(map[string]map[int32][]byte),
		complete:     make(map[string]bool),
		chunkSizeFor: make(map[string]int32),
		totalSizeFor: make(map[string]int64),
	}
}

func (f *fakeAttachmentOffice) QueryMissingAttachmentChunks(_ context.Context, req *connect.Request[syncv1.QueryMissingAttachmentChunksRequest]) (*connect.Response[syncv1.QueryMissingAttachmentChunksResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	meta := req.Msg.GetAttachment()
	hash := meta.GetContentHash()
	if f.complete[hash] {
		return connect.NewResponse(&syncv1.QueryMissingAttachmentChunksResponse{AlreadyComplete: true}), nil
	}
	f.chunkSizeFor[hash] = meta.GetChunkSize()
	f.totalSizeFor[hash] = meta.GetTotalSize()

	total := (meta.GetTotalSize() + int64(meta.GetChunkSize()) - 1) / int64(meta.GetChunkSize())
	var missing []int32
	for i := int32(0); int64(i) < total; i++ {
		if _, ok := f.received[hash][i]; !ok {
			missing = append(missing, i)
		}
	}
	return connect.NewResponse(&syncv1.QueryMissingAttachmentChunksResponse{MissingChunkIndices: missing}), nil
}

func (f *fakeAttachmentOffice) UploadAttachmentChunk(_ context.Context, req *connect.Request[syncv1.UploadAttachmentChunkRequest]) (*connect.Response[syncv1.UploadAttachmentChunkResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploadCalls++
	hash := req.Msg.GetContentHash()
	if f.received[hash] == nil {
		f.received[hash] = make(map[int32][]byte)
	}
	f.received[hash][req.Msg.GetChunkIndex()] = req.Msg.GetData()

	total := (f.totalSizeFor[hash] + int64(f.chunkSizeFor[hash]) - 1) / int64(f.chunkSizeFor[hash])
	if int64(len(f.received[hash])) >= total {
		f.complete[hash] = true
		return connect.NewResponse(&syncv1.UploadAttachmentChunkResponse{Complete: true}), nil
	}
	return connect.NewResponse(&syncv1.UploadAttachmentChunkResponse{Complete: false}), nil
}

func newFakeAttachmentOfficeServer(t *testing.T, f *fakeAttachmentOffice) *httptest.Server {
	t.Helper()
	path, handler := syncv1connect.NewSyncServiceHandler(f, syncproto.ServerOptions()...)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func assembledContent(f *fakeAttachmentOffice, hash string, chunkCount int32) []byte {
	var buf bytes.Buffer
	for i := range chunkCount {
		buf.Write(f.received[hash][i])
	}
	return buf.Bytes()
}

func TestClient_UploadAttachment_FullUpload(t *testing.T) {
	fake := newFakeAttachmentOffice()
	srv := newFakeAttachmentOfficeServer(t, fake)
	client := NewClient(srv.Client(), srv.URL, "the-vessel-credential")

	local, err := attachmentstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("attachmentstore.New: %v", err)
	}
	content := strings.Repeat("bunker delivery note content ", 50)
	hash, err := local.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	meta := &syncv1.AttachmentMeta{
		ContentHash: hash, ReportId: "report-1", VersionNo: 1, FieldName: "BDN_Scan",
		TotalSize: int64(len(content)), ChunkSize: 200, ContentType: "application/pdf",
	}
	if err := client.UploadAttachment(context.Background(), local, meta); err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}

	chunkCount := (meta.TotalSize + int64(meta.ChunkSize) - 1) / int64(meta.ChunkSize)
	got := assembledContent(fake, hash, int32(chunkCount))
	if string(got) != content {
		t.Error("assembled content on the fake office does not match the original")
	}
	if !fake.complete[hash] {
		t.Error("fake office does not consider the upload complete")
	}
}

func TestClient_UploadAttachment_AlreadyCompleteSendsNoChunks(t *testing.T) {
	fake := newFakeAttachmentOffice()
	srv := newFakeAttachmentOfficeServer(t, fake)
	client := NewClient(srv.Client(), srv.URL, "the-vessel-credential")

	local, err := attachmentstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("attachmentstore.New: %v", err)
	}
	content := "small file"
	hash, err := local.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	fake.complete[hash] = true // simulate the office already having this content (dedup)

	meta := &syncv1.AttachmentMeta{ContentHash: hash, TotalSize: int64(len(content)), ChunkSize: int32(len(content))}
	if err := client.UploadAttachment(context.Background(), local, meta); err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if fake.uploadCalls != 0 {
		t.Errorf("UploadAttachmentChunk was called %d times, want 0 (dedup: already complete)", fake.uploadCalls)
	}
}

func TestClient_UploadAttachment_ResumesOnlyMissingChunks(t *testing.T) {
	fake := newFakeAttachmentOffice()
	srv := newFakeAttachmentOfficeServer(t, fake)
	client := NewClient(srv.Client(), srv.URL, "the-vessel-credential")

	local, err := attachmentstore.New(t.TempDir())
	if err != nil {
		t.Fatalf("attachmentstore.New: %v", err)
	}
	content := strings.Repeat("x", 500)
	hash, err := local.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	meta := &syncv1.AttachmentMeta{ContentHash: hash, TotalSize: int64(len(content)), ChunkSize: 100}

	// Pre-populate the fake office with chunks 0 and 1 already received,
	// simulating a previous partial upload before a dropped link.
	fake.chunkSizeFor[hash] = 100
	fake.totalSizeFor[hash] = int64(len(content))
	fake.received[hash] = map[int32][]byte{
		0: []byte(content[0:100]),
		1: []byte(content[100:200]),
	}

	if err := client.UploadAttachment(context.Background(), local, meta); err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if fake.uploadCalls != 3 { // chunks 2, 3, 4 (5 total, 0 and 1 pre-received)
		t.Errorf("UploadAttachmentChunk was called %d times, want 3 (only the missing chunks)", fake.uploadCalls)
	}
	got := assembledContent(fake, hash, 5)
	if string(got) != content {
		t.Error("assembled content does not match after resuming from a partial upload")
	}
}
