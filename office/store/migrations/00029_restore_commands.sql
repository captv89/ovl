-- +goose Up

-- restore_commands is architecture 12.5/11.2's DR-bundle-push queue: an
-- Admin's "push to vessel" action on B2's DR tab lands one row here; the
-- vessel picks it up as a RestoreCommand in its next PullInbox pull
-- (proto's restore_commands stream, wired but always empty until this
-- table existed to back it — office/syncservice/pullinbox.go's own
-- comment), fetches the actual encrypted bundle over the new
-- FetchRestoreBundle RPC (fetched_at), applies it locally, and reports
-- back via its next SyncStatus call (applied_at). seq is the per-vessel
-- pull cursor, mirroring remarks/chat_messages/invalidation_notices' own
-- sequence idiom.
CREATE TABLE restore_commands (
    id         TEXT        NOT NULL PRIMARY KEY,
    vessel_id  UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    reason     TEXT        NOT NULL,
    issued_by  TEXT        NOT NULL,
    issued_at  TIMESTAMPTZ NOT NULL,
    fetched_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    seq        BIGSERIAL
);

CREATE INDEX idx_restore_commands_vessel_seq ON restore_commands (vessel_id, seq);

-- +goose Down
DROP TABLE restore_commands;
