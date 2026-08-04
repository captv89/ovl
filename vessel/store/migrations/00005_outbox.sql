-- +goose Up

-- outbox holds items pending push to the office (architecture 11.2 step
-- 1: PushOutbox). item_id is a UUID assigned at enqueue time, used by
-- the office for idempotent upsert on retransmission after a dropped
-- link; sequence_no is this vessel's own monotonic counter (see
-- outbox_sequence below). kind discriminates which oneof field of the
-- proto's OutboxItem this row represents. The row only references its
-- source data (reports/report_events) rather than duplicating it:
-- reportVersion items are looked up fresh from `reports` at push time by
-- (report_id, version_no) since submitted report versions are immutable
-- (nothing to go stale between enqueue and push); reportAuditEvent items
-- likewise reference report_events by its own id.
CREATE TABLE outbox (
    item_id         TEXT    NOT NULL PRIMARY KEY,
    sequence_no     INTEGER NOT NULL UNIQUE,
    kind            TEXT    NOT NULL CHECK (kind IN ('reportVersion', 'reportAuditEvent')),
    report_id       TEXT    NOT NULL,
    version_no      INTEGER NOT NULL,
    report_event_id INTEGER, -- set only when kind = 'reportAuditEvent'
    created_at      TEXT    NOT NULL
);

CREATE INDEX idx_outbox_sequence ON outbox (sequence_no);

-- outbox_sequence is a single-row monotonic counter for this vessel's
-- per-item sequence_no (architecture 11.2: "per-vessel monotonic
-- sequence number"). Incremented in the same transaction that enqueues
-- an outbox row rather than derived from MAX(sequence_no) — correct even
-- once acked rows are deleted and the table empties out.
CREATE TABLE outbox_sequence (
    id      INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    next_no INTEGER NOT NULL
);
INSERT INTO outbox_sequence (id, next_no) VALUES (1, 1);

-- +goose Down
DROP TABLE outbox_sequence;
DROP TABLE outbox;
