-- +goose Up

-- field_policies holds one row per company-overridden field, per schema
-- version (architecture 6.1 field policy, 6.4 prefill classes; design
-- handoff B6). A field with no row at all is at its default state
-- (optional / none) — only overrides are stored, matching
-- office/fieldpolicy.SchemaFieldPolicy's own "absent = default" shape.
-- There is no schema_versions table yet (design handoff B5's schema
-- upload/download workflow is a later Phase 3 checklist item), so
-- schema_name/schema_version are plain strings, not a foreign key.
CREATE TABLE field_policies (
    schema_name    TEXT        NOT NULL,
    schema_version TEXT        NOT NULL,
    field_name     TEXT        NOT NULL,
    policy_state   TEXT        NOT NULL,
    prefill_class  TEXT        NOT NULL DEFAULT 'none',
    updated_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (schema_name, schema_version, field_name)
);

-- +goose Down
DROP TABLE field_policies;
