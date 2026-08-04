-- +goose Up

-- attachments is this vessel's local metadata for Bunker/EDN report
-- attachments (architecture 15): the file bytes themselves live in the
-- content-addressed pkg/attachmentstore (BaseDir = <dataDir>/attachments,
-- see vessel/httpapi/backup.go's attachmentsDir), keyed by content_hash;
-- this table is what a report's Attachments section actually lists,
-- previews, and deletes by. synced_at is NULL until this vessel's sync
-- cycle has confirmed the office has every chunk (vessel/httpapi/
-- attachments.go's push phase) — a nullable column here rather than the
-- shared `outbox` table, since attachment transfer is chunked binary RPC
-- (QueryMissingAttachmentChunks/UploadAttachmentChunk), not the JSON-
-- envelope PushOutbox every other outbox kind rides.
CREATE TABLE attachments (
    id           TEXT    NOT NULL PRIMARY KEY,
    report_id    TEXT    NOT NULL,
    version_no   INTEGER NOT NULL,
    field_name   TEXT    NOT NULL,
    filename     TEXT    NOT NULL,
    content_type TEXT    NOT NULL,
    content_hash TEXT    NOT NULL,
    size_bytes   INTEGER NOT NULL,
    uploaded_at  TEXT    NOT NULL,
    uploaded_by  TEXT    NOT NULL,
    synced_at    TEXT
);

CREATE INDEX idx_attachments_report ON attachments (report_id, version_no);
CREATE INDEX idx_attachments_pending_sync ON attachments (synced_at) WHERE synced_at IS NULL;

-- +goose Down
DROP TABLE attachments;
