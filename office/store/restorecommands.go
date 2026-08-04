// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RestoreCommand is one office-issued "push this restore bundle to the
// vessel" instruction (architecture 12.5/11.2) — see migration
// 00029_restore_commands.sql's own doc comment for the full lifecycle
// (queued -> fetched -> applied).
type RestoreCommand struct {
	ID        string
	VesselID  string
	Reason    string
	IssuedBy  string
	IssuedAt  time.Time
	FetchedAt *time.Time
	AppliedAt *time.Time
}

// QueueRestoreCommand lands one restore command (design handoff B2's DR
// tab "push to vessel" action). Caller mints ID (uuid.NewV7, same
// convention office/httpapi's remark-set handler already uses).
func (s *Store) QueueRestoreCommand(ctx context.Context, cmd *RestoreCommand) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO restore_commands (id, vessel_id, reason, issued_by, issued_at)
		VALUES ($1, $2, $3, $4, $5)
	`, cmd.ID, cmd.VesselID, cmd.Reason, cmd.IssuedBy, cmd.IssuedAt)
	if err != nil {
		return fmt.Errorf("queue restore command %s for vessel %s: %w", cmd.ID, cmd.VesselID, err)
	}
	return nil
}

// RestoreCommandCursorItem pairs a restore command with its pull cursor
// value — mirrors RemarkCursorItem/ChatMessageCursorItem's own shape.
type RestoreCommandCursorItem struct {
	Command RestoreCommand
	Cursor  int64
}

// ListRestoreCommandsSince returns vesselID's restore commands with
// seq > sinceCursor, cursor ascending — PullInbox's own vessel-scoped
// stream.
func (s *Store) ListRestoreCommandsSince(ctx context.Context, vesselID string, sinceCursor int64) ([]RestoreCommandCursorItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vessel_id, reason, issued_by, issued_at, fetched_at, applied_at, seq
		FROM restore_commands WHERE vessel_id = $1 AND seq > $2 ORDER BY seq ASC
	`, vesselID, sinceCursor)
	if err != nil {
		return nil, fmt.Errorf("list restore commands since cursor %d, vessel %s: %w", sinceCursor, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []RestoreCommandCursorItem
	for rows.Next() {
		item, err := scanRestoreCommandCursorItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restore commands: %w", err)
	}
	return out, nil
}

// ListRestoreCommandsForVessel returns every restore command ever queued
// for vesselID, newest first — B2's DR tab status readout (queued/
// fetched/applied), not a pull stream.
func (s *Store) ListRestoreCommandsForVessel(ctx context.Context, vesselID string) ([]RestoreCommand, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vessel_id, reason, issued_by, issued_at, fetched_at, applied_at
		FROM restore_commands WHERE vessel_id = $1 ORDER BY issued_at DESC
	`, vesselID)
	if err != nil {
		return nil, fmt.Errorf("list restore commands for vessel %s: %w", vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []RestoreCommand
	for rows.Next() {
		var cmd RestoreCommand
		if err := rows.Scan(&cmd.ID, &cmd.VesselID, &cmd.Reason, &cmd.IssuedBy, &cmd.IssuedAt, &cmd.FetchedAt, &cmd.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan restore command: %w", err)
		}
		out = append(out, cmd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restore commands: %w", err)
	}
	return out, nil
}

// GetRestoreCommand looks up one command by id, scoped to vesselID so
// FetchRestoreBundle can't be used to pull another vessel's command by
// guessing an id — the calling vessel's own credential already
// determines vesselID (office/syncservice.VesselIDFromContext), this
// just enforces it matches the row. Returns ErrNotFound if absent or if
// it belongs to a different vessel.
func (s *Store) GetRestoreCommand(ctx context.Context, id, vesselID string) (*RestoreCommand, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, vessel_id, reason, issued_by, issued_at, fetched_at, applied_at
		FROM restore_commands WHERE id = $1 AND vessel_id = $2
	`, id, vesselID)
	var cmd RestoreCommand
	err := row.Scan(&cmd.ID, &cmd.VesselID, &cmd.Reason, &cmd.IssuedBy, &cmd.IssuedAt, &cmd.FetchedAt, &cmd.AppliedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get restore command %s: %w", id, err)
	}
	return &cmd, nil
}

// MarkRestoreCommandFetched records that the vessel has downloaded the
// encrypted bundle for this command (FetchRestoreBundle RPC) — not yet
// proof it applied successfully, just that the bytes were served.
func (s *Store) MarkRestoreCommandFetched(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE restore_commands SET fetched_at = $1 WHERE id = $2`, at, id)
	if err != nil {
		return fmt.Errorf("mark restore command %s fetched: %w", id, err)
	}
	return nil
}

// MarkRestoreCommandsApplied records that the vessel confirmed applying
// these commands, as reported via its next SyncStatus call
// (applied_restore_command_ids) — this is the "confirming it's pushed"
// signal B2's DR tab surfaces. Silently ignores ids that don't belong to
// vesselID (defensive: a vessel can only ever legitimately report ids it
// received from its own PullInbox pulls, so a mismatch here would be a
// bug elsewhere, not a case worth failing the whole SyncStatus call
// over).
func (s *Store) MarkRestoreCommandsApplied(ctx context.Context, vesselID string, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE restore_commands SET applied_at = $1 WHERE vessel_id = $2 AND id = ANY($3)
	`, at, vesselID, ids)
	if err != nil {
		return fmt.Errorf("mark restore commands applied for vessel %s: %w", vesselID, err)
	}
	return nil
}

func scanRestoreCommandCursorItem(row rowScanner) (RestoreCommandCursorItem, error) {
	var (
		item   RestoreCommandCursorItem
		cursor int64
	)
	if err := row.Scan(&item.Command.ID, &item.Command.VesselID, &item.Command.Reason, &item.Command.IssuedBy,
		&item.Command.IssuedAt, &item.Command.FetchedAt, &item.Command.AppliedAt, &cursor); err != nil {
		return RestoreCommandCursorItem{}, fmt.Errorf("scan restore command: %w", err)
	}
	item.Cursor = cursor
	return item, nil
}
