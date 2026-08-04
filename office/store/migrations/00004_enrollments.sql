-- +goose Up

-- enrollments holds one row per vessel enrollment (architecture 12.4,
-- design handoff B2's Enrollment tab). vessel_id is the primary key
-- (not a separate surrogate id): there is at most one active enrollment
-- per vessel — re-issuing overwrites the previous code/credentials in
-- place rather than accumulating history, matching "revoke and re-issue
-- credential" (singular) in the design handoff. ON DELETE CASCADE
-- because an enrollment has no meaning once its vessel is gone.
CREATE TABLE enrollments (
    vessel_id                    UUID        NOT NULL PRIMARY KEY REFERENCES vessels(id) ON DELETE CASCADE,
    state                        TEXT        NOT NULL,
    code_hash                    TEXT        NOT NULL DEFAULT '',
    initial_master_username      TEXT        NOT NULL,
    initial_master_password_hash TEXT        NOT NULL,
    issued_at                    TIMESTAMPTZ NOT NULL,
    revoked_at                   TIMESTAMPTZ,
    updated_at                   TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE enrollments;
