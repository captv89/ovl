-- +goose Up

-- api_key_events is API Access's per-key activity log (design handoff
-- B10, redesign 2026-07-25): "created", "revoked", and every successful
-- use of the data API (GraphQL vs CSV export kept distinct, since a
-- customer's usage pattern is itself useful to an admin deciding
-- whether a key is still needed). Recorded at the same points
-- api_keys.created_at/revoked_at/last_used_at already update — this
-- table is the append-only history those three columns only keep the
-- latest of.
--
-- No FK to api_keys: a key can be hard-deleted (DeleteAPIKey, only once
-- already revoked) once an admin no longer needs its history, and this
-- table's rows are of no use once the key itself is gone — ON DELETE
-- CASCADE keeps that in the database rather than a second query the
-- application would otherwise have to remember to make.
CREATE TABLE api_key_events (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    api_key_id UUID        NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL, -- "created" | "revoked" | "usedGraphQL" | "usedCSV"
    at         TIMESTAMPTZ NOT NULL
);

CREATE INDEX ix_api_key_events_api_key_id ON api_key_events (api_key_id, at DESC);

-- +goose Down
DROP TABLE api_key_events;
