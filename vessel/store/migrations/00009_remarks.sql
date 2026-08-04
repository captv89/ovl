-- +goose Up

-- remarks is this vessel's local copy of office-authored field remarks
-- (architecture 12.3, design handoff A7's Remarks tab). No
-- vessel_id/seq (single-vessel scope) and no remark_set_id (the office-
-- only grouping column the wire Remark message doesn't carry) — a
-- vessel that wants to show "which remarks arrived together" regroups
-- structurally by (created_at, author) instead, since remarks pulled in
-- the same PullInbox response naturally share both.
CREATE TABLE remarks (
    id         TEXT    NOT NULL PRIMARY KEY,
    report_id  TEXT    NOT NULL,
    version_no INTEGER NOT NULL,
    field_name TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    author     TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    resolved   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_remarks_report ON remarks (report_id);

-- +goose Down
DROP TABLE remarks;
