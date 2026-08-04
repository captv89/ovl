-- +goose Up

-- Replaces field_policies (migration 00005: one global row set per
-- schema version, no scope at all) with field_policy_assignments: one
-- row per (scope, schema, version) holding that scope's whole policy/
-- prefill map as JSONB, the same shape regulatory_profile_assignments/
-- rule_severity_assignments already use (migrations 00006/00009) for
-- their identical fleet/group/vessel scope. Field policy was the one
-- config-bundle component (architecture 6.5) that never got this
-- scoping — this closes that gap, so a company can give e.g. DP
-- vessels a policy that shows DP-specific fields while a container or
-- bulk vessel group's policy keeps them hidden, instead of the whole
-- fleet being stuck on one global field policy.
CREATE TABLE field_policy_assignments (
    scope_type     TEXT        NOT NULL CHECK (scope_type IN ('fleet', 'group', 'vessel')),
    vessel_id      UUID        REFERENCES vessels(id) ON DELETE CASCADE,
    group_tag      TEXT,
    schema_name    TEXT        NOT NULL,
    schema_version TEXT        NOT NULL,
    policy         JSONB       NOT NULL DEFAULT '{}',
    prefill        JSONB       NOT NULL DEFAULT '{}',
    updated_at     TIMESTAMPTZ NOT NULL,
    CHECK (
        (scope_type = 'fleet'  AND vessel_id IS NULL     AND group_tag IS NULL) OR
        (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
        (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
    )
);

CREATE UNIQUE INDEX ux_field_policy_assignments_fleet ON field_policy_assignments (schema_name, schema_version) WHERE scope_type = 'fleet';
CREATE UNIQUE INDEX ux_field_policy_assignments_vessel ON field_policy_assignments (schema_name, schema_version, vessel_id) WHERE scope_type = 'vessel';
CREATE UNIQUE INDEX ux_field_policy_assignments_group ON field_policy_assignments (schema_name, schema_version, group_tag) WHERE scope_type = 'group';

-- Carry forward whatever was already saved under the old global
-- (schema, version) shape as a fleet-wide assignment — the only scope
-- that existed before this migration.
INSERT INTO field_policy_assignments (scope_type, schema_name, schema_version, policy, prefill, updated_at)
SELECT
    'fleet',
    schema_name,
    schema_version,
    jsonb_object_agg(field_name, policy_state),
    jsonb_object_agg(field_name, prefill_class) FILTER (WHERE prefill_class <> 'none'),
    max(updated_at)
FROM field_policies
GROUP BY schema_name, schema_version;

-- A schema/version group with only prefill_class = 'none' rows produces
-- prefill = NULL from the FILTER above (jsonb_object_agg with no
-- matching rows); normalize that to an empty object so it round-trips
-- the same way a brand-new fleet assignment does.
UPDATE field_policy_assignments SET prefill = '{}' WHERE prefill IS NULL;

DROP TABLE field_policies;

-- +goose Down
CREATE TABLE field_policies (
    schema_name    TEXT        NOT NULL,
    schema_version TEXT        NOT NULL,
    field_name     TEXT        NOT NULL,
    policy_state   TEXT        NOT NULL,
    prefill_class  TEXT        NOT NULL DEFAULT 'none',
    updated_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (schema_name, schema_version, field_name)
);

INSERT INTO field_policies (schema_name, schema_version, field_name, policy_state, prefill_class, updated_at)
SELECT
    a.schema_name,
    a.schema_version,
    kv.key,
    kv.value #>> '{}',
    COALESCE(a.prefill ->> kv.key, 'none'),
    a.updated_at
FROM field_policy_assignments a, jsonb_each(a.policy) kv
WHERE a.scope_type = 'fleet';

DROP TABLE field_policy_assignments;
