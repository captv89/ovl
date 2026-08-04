-- +goose Up

-- active gates login (design handoff A9's "deactivate" user action): a
-- deactivated account cannot authenticate, and an existing session for
-- it stops resolving on the very next request (see
-- httpapi.authenticatedUser). Defaults to 1 so every existing row
-- (including the Master account created before this column existed)
-- stays active.
ALTER TABLE users ADD COLUMN active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1));

-- +goose Down
ALTER TABLE users DROP COLUMN active;
