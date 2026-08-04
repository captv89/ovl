-- +goose Up

-- vessel_sync_status records the last time each vessel successfully
-- completed a SyncStatus call (architecture 11.2 step 4: "vessel reports
-- its cursors and app version. Office records last-seen for the
-- dashboard."). One current-state row per vessel, not a history table —
-- the Phase 7 dashboard needs "when did we last hear from this vessel,"
-- not a log of every sync attempt.
CREATE TABLE vessel_sync_status (
    vessel_id    UUID        NOT NULL PRIMARY KEY REFERENCES vessels(id) ON DELETE CASCADE,
    app_version  TEXT        NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE vessel_sync_status;
