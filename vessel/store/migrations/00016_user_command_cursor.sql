-- +goose Up

-- user_command_cursor extends inbox_cursors (00006) for architecture
-- 9.3/12.4's remote vessel-user-administration PullInbox stream
-- (2026-07-21) — same "advanced in the same transaction as the content
-- it's advancing past" reasoning as every other cursor on this table,
-- see 00006's own doc comment.
ALTER TABLE inbox_cursors ADD COLUMN user_command_cursor INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE inbox_cursors DROP COLUMN user_command_cursor;
