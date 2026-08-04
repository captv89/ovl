-- +goose Up

-- vessel_credentials holds each vessel's long-lived sync bearer token
-- (architecture 11.1: "Authentication via the per-vessel credential from
-- enrollment, revocable in the office"), minted once when an enrollment
-- code is redeemed (office/enrollment.Redeem + office/synccred.Mint).
-- vessel_id is the primary key: a vessel has at most one active
-- credential, matching enrollments' singular "revoke and re-issue"
-- pattern — re-enrolling supersedes the previous row rather than
-- accumulating history.
--
-- token_lookup_hash is a non-secret SHA-256 index (see
-- office/synccred.Credential's doc comment for why this is safe): the
-- Step 2 auth interceptor looks up a candidate row by this column in
-- O(1), then argon2id-verifies token_hash against it — this index alone
-- never grants access.
CREATE TABLE vessel_credentials (
    vessel_id         UUID        NOT NULL PRIMARY KEY REFERENCES vessels(id) ON DELETE CASCADE,
    token_hash        TEXT        NOT NULL,
    token_lookup_hash TEXT        NOT NULL,
    issued_at         TIMESTAMPTZ NOT NULL,
    revoked_at        TIMESTAMPTZ,
    last_used_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_vessel_credentials_lookup_hash ON vessel_credentials (token_lookup_hash);

-- +goose Down
DROP TABLE vessel_credentials;
