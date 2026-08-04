-- +goose Up

-- chat_messages is the office's copy of every vessel's per-report chat
-- wall (architecture 12.3, design handoff B4/A8): both directions,
-- text-only, size-capped (pkg/domain.MaxChatBodyBytes). id is the
-- UUIDv7 minted at authoring time on whichever side sent the message
-- (vessel or office), a stable identity across the sync boundary. seq
-- is the per-vessel pull cursor (mirrors outbox_receipts/
-- invalidation_notices' own sequence idiom) — a vessel only ever pulls
-- direction='office' rows with seq greater than its ChatCursor.
CREATE TABLE chat_messages (
    id        TEXT        NOT NULL PRIMARY KEY,
    vessel_id UUID        NOT NULL REFERENCES vessels(id) ON DELETE CASCADE,
    report_id TEXT        NOT NULL,
    sender    TEXT        NOT NULL,
    body      TEXT        NOT NULL,
    sent_at   TIMESTAMPTZ NOT NULL,
    direction TEXT        NOT NULL CHECK (direction IN ('vessel', 'office')),
    seq       BIGSERIAL
);

CREATE INDEX idx_chat_messages_vessel_seq ON chat_messages (vessel_id, seq);
CREATE INDEX idx_chat_messages_report ON chat_messages (vessel_id, report_id, sent_at);

-- +goose Down
DROP TABLE chat_messages;
