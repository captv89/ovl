-- +goose Up

-- schema_versions is the office-side registry of published schema
-- versions (architecture 5.2: "published schema versions are immutable...
-- a new version is always a new record"; design handoff B5's version
-- list). There is no UPDATE path at the store layer for this table on
-- purpose — every column is set once at publish time and never revisited,
-- matching office/schemaversions.SchemaVersion having no setters either.
--
-- content is BYTEA, not JSONB: Postgres's JSONB type re-serializes
-- documents (whitespace/key-order are not preserved), which would break
-- the "carrying the exact JSON... verbatim" choice PROJECT.md's
-- 2026-07-04 decisions log already made for syncproto's
-- ConfigBundle.content_json/SchemaVersion.schema_json — the same
-- verbatim-bytes reasoning applies here.
--
-- UNIQUE(schema_name, version) is the real-world identity: architecture
-- 5.2 names schema versions by exactly this pair ("3.13",
-- "3.13-company-r2"), and reports permanently reference it.
CREATE TABLE schema_versions (
    id           UUID        PRIMARY KEY,
    schema_name  TEXT        NOT NULL,
    version      TEXT        NOT NULL,
    source       TEXT        NOT NULL,
    content      BYTEA       NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    published_by TEXT        NOT NULL,
    UNIQUE (schema_name, version)
);

-- Serves both ListSchemaVersions (all versions of one schema, newest
-- first) and LatestSchemaVersion (same query, LIMIT 1).
CREATE INDEX ix_schema_versions_schema_name ON schema_versions (schema_name, published_at DESC);

-- +goose Down
DROP TABLE schema_versions;
