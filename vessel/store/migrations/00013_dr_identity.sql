-- +goose Up

-- dr_identity holds this vessel's restore-bundle decryption keypair
-- (architecture 12.5, pkg/backupcrypto — an age X25519 keypair),
-- generated locally at enrollment redemption time (vessel/sync.Redeem)
-- and never sent to office past the public half. A single row (id fixed
-- at 1), same "one active keypair" shape as sync_credential — a fresh
-- redemption (including re-enrollment after a revoked/reissued code)
-- supersedes it in place.
CREATE TABLE dr_identity (
    id          INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    public_key  TEXT    NOT NULL,
    private_key TEXT    NOT NULL,
    issued_at   TEXT    NOT NULL
);

-- +goose Down
DROP TABLE dr_identity;
