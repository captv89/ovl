-- +goose Up

-- sync_credential holds this vessel's long-lived office sync bearer
-- token (architecture 11.1), redeemed once from a one-time enrollment
-- code (architecture 11.2's sync handshake, vessel/sync.Redeem). A
-- single row (id fixed at 1) — a vessel syncs with exactly one office.
-- Stored in SQLite rather than bootstrap.json (Phase 4 decision) so it
-- travels automatically with the existing nightly VACUUM INTO DR
-- snapshot and office restore-bundle import — a restored vessel keeps
-- its sync credential without re-enrolling. Unlike the office side,
-- which only ever stores an argon2id hash of a presented token, the
-- vessel stores the token itself: it is the party that must present it
-- on every sync call, the same way any API client holds its own key in
-- the clear.
CREATE TABLE sync_credential (
    id        INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    token     TEXT    NOT NULL,
    issued_at TEXT    NOT NULL
);

-- +goose Down
DROP TABLE sync_credential;
