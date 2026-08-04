-- +goose Up

-- sensor_source is this vessel's configured onboard sensor-data REST
-- service (18.07.26 manual-test items 4/9: "vessel pulls from an
-- onboard sensor service" — the vessel-initiated pull architecture, not
-- an ingestion endpoint). A single row (id fixed at 1), same "one active
-- config" shape as dr_identity/sync_credential — reconfiguring from
-- Settings supersedes it in place. api_key is stored in the clear (this
-- table lives in the vessel's own local SQLite, never synced to office
-- or exposed over the wire — same trust boundary vessel_identity already
-- assumes), unlike office's api_key store (hashed, since that one is
-- reachable by external network callers).
CREATE TABLE sensor_source (
    id         INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    base_url   TEXT    NOT NULL,
    api_key    TEXT    NOT NULL,
    enabled    INTEGER NOT NULL,
    updated_at TEXT    NOT NULL
);

-- +goose Down
DROP TABLE sensor_source;
