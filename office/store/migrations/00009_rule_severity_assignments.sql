-- +goose Up

-- rule_severity_assignments holds per-scope severity overrides for the
-- overridable plausibility/continuity rules (architecture 10.2; design
-- handoff B7's "table of plausibility rules with severity selector").
-- Identical scope shape to regulatory_profile_assignments/cadence_rules
-- (migrations 00006/00007) — see those files' comments for why scope is
-- three nullable columns plus a CHECK rather than three tables.
CREATE TABLE rule_severity_assignments (
    scope_type TEXT        NOT NULL CHECK (scope_type IN ('fleet', 'group', 'vessel')),
    vessel_id  UUID        REFERENCES vessels(id) ON DELETE CASCADE,
    group_tag  TEXT,
    severities JSONB       NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (scope_type = 'fleet'  AND vessel_id IS NULL     AND group_tag IS NULL) OR
        (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
        (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
    )
);

CREATE UNIQUE INDEX ux_rule_sev_assignments_fleet ON rule_severity_assignments (scope_type) WHERE scope_type = 'fleet';
CREATE UNIQUE INDEX ux_rule_sev_assignments_vessel ON rule_severity_assignments (vessel_id) WHERE scope_type = 'vessel';
CREATE UNIQUE INDEX ux_rule_sev_assignments_group ON rule_severity_assignments (group_tag) WHERE scope_type = 'group';

-- +goose Down
DROP TABLE rule_severity_assignments;
