-- +goose Up

-- report_attachments is the durable "which report does this
-- content-addressed blob belong to" record architecture 15's "inline
-- preview on vessel and office" needs — a deliberate reversal of
-- 00017_attachment_uploads.sql's own comment ("there is deliberately no
-- permanent attachments metadata table for completed uploads"), made
-- once Phase 6's vessel-side attachment capture actually landed and
-- exposed the gap: AttachmentMeta's report_id/version_no/field_name/
-- filename were already flowing over QueryMissingAttachmentChunks but
-- had nowhere to be persisted, so office had no way to list or preview
-- what it had received. Inserted from QueryMissingAttachmentChunks
-- (office/syncservice/attachments.go) — the only RPC that carries the
-- full AttachmentMeta context — regardless of whether the content itself
-- turns out already-complete (dedup) or needs uploading, so a resumed
-- sync's repeat call is a harmless upsert, not a duplicate row.
CREATE TABLE report_attachments (
    id           TEXT        NOT NULL PRIMARY KEY,
    vessel_id    UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id    TEXT        NOT NULL,
    version_no   INTEGER     NOT NULL,
    field_name   TEXT        NOT NULL,
    filename     TEXT        NOT NULL,
    content_type TEXT        NOT NULL,
    content_hash TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL,
    UNIQUE (vessel_id, report_id, version_no, content_hash)
);

CREATE INDEX idx_report_attachments_report ON report_attachments (vessel_id, report_id, version_no);

-- +goose Down
DROP TABLE report_attachments;
