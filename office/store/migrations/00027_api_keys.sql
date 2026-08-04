-- +goose Up

-- api_keys holds bearer credentials external customers use to query the
-- data API (architecture 13.1) — GraphQL/CSV, gated separately from the
-- session-cookie auth office staff use. Modeled directly on
-- vessel_credentials (00012_vessel_credentials.sql): token_lookup_hash
-- is a non-secret SHA-256 index for the O(1) candidate lookup a bearer
-- token checked on every request needs (see office/synccred.Credential's
-- doc comment for why that's safe here despite this project otherwise
-- hashing every secret with argon2id), token_hash is the actual argon2id
-- verification. Unlike vessel_credentials (one per vessel, upsert in
-- place), a customer can hold many keys and a vessel isn't the natural
-- owner here, so this has its own surrogate id instead of a foreign-key
-- primary key.
--
-- group_id is a free-form vessel-group tag string, not a foreign key —
-- groups aren't a normalized table anywhere in this schema (vessels.groups
-- is JSONB, matched by containment; see office/store.ReportFilter.GroupID's
-- own doc comment for the same naming convention despite holding a tag,
-- not a surrogate id). NULL means unscoped: the key can see every vessel.
--
-- created_by is the issuing admin's username, stored as plain text like
-- report_audit_events.actor rather than a foreign key to users(id) —
-- this is audit attribution, not a relationship the schema needs to
-- enforce, and users are never hard-deleted in this project anyway
-- (only deactivated), so a FK here would add coupling without ever
-- actually protecting anything.
CREATE TABLE api_keys (
    id                UUID        NOT NULL PRIMARY KEY,
    label             TEXT        NOT NULL,
    token_hash        TEXT        NOT NULL,
    token_lookup_hash TEXT        NOT NULL,
    group_id          TEXT,
    created_by        TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL,
    revoked_at        TIMESTAMPTZ,
    last_used_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_api_keys_lookup_hash ON api_keys (token_lookup_hash);

-- +goose Down
DROP TABLE api_keys;
