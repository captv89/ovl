-- +goose Up

-- users holds local office accounts (architecture 12.2). This is a
-- local stand-in pending real OIDC integration (Keycloak or Authentik,
-- broker-federated to whichever IdP a deployment's own company uses) —
-- see PROJECT.md's Phase 3 decisions log. roles is JSONB (a small array
-- of role-name
-- strings) rather than a native TEXT[] column or a join table: the role
-- set is fixed and small (office/auth.AllRoles, 5 values), and JSONB
-- keeps the same "encode/decode at the app boundary" shape vessel/store
-- already uses for reports.fields/invalidated_rules, avoiding a second
-- persistence pattern for what's conceptually the same kind of data.
CREATE TABLE users (
    id                   UUID        NOT NULL PRIMARY KEY,
    username             TEXT        NOT NULL UNIQUE,
    password_hash        TEXT        NOT NULL,
    roles                JSONB       NOT NULL,
    must_change_password BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE users;
