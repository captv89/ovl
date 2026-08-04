-- +goose Up

-- Record which config bundle a vessel is actually running on, as it
-- reports on every SyncStatus check-in (codebase audit 2026-07-22 §3.3).
-- bundle_assignments already records the bundle the office *intends* a
-- vessel to run; these columns capture what the vessel reports it has
-- *applied*, so B2 can tell whether a ship is on its assigned bundle or
-- several behind. Empty id / 0 version until the vessel applies any
-- bundle at all. No FK to config_bundles: the office may have pruned or
-- never held the exact row a vessel names, and this is a best-effort
-- last-seen report, not a referential relationship.
ALTER TABLE vessel_sync_status
    ADD COLUMN applied_bundle_id      TEXT   NOT NULL DEFAULT '',
    ADD COLUMN applied_bundle_version BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE vessel_sync_status
    DROP COLUMN applied_bundle_id,
    DROP COLUMN applied_bundle_version;
