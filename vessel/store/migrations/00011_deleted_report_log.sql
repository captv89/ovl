-- +goose Up

-- deleted_report_log is a durable, local-only trace of draft/ready
-- reports deleted via DELETE /api/reports/{id} (2026-07-13 manual-test
-- feedback: "there is no way to delete a form in draft status"). Every
-- other mutation in this app is captured in report_events, an append-
-- only trail that survives forever — but a deleted report's own
-- report_events rows are deleted along with it (nothing left to
-- append to), so this table exists purely so *that a report once
-- existed and was removed* isn't lost entirely. Deliberately not a
-- domain.Event/report_events row: it has no report_id left to attach
-- to once the delete completes, and it is never synced to office (a
-- draft/ready report was never enqueued to the outbox in the first
-- place — see vessel/httpapi/reports.go's handleDeleteReport for why
-- this is a purely vessel-local operation).
--
-- No UI reads this table yet — PROJECT.md's backlog carries "admin UI
-- to view the deleted-report log" as a follow-up, not part of this
-- change.
CREATE TABLE deleted_report_log (
    id          INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    report_id   TEXT    NOT NULL,
    schema_name TEXT    NOT NULL,
    event_type  TEXT    NOT NULL,
    event_time  TEXT    NOT NULL, -- RFC3339, UTC; the deleted report's own event time
    state       TEXT    NOT NULL, -- draft or ready — its state at the moment of deletion
    deleted_by  TEXT    NOT NULL,
    deleted_at  TEXT    NOT NULL -- RFC3339, UTC
);

CREATE INDEX idx_deleted_report_log_deleted_at ON deleted_report_log (deleted_at);

-- +goose Down
DROP TABLE deleted_report_log;
