-- +goose Up

-- invalidation_notices is this vessel's local copy of the office's
-- cascade-revalidation output (architecture 8.3, Phase 5 Slice S2):
-- one row per report version the office found newly broken. applied_at
-- records when this vessel actually drove the local report through
-- Invalidate, distinct from computed_at (when the office computed it) —
-- useful for the audit trail's own timestamp story and for the notices
-- strip design handoff A7 shows.
CREATE TABLE invalidation_notices (
    id           INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    report_id    TEXT    NOT NULL,
    version_no   INTEGER NOT NULL,
    broken_rules TEXT    NOT NULL,
    computed_at  TEXT    NOT NULL,
    applied_at   TEXT    NOT NULL
);

CREATE INDEX idx_invalidation_notices_report ON invalidation_notices (report_id, version_no);

-- +goose Down
DROP TABLE invalidation_notices;
