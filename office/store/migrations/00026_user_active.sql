-- +goose Up

-- active supports Administration's (design handoff B10) "deactivate a
-- user" action — a soft toggle rather than a delete, matching this
-- project's general preference for reversible admin actions
-- (enrollment revoke, not vessel deletion) and preserving the user's
-- row for report/audit-event attribution history (report_audit_events
-- and similar tables reference users by username, not a foreign key,
-- but a deleted account would still read oddly in that history).
ALTER TABLE users ADD COLUMN active BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE users DROP COLUMN active;
