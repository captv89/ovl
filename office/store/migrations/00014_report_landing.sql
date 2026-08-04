-- +goose Up

-- report_versions is the office's landing table for report versions
-- pushed from vessels (architecture 11.2 step 1: PushOutbox). Keyed by
-- (vessel_id, report_id, version_no) — report_id is a vessel-minted
-- UUIDv7 (pkg/domain.Report's own doc comment), globally unique in
-- practice but scoped by vessel_id anyway since nothing else here says
-- which vessel a report belongs to. Upserted, not append-only:
-- PushOutbox's own idempotent-upsert contract (proto's OutboxItem
-- comment) means a retransmitted item after a dropped link must land as
-- a no-op, and a version's content never changes once submitted
-- (pkg/domain's own immutability rule), so upsert-in-place is safe —
-- there is nothing to lose by overwriting identical data.
CREATE TABLE report_versions (
    vessel_id      UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id      TEXT        NOT NULL,
    version_no     INTEGER     NOT NULL,
    schema_kind    TEXT        NOT NULL,
    schema_version TEXT        NOT NULL DEFAULT '',
    event_type     TEXT        NOT NULL,
    state          TEXT        NOT NULL,
    event_time     TIMESTAMPTZ NOT NULL,
    fields         JSONB       NOT NULL,
    submitted_at   TIMESTAMPTZ,
    received_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (vessel_id, report_id, version_no)
);

CREATE INDEX idx_report_versions_vessel ON report_versions (vessel_id);

-- report_audit_events is the office's append-only mirror of each
-- vessel's audit trail (architecture 14 spans "both vessel and
-- office"). Unlike report_versions, this is genuinely append-only —
-- audit events, once they happen, are never re-sent with different
-- content — but PushOutbox retransmission after a dropped link still
-- needs idempotency, provided by outbox_receipts (00015) rather than a
-- dedup check on this table itself.
CREATE TABLE report_audit_events (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    vessel_id   UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id   TEXT        NOT NULL,
    version_no  INTEGER     NOT NULL,
    event_type  TEXT        NOT NULL,
    actor       TEXT        NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    detail      JSONB       NOT NULL DEFAULT '{}',
    received_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_report_audit_events_report ON report_audit_events (vessel_id, report_id, occurred_at);

-- +goose Down
DROP TABLE report_audit_events;
DROP TABLE report_versions;
