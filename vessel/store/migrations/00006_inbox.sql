-- +goose Up

-- schema_versions is the vessel's local cache of schema versions pulled
-- from the office (architecture 11.2 step 3, PullInbox). Byte-verbatim
-- content, matching the office's own "exact JSON... verbatim" choice
-- (office/store/migrations/00008's comment). Insert-only: schema
-- versions are immutable at the source (architecture 5.2), so a given
-- (schema_name, version) never needs updating once received.
CREATE TABLE schema_versions (
    schema_name  TEXT NOT NULL,
    version      TEXT NOT NULL,
    content      BLOB NOT NULL,
    published_at TEXT NOT NULL,
    received_at  TEXT NOT NULL,
    PRIMARY KEY (schema_name, version)
);

-- config_bundles is the vessel's local cache of config bundles pulled
-- from the office. Insert-only, same immutability reasoning as
-- schema_versions above — a bundle_id never changes content once
-- published (architecture 6.5).
CREATE TABLE config_bundles (
    bundle_id    TEXT    NOT NULL PRIMARY KEY,
    version_no   INTEGER NOT NULL,
    content      BLOB    NOT NULL,
    published_at TEXT    NOT NULL,
    received_at  TEXT    NOT NULL
);

-- inbox_cursors is a single row tracking this vessel's pull position on
-- every PullInbox stream (architecture 11.2's SyncCursors) — advanced in
-- the same transaction as the content it's advancing past (see
-- vessel/store.ApplyInboxPull), so a crash mid-apply can never leave a
-- pulled item stored with an un-advanced cursor (re-pull next cycle,
-- harmless — insert-only content is idempotent) or an advanced cursor
-- with missing content (silent data loss).
CREATE TABLE inbox_cursors (
    id                         INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    config_bundle_cursor       INTEGER NOT NULL DEFAULT 0,
    schema_version_cursor      INTEGER NOT NULL DEFAULT 0,
    remark_cursor              INTEGER NOT NULL DEFAULT 0,
    chat_cursor                INTEGER NOT NULL DEFAULT 0,
    invalidation_notice_cursor INTEGER NOT NULL DEFAULT 0,
    restore_command_cursor     INTEGER NOT NULL DEFAULT 0
);
INSERT INTO inbox_cursors (id) VALUES (1);

-- +goose Down
DROP TABLE inbox_cursors;
DROP TABLE config_bundles;
DROP TABLE schema_versions;
