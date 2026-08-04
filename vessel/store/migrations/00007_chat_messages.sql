-- +goose Up

-- chat_messages is this vessel's local copy of its per-report chat wall
-- (architecture 12.3, design handoff A8): both directions, text-only,
-- size-capped (pkg/domain.MaxChatBodyBytes). direction='vessel' rows are
-- authored locally and enqueued to the outbox; direction='office' rows
-- arrive via PullInbox. id is a UUIDv7 minted at creation on whichever
-- side authors the message, so it round-trips the sync boundary as a
-- stable identity (ON CONFLICT DO NOTHING makes re-pull idempotent).
CREATE TABLE chat_messages (
    id        TEXT NOT NULL PRIMARY KEY,
    report_id TEXT NOT NULL,
    sender    TEXT NOT NULL,
    body      TEXT NOT NULL,
    sent_at   TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('vessel', 'office'))
);

CREATE INDEX idx_chat_messages_report ON chat_messages (report_id, sent_at);

-- outbox's kind CHECK constraint and column set need extending for a
-- new 'chatMessage' item kind (a chat message has no version_no or
-- report_events row to reference, unlike the two existing kinds) —
-- SQLite has no ALTER TABLE...ADD CONSTRAINT, so this rebuilds the table
-- rather than just adding a column. outbox is transient (rows deleted
-- once acked, see its own migration comment), so a full rebuild here is
-- safe: there is no meaningful history to lose.
ALTER TABLE outbox RENAME TO outbox_old;

CREATE TABLE outbox (
    item_id         TEXT    NOT NULL PRIMARY KEY,
    sequence_no     INTEGER NOT NULL UNIQUE,
    kind            TEXT    NOT NULL CHECK (kind IN ('reportVersion', 'reportAuditEvent', 'chatMessage')),
    report_id       TEXT    NOT NULL,
    version_no      INTEGER NOT NULL,
    report_event_id INTEGER, -- set only when kind = 'reportAuditEvent'
    chat_message_id TEXT,    -- set only when kind = 'chatMessage'
    created_at      TEXT    NOT NULL
);

INSERT INTO outbox (item_id, sequence_no, kind, report_id, version_no, report_event_id, created_at)
    SELECT item_id, sequence_no, kind, report_id, version_no, report_event_id, created_at FROM outbox_old;

DROP TABLE outbox_old;

CREATE INDEX idx_outbox_sequence ON outbox (sequence_no);

-- +goose Down
ALTER TABLE outbox RENAME TO outbox_new;

CREATE TABLE outbox (
    item_id         TEXT    NOT NULL PRIMARY KEY,
    sequence_no     INTEGER NOT NULL UNIQUE,
    kind            TEXT    NOT NULL CHECK (kind IN ('reportVersion', 'reportAuditEvent')),
    report_id       TEXT    NOT NULL,
    version_no      INTEGER NOT NULL,
    report_event_id INTEGER,
    created_at      TEXT    NOT NULL
);

INSERT INTO outbox (item_id, sequence_no, kind, report_id, version_no, report_event_id, created_at)
    SELECT item_id, sequence_no, kind, report_id, version_no, report_event_id, created_at
    FROM outbox_new WHERE kind != 'chatMessage';

DROP TABLE outbox_new;

CREATE INDEX idx_outbox_sequence ON outbox (sequence_no);

DROP TABLE chat_messages;
