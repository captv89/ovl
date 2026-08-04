-- +goose Up

-- origin records which side authored an audit event: 'vessel' for
-- events relayed from a vessel's outbox (pkg/domain's lifecycle state
-- machine only ever runs on the vessel, so every vessel-pushed event is
-- vessel-authored by construction), 'office' for events office computes
-- or authors itself directly — cascade-triggered invalidations
-- (office/syncservice/cascade.go) and reviewer-authored remark sets
-- (office/httpapi/remarks.go). Phase 5 T6.3's Audit trail "side" filter.
ALTER TABLE report_audit_events ADD COLUMN origin TEXT NOT NULL DEFAULT 'vessel'
    CHECK (origin IN ('vessel', 'office'));

-- +goose Down
ALTER TABLE report_audit_events DROP COLUMN origin;
