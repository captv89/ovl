-- +goose Up

-- user_commands is architecture 9.3/12.4's remote vessel-user-
-- administration queue (2026-07-21): an Admin's action on B2's vessel
-- Users tab lands one row here; the vessel picks it up as a UserCommand
-- in its next PullInbox pull, applies it locally, and reports back via
-- SyncStatus (applied_at) — same seq-cursor pull-stream shape as
-- restore_commands (00029). action is vessel/httpapi's own
-- UserCommandAction string value. temporary_password is plaintext,
-- populated only for create/reset_password — this table is office's own
-- Postgres, already the trust boundary for every other one-time secret
-- this project stores in the clear pre-hash (e.g. issued enrollment
-- codes before redemption), and is cleared once fetched_at is set so it
-- doesn't linger after the vessel has picked it up.
CREATE TABLE user_commands (
    id                 TEXT        NOT NULL PRIMARY KEY,
    vessel_id          UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    action             TEXT        NOT NULL,
    username           TEXT        NOT NULL,
    role               TEXT        NOT NULL DEFAULT '',
    temporary_password TEXT        NOT NULL DEFAULT '',
    can_submit         BOOLEAN     NOT NULL DEFAULT false,
    active             BOOLEAN     NOT NULL DEFAULT false,
    issued_by          TEXT        NOT NULL,
    issued_at          TIMESTAMPTZ NOT NULL,
    fetched_at         TIMESTAMPTZ,
    applied_at         TIMESTAMPTZ,
    seq                BIGSERIAL
);

CREATE INDEX idx_user_commands_vessel_seq ON user_commands (vessel_id, seq);

-- vessel_users is a read-only mirror of each vessel's local account
-- roster, reported on every SyncStatus check-in (architecture 9.3 —
-- vessel accounts are otherwise purely local; office has no other way
-- to know who exists on a given vessel to act on). Never a source of
-- truth — the vessel's own SQLite users table is authoritative; this is
-- purely a cache for B2's Users tab to render against. No password data.
CREATE TABLE vessel_users (
    vessel_id  UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    username   TEXT        NOT NULL,
    role       TEXT        NOT NULL,
    active     BOOLEAN     NOT NULL,
    can_submit BOOLEAN     NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (vessel_id, username)
);

-- +goose Down
DROP TABLE vessel_users;
DROP TABLE user_commands;
