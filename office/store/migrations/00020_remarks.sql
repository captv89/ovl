-- +goose Up

-- remarks is one field-level reviewer comment against a specific report
-- version (architecture 12.3, design handoff B4's Remark mode). One B4
-- "send remark set" action produces several rows sharing remark_set_id
-- — an office-only grouping column, since the wire Remark message is
-- per-field and carries no set id; the vessel regroups received remarks
-- structurally by (created_at, author) instead (see the vessel table's
-- own migration comment). seq is the per-vessel pull cursor, mirroring
-- chat_messages/invalidation_notices' own sequence idiom.
CREATE TABLE remarks (
    id            TEXT        NOT NULL PRIMARY KEY,
    remark_set_id TEXT        NOT NULL,
    vessel_id     UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id     TEXT        NOT NULL,
    version_no    INTEGER     NOT NULL,
    field_name    TEXT        NOT NULL,
    body          TEXT        NOT NULL,
    author        TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    resolved      BOOLEAN     NOT NULL DEFAULT false,
    resolved_at   TIMESTAMPTZ,
    seq           BIGSERIAL
);

CREATE INDEX idx_remarks_vessel_seq ON remarks (vessel_id, seq);
CREATE INDEX idx_remarks_report ON remarks (vessel_id, report_id);

-- +goose Down
DROP TABLE remarks;
