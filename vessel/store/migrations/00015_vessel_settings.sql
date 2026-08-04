-- +goose Up

-- vessel_settings is Master-configurable operational settings that
-- previously lived as fixed Go constants — architecture 11.2's "the
-- vessel syncs on a configurable interval plus a manual 'Sync now'
-- button" was never actually made configurable (vessel/main.go's old
-- syncInterval was a hardcoded 5-minute const, per its own now-removed
-- doc comment). Single row (id fixed at 1), same "one active config"
-- shape as sensor_source/vessel_identity. sync_interval_seconds bounds
-- are enforced in Go (httpapi), not here, matching sensor_source's own
-- validation split.
CREATE TABLE vessel_settings (
    id                    INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    sync_interval_seconds INTEGER NOT NULL DEFAULT 300
);

INSERT INTO vessel_settings (id, sync_interval_seconds) VALUES (1, 300);

-- +goose Down
DROP TABLE vessel_settings;
