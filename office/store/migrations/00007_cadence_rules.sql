-- +goose Up

-- cadence_rules holds one min-report-interval/max-gap threshold pair
-- (architecture 6.3) per scope, same fleet/group/vessel shape as
-- regulatory_profile_assignments (00006) — see that migration's comment
-- for why scope is modeled as nullable vessel_id/group_tag columns plus
-- a CHECK rather than three tables or a generic scope_key column.
CREATE TABLE cadence_rules (
    scope_type                 TEXT             NOT NULL CHECK (scope_type IN ('fleet', 'group', 'vessel')),
    vessel_id                  UUID             REFERENCES vessels(id) ON DELETE CASCADE,
    group_tag                  TEXT,
    min_report_interval_hours  DOUBLE PRECISION NOT NULL,
    max_gap_hours              DOUBLE PRECISION NOT NULL,
    updated_at                 TIMESTAMPTZ      NOT NULL,
    CHECK (
        (scope_type = 'fleet'  AND vessel_id IS NULL     AND group_tag IS NULL) OR
        (scope_type = 'vessel' AND vessel_id IS NOT NULL AND group_tag IS NULL) OR
        (scope_type = 'group'  AND vessel_id IS NULL     AND group_tag IS NOT NULL)
    )
);

CREATE UNIQUE INDEX ux_cadence_rules_fleet ON cadence_rules (scope_type) WHERE scope_type = 'fleet';
CREATE UNIQUE INDEX ux_cadence_rules_vessel ON cadence_rules (vessel_id) WHERE scope_type = 'vessel';
CREATE UNIQUE INDEX ux_cadence_rules_group ON cadence_rules (group_tag) WHERE scope_type = 'group';

-- +goose Down
DROP TABLE cadence_rules;
