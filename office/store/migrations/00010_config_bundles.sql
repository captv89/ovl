-- +goose Up

-- config_bundles is the office-side ledger of published config bundles
-- (architecture 6.5; design handoff B7's bundle composer/history). Each
-- row is a frozen snapshot of every component architecture 6.5 names —
-- schema version references, field policies, regulatory profiles,
-- cadence rules, rule severities, and a default vessel role set — at
-- publish time, decoupled from field_policies/regulatory_profile_
-- assignments/cadence_rules/rule_severity_assignments, which keep
-- changing after a bundle is published. Every component is JSONB, not
-- normalized into its own join table: unlike field_policies (up to 409
-- rows, wants SQL-level search/filter — see that migration's comment),
-- a published bundle is read as a whole document (the history list, or
-- a diff against another whole bundle), never queried per-field.
--
-- No UPDATE path at the store layer: every column is set once at publish
-- time, matching office/configbundle.ConfigBundle having no setters
-- either (architecture 6.5's own "config bundle" is in CLAUDE.md's list
-- of things immutable once published).
CREATE TABLE config_bundles (
    id                  UUID        PRIMARY KEY,
    label               TEXT        NOT NULL DEFAULT '',
    schema_versions     JSONB       NOT NULL DEFAULT '[]',
    field_policies      JSONB       NOT NULL DEFAULT '[]',
    regulatory_profiles JSONB       NOT NULL DEFAULT '[]',
    cadence_rules       JSONB       NOT NULL DEFAULT '[]',
    rule_severities     JSONB       NOT NULL DEFAULT '[]',
    default_role_names  JSONB       NOT NULL DEFAULT '[]',
    published_at        TIMESTAMPTZ NOT NULL,
    published_by        TEXT        NOT NULL
);

-- Bundle history (design handoff B7) is newest-first.
CREATE INDEX ix_config_bundles_published_at ON config_bundles (published_at DESC);

-- +goose Down
DROP TABLE config_bundles;
