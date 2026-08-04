-- +goose Up

-- attachment_uploads tracks in-progress, not-yet-complete attachment
-- uploads (architecture 15: "chunked and resumable over sync") — one
-- row per content_hash currently being assembled, recording the
-- upload's own declared total_size/chunk_size (from the first
-- QueryMissingAttachmentChunks/UploadAttachmentChunk call to mention
-- this hash) so later chunk arrivals and completeness checks don't need
-- the caller to keep re-supplying them. Deleted once the attachment is
-- verified and promoted into the final content-addressed store — see
-- office/syncservice's attachment handlers. There is deliberately no
-- permanent "attachments" metadata table for completed uploads: the
-- content-addressed filesystem store itself (pkg/attachmentstore) is
-- already the durable record of "do we have this hash," the same way
-- the vessel's own copy has no DB-side attachment table either.
CREATE TABLE attachment_uploads (
    content_hash TEXT        NOT NULL PRIMARY KEY,
    total_size   BIGINT      NOT NULL,
    chunk_size   INTEGER     NOT NULL,
    content_type TEXT        NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ NOT NULL
);

-- attachment_upload_chunks records which chunk indices have been
-- received for a still-in-progress upload — the resumability state
-- QueryMissingAttachmentChunks answers from. Chunk bytes themselves live
-- on the filesystem (a staging directory alongside the final
-- content-addressed store), not here.
CREATE TABLE attachment_upload_chunks (
    content_hash TEXT        NOT NULL REFERENCES attachment_uploads(content_hash) ON DELETE CASCADE,
    chunk_index  INTEGER     NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (content_hash, chunk_index)
);

-- +goose Down
DROP TABLE attachment_upload_chunks;
DROP TABLE attachment_uploads;
