-- +goose Up

-- Allows bundle_assignments a 'fleet' scope_type — 2026-07-15 test
-- feedback #3 corrected architecture 6.5 (originally "assigned per
-- vessel or per vessel group" only) to match cadence_rules/
-- regulatory_profile_assignments/rule_severity_assignments' own
-- fleet/group/vessel shape (00007/00006/00009). See office/configbundle/
-- assignment.go's own doc comment for the precedence this now resolves
-- with (vessel > group > fleet).
ALTER TABLE bundle_assignments DROP CONSTRAINT bundle_assignments_scope_type_check;
ALTER TABLE bundle_assignments ADD CONSTRAINT bundle_assignments_scope_type_check
    CHECK (scope_type IN ('fleet', 'group', 'vessel'));

ALTER TABLE bundle_assignments DROP CONSTRAINT bundle_assignments_check;
ALTER TABLE bundle_assignments ADD CONSTRAINT bundle_assignments_check CHECK (
    (scope_type = 'fleet'  AND vessel_id IS NULL     AND group_tag IS NULL) OR
    (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
    (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
);

CREATE UNIQUE INDEX ux_bundle_assignments_fleet ON bundle_assignments (scope_type) WHERE scope_type = 'fleet';

-- +goose Down
DROP INDEX ux_bundle_assignments_fleet;
ALTER TABLE bundle_assignments DROP CONSTRAINT bundle_assignments_check;
ALTER TABLE bundle_assignments ADD CONSTRAINT bundle_assignments_check CHECK (
    (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
    (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
);
ALTER TABLE bundle_assignments DROP CONSTRAINT bundle_assignments_scope_type_check;
ALTER TABLE bundle_assignments ADD CONSTRAINT bundle_assignments_scope_type_check
    CHECK (scope_type IN ('group', 'vessel'));
