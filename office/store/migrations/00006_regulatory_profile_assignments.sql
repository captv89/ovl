-- +goose Up

-- regulatory_profile_assignments holds the enabled regulatory profiles
-- (architecture 6.2) at one scope: fleet-wide, one vessel group, or one
-- vessel (design handoff B7's "toggle cards ... assignable fleet-wide,
-- per group or per vessel"). exactly one of vessel_id/group_tag is set,
-- matching scope_type — enforced by the CHECK below rather than three
-- separate tables, since the three scopes share every other column.
-- ON DELETE CASCADE for vessel_id: a vessel-specific assignment has no
-- meaning once its vessel is gone (same reasoning as enrollments).
CREATE TABLE regulatory_profile_assignments (
    scope_type TEXT        NOT NULL CHECK (scope_type IN ('fleet', 'group', 'vessel')),
    vessel_id  UUID        REFERENCES vessels(id) ON DELETE CASCADE,
    group_tag  TEXT,
    profiles   JSONB       NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (scope_type = 'fleet'  AND vessel_id IS NULL     AND group_tag IS NULL) OR
        (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
        (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
    )
);

-- Partial unique indexes enforce "at most one assignment per scope"
-- (one fleet-wide row, one row per vessel, one row per group tag)
-- without a generated/coalesced key column.
CREATE UNIQUE INDEX ux_reg_profile_assignments_fleet ON regulatory_profile_assignments (scope_type) WHERE scope_type = 'fleet';
CREATE UNIQUE INDEX ux_reg_profile_assignments_vessel ON regulatory_profile_assignments (vessel_id) WHERE scope_type = 'vessel';
CREATE UNIQUE INDEX ux_reg_profile_assignments_group ON regulatory_profile_assignments (group_tag) WHERE scope_type = 'group';

-- +goose Down
DROP TABLE regulatory_profile_assignments;
