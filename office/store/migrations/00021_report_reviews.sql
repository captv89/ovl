-- +goose Up

-- report_reviews is design handoff B3's "mark reviewed" bulk action
-- (Phase 5 open question 3, resolved default): a lightweight, office-
-- only triage annotation. Deliberately not a lifecycle state — it never
-- touches report_versions.state or anything the vessel ever sees, only
-- office's own "have I looked at this" bookkeeping, distinct from
-- remarked/invalidated which are real cross-side states. Upserted, not
-- append-only: re-marking an already-reviewed report just updates who/
-- when, there is no value in keeping prior review history for v1.
CREATE TABLE report_reviews (
    vessel_id   UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id   TEXT        NOT NULL,
    reviewed_by TEXT        NOT NULL,
    reviewed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (vessel_id, report_id)
);

-- +goose Down
DROP TABLE report_reviews;
