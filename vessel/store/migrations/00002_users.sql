-- +goose Up

-- users holds local vessel accounts (architecture 9.3). Roles are a
-- fixed Go enum for now (vessel/auth.Role), not a foreign-keyed table —
-- see vessel/auth's doc comment on why they aren't company-configurable
-- yet.
CREATE TABLE users (
    id                   TEXT    NOT NULL PRIMARY KEY,
    username             TEXT    NOT NULL UNIQUE,
    password_hash        TEXT    NOT NULL,
    role                 TEXT    NOT NULL,
    can_submit           INTEGER NOT NULL DEFAULT 0 CHECK (can_submit IN (0, 1)),
    must_change_password INTEGER NOT NULL DEFAULT 1 CHECK (must_change_password IN (0, 1)),
    created_at           TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL
);

-- +goose Down
DROP TABLE users;
