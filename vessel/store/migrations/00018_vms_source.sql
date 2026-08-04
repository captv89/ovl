-- +goose Up

-- vms_source is this vessel's configured VMS (voyage management system)
-- reference-data REST service — the second half of the sensor+VMS stub
-- expansion (docs/superpowers/specs/2026-08-01-sensor-and-vms-stub-
-- expansion-design.md). Structurally identical to sensor_source
-- (00014_sensor_source.sql) — a single row (id fixed at 1), api_key
-- stored in the clear (same trust boundary: this table lives in the
-- vessel's own local SQLite, never synced to office or exposed over the
-- wire). Deliberately a separate table from sensor_source, not a shared
-- one with a "kind" column — the two sources have independent
-- configure/enable/fail states the Settings UI surfaces separately, and
-- collapsing them would hide that from the officer.
CREATE TABLE vms_source (
    id         INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    base_url   TEXT    NOT NULL,
    api_key    TEXT    NOT NULL,
    enabled    INTEGER NOT NULL,
    updated_at TEXT    NOT NULL
);

-- +goose Down
DROP TABLE vms_source;
