-- +goose Up

-- notification_read_state tracks which of the Dashboard/top-bar
-- notification panel's projected notifications (Office UI rework Phase
-- O6, design handoff B1·N) a user has dismissed. There is deliberately
-- no notifications table of its own — a notification is a live
-- projection over existing data (overdue vessels, vessel chat messages,
-- synced report counts), computed at read time by
-- office/httpapi/notifications.go, not a persisted event stream. This
-- table only remembers "user X has seen notification id Y," where id is
-- a deterministic string the projection derives per source row (e.g.
-- "chat:<chat_messages.id>", "overdue:<vessel_id>").
CREATE TABLE notification_read_state (
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_id TEXT        NOT NULL,
    read_at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, notification_id)
);

-- +goose Down
DROP TABLE notification_read_state;
