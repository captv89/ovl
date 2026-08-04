-- +goose Up

-- outbox_receipts is the office's per-vessel idempotency ledger for
-- PushOutbox (architecture 11.2 step 1: "retransmission after a dropped
-- link is harmless — idempotent upsert by UUID + sequence"). A
-- duplicate item_id for the same vessel is a no-op: PushOutbox checks
-- this table before applying a report_versions/report_audit_events
-- write, and every item — whether newly applied or a repeat — still
-- returns ItemAck.accepted = true.
CREATE TABLE outbox_receipts (
    vessel_id   UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    item_id     TEXT        NOT NULL,
    sequence_no BIGINT      NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (vessel_id, item_id)
);

CREATE INDEX idx_outbox_receipts_vessel_sequence ON outbox_receipts (vessel_id, sequence_no);

-- +goose Down
DROP TABLE outbox_receipts;
