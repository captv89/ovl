-- +goose Up

-- invalidation_notices is office cascade revalidation's (architecture
-- 8.3) pull queue for vessels: each row is one report version newly
-- found to violate a continuity rule, computed office-side after a
-- report version lands (Phase 5's syncservice/cascade.go). seq is the
-- per-vessel pull cursor (mirrors outbox_receipts' sequence_no idiom) —
-- Slice S4 pulls rows with seq > InvalidationNoticeCursor. Also doubles
-- as the office's own "was this already reported with the same broken
-- rules" dedup check (see cascade.go's own doc comment) via the latest
-- row per (vessel_id, report_id, version_no), so cascade re-running
-- after an unrelated push doesn't spam duplicate notices/audit events.
CREATE TABLE invalidation_notices (
    seq          BIGSERIAL   PRIMARY KEY,
    vessel_id    UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id    TEXT        NOT NULL,
    version_no   INTEGER     NOT NULL,
    broken_rules JSONB       NOT NULL,
    computed_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_invalidation_notices_vessel_seq ON invalidation_notices (vessel_id, seq);

-- +goose Down
DROP TABLE invalidation_notices;
