// SPDX-License-Identifier: AGPL-3.0-only

package sync

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	syncv1 "github.com/captv89/ovl/pkg/syncproto/gen/ovl/sync/v1"

	"github.com/captv89/ovl/pkg/attachmentstore"
)

// QueryMissingAttachmentChunks calls the office's
// QueryMissingAttachmentChunks RPC (architecture 15: "chunked and
// resumable over sync").
func (c *Client) QueryMissingAttachmentChunks(ctx context.Context, meta *syncv1.AttachmentMeta) (*syncv1.QueryMissingAttachmentChunksResponse, error) {
	resp, err := c.rpc.QueryMissingAttachmentChunks(ctx, connect.NewRequest(&syncv1.QueryMissingAttachmentChunksRequest{Attachment: meta}))
	if err != nil {
		return nil, fmt.Errorf("sync: call QueryMissingAttachmentChunks: %w", err)
	}
	return resp.Msg, nil
}

// UploadAttachmentChunk calls the office's UploadAttachmentChunk RPC,
// returning whether the office now considers contentHash fully
// assembled and promoted.
func (c *Client) UploadAttachmentChunk(ctx context.Context, contentHash string, chunkIndex int32, data []byte) (complete bool, err error) {
	resp, err := c.rpc.UploadAttachmentChunk(ctx, connect.NewRequest(&syncv1.UploadAttachmentChunkRequest{
		ContentHash: contentHash, ChunkIndex: chunkIndex, Data: data,
	}))
	if err != nil {
		return false, fmt.Errorf("sync: call UploadAttachmentChunk: %w", err)
	}
	return resp.Msg.GetComplete(), nil
}

// UploadAttachment pushes one attachment's remaining chunks to the
// office (architecture 11.2 step 2: "chunked, resumable ... only chunks
// the office confirms missing are sent"): queries which chunks are
// still needed, then sends exactly those, reading each one directly
// from store rather than loading the whole file into memory at once.
// meta.ContentHash must already equal the sha256 of the content stored
// under that hash in store — this function only moves bytes, it doesn't
// compute or verify the hash itself (the office does that on
// promotion, rejecting anything that doesn't match).
//
// Nothing in vessel/httpapi.RunSyncCycle calls this automatically yet:
// report fields have no attachment-reference concept in this codebase
// (no local capture/upload UI exists — see PROJECT.md's Phase 4 Step 5
// notes), so there is nothing for an automatic outbox scan to discover
// and call this for. This method is built and fully tested as a
// standalone, reusable piece — office/syncservice's own tests cover the
// server-side resumability/dedup/corruption-rejection contract this
// relies on — ready for whenever attachment capture lands.
func (c *Client) UploadAttachment(ctx context.Context, store *attachmentstore.Store, meta *syncv1.AttachmentMeta) error {
	queryResp, err := c.QueryMissingAttachmentChunks(ctx, meta)
	if err != nil {
		return err
	}
	if queryResp.GetAlreadyComplete() {
		return nil
	}
	for _, idx := range queryResp.GetMissingChunkIndices() {
		chunk, err := store.Chunk(meta.GetContentHash(), idx, meta.GetChunkSize())
		if err != nil {
			return fmt.Errorf("sync: read chunk %d of %s: %w", idx, meta.GetContentHash(), err)
		}
		if _, err := c.UploadAttachmentChunk(ctx, meta.GetContentHash(), idx, chunk); err != nil {
			return err
		}
	}
	return nil
}
