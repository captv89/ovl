-- +goose Up

-- vessels holds office-side vessel profile data (architecture 12.4,
-- design handoff B2's Profile tab). Enrollment state (B2's separate
-- Enrollment tab) isn't modeled yet — a later Phase 3 checklist item.
CREATE TABLE vessels (
    id         UUID        NOT NULL PRIMARY KEY,
    imo        TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    type       TEXT        NOT NULL,
    -- groups is JSONB for the same reason users.roles is (see
    -- 00002_users.sql): a small structured value living inside one row,
    -- kept in the one persistence shape this store already uses for
    -- that, rather than a TEXT[] or a join table. Unlike roles, group
    -- tags are free-form (no fixed catalog), so there's nothing to
    -- enumerate even if a join table were otherwise justified.
    groups     JSONB       NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- Group-tag lookups ("vessels in Fleet A") are named in architecture
-- 12.4 ("used for config assignment and dashboard filtering") as a real
-- query pattern, not a hypothetical one — a GIN index on the JSONB
-- column makes "does this vessel have group X" queries index-backed
-- instead of a full table scan.
CREATE INDEX idx_vessels_groups ON vessels USING GIN (groups);

-- +goose Down
DROP TABLE vessels;
