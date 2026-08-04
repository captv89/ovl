-- +goose Up

-- reports holds one row per report version (architecture 8.1/8.2). A
-- version's report data (fields, event_time, event_type) is mutable
-- while state is draft/ready and immutable once it leaves that pair —
-- see the trigger below — but its lifecycle-tracking columns (state,
-- invalidated_*, submitted_*) keep changing on the same row as the
-- version progresses through submitted/synced/pushed/invalidated. A
-- correction creates a new row with the next version_no instead of
-- mutating a locked one.
CREATE TABLE reports (
    report_id         TEXT    NOT NULL,
    version_no        INTEGER NOT NULL,
    schema_name       TEXT    NOT NULL,
    event_type        TEXT    NOT NULL,
    event_time        TEXT    NOT NULL, -- RFC3339, UTC
    fields            TEXT    NOT NULL CHECK (json_valid(fields)),
    state             TEXT    NOT NULL,
    invalidated_from  TEXT    NOT NULL DEFAULT '',
    invalidated_rules TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(invalidated_rules)),
    created_at        TEXT    NOT NULL,
    created_by        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL,
    submitted_at      TEXT,
    submitted_by      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (report_id, version_no)
);

CREATE INDEX idx_reports_schema_event_time ON reports (schema_name, event_time);

-- Report data is immutable once a version leaves draft/ready (CLAUDE.md:
-- "every report version is immutable once published or submitted").
-- Lifecycle columns (state, invalidated_*, submitted_*, updated_at) are
-- deliberately exempt: they keep evolving on the same row as the
-- version syncs, pushes, gets invalidated, etc.
-- +goose StatementBegin
CREATE TRIGGER reports_locked_after_submit
BEFORE UPDATE ON reports
WHEN OLD.state NOT IN ('draft', 'ready')
  AND (NEW.fields != OLD.fields OR NEW.event_time != OLD.event_time OR NEW.event_type != OLD.event_type)
BEGIN
    SELECT RAISE(ABORT, 'report data is immutable once submitted; create a correction (new version) instead');
END;
-- +goose StatementEnd

-- report_events is the append-only audit trail (architecture 14).
CREATE TABLE report_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id  TEXT    NOT NULL,
    version_no INTEGER NOT NULL,
    type       TEXT    NOT NULL,
    at         TEXT    NOT NULL, -- RFC3339, UTC
    actor      TEXT    NOT NULL DEFAULT '',
    detail     TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(detail))
);

CREATE INDEX idx_report_events_report ON report_events (report_id, at);

-- Nothing is ever deleted from report_events (architecture 14); no
-- delete trigger is needed to enforce that because the application
-- simply never issues a DELETE against this table.

-- +goose Down
DROP TABLE report_events;
DROP TRIGGER reports_locked_after_submit;
DROP TABLE reports;
