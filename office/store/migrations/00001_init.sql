-- +goose Up

-- pgcrypto provides gen_random_uuid(), which future office-side tables
-- (vessels, config bundles, enrollment records — architecture 12.4, 6.5)
-- will use for primary keys, matching the UUIDv7 identity scheme
-- pkg/domain already uses on the vessel side. No domain tables exist yet
-- (see PROJECT.md's Phase 3 decisions log) — this migration only proves
-- the goose/pgx wiring against a real Postgres instance.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- +goose Down
DROP EXTENSION IF EXISTS pgcrypto;
