-- +goose Up

-- vessel_identity is this vessel's own copy of the office-side vessel
-- profile (name/IMO), captured once at enrollment redemption (2026-07-14
-- manual-test feedback: "after vessel enrollment why cant the vessel get
-- the basic metadata from the office" / "an enrolled vessel ... should
-- not be asked to enter the IMO ... again"). Singleton row, same
-- id=1-on-conflict pattern as sync_credential — a vessel has exactly one
-- identity. Never populated for the offline/deferred-enrollment path
-- (architecture 9.2's "skip" option), since there is no office to ask.
CREATE TABLE vessel_identity (
    id     INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    name   TEXT    NOT NULL,
    imo    TEXT    NOT NULL
);

-- +goose Down
DROP TABLE vessel_identity;
