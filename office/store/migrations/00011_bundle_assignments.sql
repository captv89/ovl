-- +goose Up

-- bundle_assignments records which published config bundle currently
-- applies to one vessel or vessel group (design handoff B7's "assignment
-- picker (vessels or groups)"). Unlike regulatory_profile_assignments/
-- cadence_rules/rule_severity_assignments, there is deliberately no
-- 'fleet' scope_type here — architecture 6.5 says bundles are "assigned
-- per vessel or per vessel group" only, matching
-- office/configbundle.NewBundleAssignment's own rejection of
-- compliance.ScopeFleet.
--
-- One active assignment per scope (partial unique indexes below), same
-- upsert-in-place shape as the other scope-based config tables: "the
-- vessel pulls the newest assigned bundle" (architecture 6.5) describes
-- a current pointer, not an accumulating history — bundle history lives
-- in config_bundles itself (immutable rows, newest-first), not here.
--
-- Per-vessel applied-state ("pulled or pending next sync," design
-- handoff B7) is NOT tracked by this table — that needs a real sync
-- cursor concept, which doesn't exist anywhere in office/store yet
-- (Phase 4).
CREATE TABLE bundle_assignments (
    scope_type  TEXT        NOT NULL CHECK (scope_type IN ('group', 'vessel')),
    vessel_id   UUID        REFERENCES vessels(id) ON DELETE CASCADE,
    group_tag   TEXT,
    bundle_id   UUID        NOT NULL REFERENCES config_bundles(id),
    assigned_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
        (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
    )
);

CREATE UNIQUE INDEX ux_bundle_assignments_vessel ON bundle_assignments (vessel_id) WHERE scope_type = 'vessel';
CREATE UNIQUE INDEX ux_bundle_assignments_group ON bundle_assignments (group_tag) WHERE scope_type = 'group';

-- +goose Down
DROP TABLE bundle_assignments;
