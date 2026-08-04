-- +goose Up

-- cursor is the monotonic ordering key PullInbox (architecture 11.2
-- step 3) uses to answer "what's new since the vessel's last pull" —
-- deliberately a new dedicated column, not published_at: published_at
-- has no uniqueness or strict-monotonicity guarantee (two rows could in
-- principle share a timestamp), which a sync cursor cannot tolerate
-- (architecture 11.2's "retransmission after a dropped link is
-- harmless" depends on cursor comparisons being unambiguous). GENERATED
-- ALWAYS AS IDENTITY gives a gap-tolerant, collision-free, insertion-
-- ordered sequence for free.
--
-- schema_versions.cursor backs a global stream (every vessel eventually
-- receives every published version, Phase 4 decision, PROJECT.md
-- 2026-07-11's decisions log) — config_bundles.cursor backs a
-- per-vessel "is the currently-assigned bundle newer than what I have"
-- comparison (architecture 6.5: "the vessel pulls the newest assigned
-- bundle"), resolved via office/configbundle.Resolve, not a list stream
-- like schema versions.
ALTER TABLE schema_versions ADD COLUMN cursor BIGINT GENERATED ALWAYS AS IDENTITY;
ALTER TABLE config_bundles ADD COLUMN cursor BIGINT GENERATED ALWAYS AS IDENTITY;

CREATE INDEX idx_schema_versions_cursor ON schema_versions (cursor);
CREATE INDEX idx_config_bundles_cursor ON config_bundles (cursor);

-- +goose Down
ALTER TABLE config_bundles DROP COLUMN cursor;
ALTER TABLE schema_versions DROP COLUMN cursor;
