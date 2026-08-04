// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"fmt"
	"time"
)

// UserCommand is one office-issued vessel-account command (architecture
// 9.3/12.4's remote user administration) — see migration
// 00030_user_commands.sql's own doc comment for the full lifecycle
// (queued -> fetched -> applied).
type UserCommand struct {
	ID                string
	VesselID          string
	Action            string
	Username          string
	Role              string
	TemporaryPassword string
	CanSubmit         bool
	Active            bool
	IssuedBy          string
	IssuedAt          time.Time
	FetchedAt         *time.Time
	AppliedAt         *time.Time
}

// QueueUserCommand lands one remote user-management command (B2's
// vessel Users tab). Caller mints ID (uuid.NewV7, same convention
// QueueRestoreCommand's own callers use).
func (s *Store) QueueUserCommand(ctx context.Context, cmd *UserCommand) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_commands (id, vessel_id, action, username, role, temporary_password, can_submit, active, issued_by, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, cmd.ID, cmd.VesselID, cmd.Action, cmd.Username, cmd.Role, cmd.TemporaryPassword, cmd.CanSubmit, cmd.Active, cmd.IssuedBy, cmd.IssuedAt)
	if err != nil {
		return fmt.Errorf("queue user command %s for vessel %s: %w", cmd.ID, cmd.VesselID, err)
	}
	return nil
}

// UserCommandCursorItem pairs a user command with its pull cursor value
// — mirrors RestoreCommandCursorItem's own shape.
type UserCommandCursorItem struct {
	Command UserCommand
	Cursor  int64
}

// ListUserCommandsSince returns vesselID's user commands with
// seq > sinceCursor, cursor ascending, including TemporaryPassword (the
// caller needs it to build the wire message the vessel applies) —
// PullInbox's own vessel-scoped stream. The plaintext survives across
// re-delivery (a lost PullInbox response leaves the vessel's cursor
// un-advanced, so the same command must be pulled again with its
// password intact) and is cleared from storage only once the vessel
// confirms it applied — see MarkUserCommandsApplied.
func (s *Store) ListUserCommandsSince(ctx context.Context, vesselID string, sinceCursor int64) ([]UserCommandCursorItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vessel_id, action, username, role, temporary_password, can_submit, active, issued_by, issued_at, fetched_at, applied_at, seq
		FROM user_commands WHERE vessel_id = $1 AND seq > $2 ORDER BY seq ASC
	`, vesselID, sinceCursor)
	if err != nil {
		return nil, fmt.Errorf("list user commands since cursor %d, vessel %s: %w", sinceCursor, vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []UserCommandCursorItem
	for rows.Next() {
		item, err := scanUserCommandCursorItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user commands: %w", err)
	}
	return out, nil
}

// ListUserCommandsForVessel returns every user command ever queued for
// vesselID, newest first — B2's Users tab status readout, not a pull
// stream. TemporaryPassword is always returned empty here regardless of
// storage state — this is the office-staff-facing view, and a temporary
// password should never round-trip back to the browser after the
// create/reset action's own one-time response already showed it.
func (s *Store) ListUserCommandsForVessel(ctx context.Context, vesselID string) ([]UserCommand, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vessel_id, action, username, role, can_submit, active, issued_by, issued_at, fetched_at, applied_at
		FROM user_commands WHERE vessel_id = $1 ORDER BY issued_at DESC
	`, vesselID)
	if err != nil {
		return nil, fmt.Errorf("list user commands for vessel %s: %w", vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []UserCommand
	for rows.Next() {
		var cmd UserCommand
		if err := rows.Scan(&cmd.ID, &cmd.VesselID, &cmd.Action, &cmd.Username, &cmd.Role, &cmd.CanSubmit, &cmd.Active,
			&cmd.IssuedBy, &cmd.IssuedAt, &cmd.FetchedAt, &cmd.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan user command: %w", err)
		}
		out = append(out, cmd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user commands: %w", err)
	}
	return out, nil
}

// MarkUserCommandFetched records only that the vessel has pulled this
// command (stamps fetched_at). It deliberately does NOT clear the
// plaintext temporary password: PullInbox marks a command fetched while
// still assembling the response, so if that response is lost in transit
// the vessel's cursor never advances and the very same command must be
// re-delivered — with its password still intact. Clearing the plaintext
// is deferred to MarkUserCommandsApplied, driven by the vessel's own
// confirmation that it applied the command (which proves it received the
// password successfully).
func (s *Store) MarkUserCommandFetched(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE user_commands SET fetched_at = $1 WHERE id = $2`, at, id)
	if err != nil {
		return fmt.Errorf("mark user command %s fetched: %w", id, err)
	}
	return nil
}

// MarkUserCommandsApplied records that the vessel confirmed applying
// these commands, as reported via its next SyncStatus call
// (applied_user_command_ids) — mirrors MarkRestoreCommandsApplied. It
// also clears the plaintext temporary password in the same statement:
// the vessel could not have applied a CREATE/RESET_PASSWORD without
// receiving that password, so once applied it has served its purpose and
// has no reason to keep existing in Postgres (see MarkUserCommandFetched
// for why the wipe cannot happen any earlier).
func (s *Store) MarkUserCommandsApplied(ctx context.Context, vesselID string, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_commands SET applied_at = $1, temporary_password = '' WHERE vessel_id = $2 AND id = ANY($3)
	`, at, vesselID, ids)
	if err != nil {
		return fmt.Errorf("mark user commands applied for vessel %s: %w", vesselID, err)
	}
	return nil
}

func scanUserCommandCursorItem(row rowScanner) (UserCommandCursorItem, error) {
	var (
		item   UserCommandCursorItem
		cursor int64
	)
	if err := row.Scan(&item.Command.ID, &item.Command.VesselID, &item.Command.Action, &item.Command.Username, &item.Command.Role,
		&item.Command.TemporaryPassword, &item.Command.CanSubmit, &item.Command.Active, &item.Command.IssuedBy, &item.Command.IssuedAt,
		&item.Command.FetchedAt, &item.Command.AppliedAt, &cursor); err != nil {
		return UserCommandCursorItem{}, fmt.Errorf("scan user command: %w", err)
	}
	item.Cursor = cursor
	return item, nil
}

// VesselUser is a read-only mirror of one vessel-local account
// (migration 00030's own doc comment on why office has this at all).
type VesselUser struct {
	VesselID   string
	Username   string
	Role       string
	Active     bool
	CanSubmit  bool
	UpdatedAt  time.Time
	ReportedAt time.Time
}

// ReplaceVesselUsers overwrites vesselID's entire roster mirror with
// users — the vessel reports its full current roster on every
// SyncStatus call (not a cursor-delta stream), so a full replace is both
// simpler and self-healing against a missed update, unlike an
// insert-only stream that could accumulate stale rows for since-deleted
// local accounts forever.
func (s *Store) ReplaceVesselUsers(ctx context.Context, vesselID string, users []VesselUser, reportedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace vessel users: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	if _, err := tx.ExecContext(ctx, `DELETE FROM vessel_users WHERE vessel_id = $1`, vesselID); err != nil {
		return fmt.Errorf("clear vessel users for vessel %s: %w", vesselID, err)
	}
	for _, u := range users {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO vessel_users (vessel_id, username, role, active, can_submit, updated_at, reported_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, vesselID, u.Username, u.Role, u.Active, u.CanSubmit, u.UpdatedAt, reportedAt); err != nil {
			return fmt.Errorf("insert vessel user %s for vessel %s: %w", u.Username, vesselID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace vessel users: %w", err)
	}
	return nil
}

// ListVesselUsers returns vesselID's mirrored roster, username
// ascending. Returns ErrNotFound-free empty slice if the vessel has
// never reported (e.g. not yet enrolled/synced), not an error — a real,
// reachable state B2's Users tab must render sensibly.
func (s *Store) ListVesselUsers(ctx context.Context, vesselID string) ([]VesselUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vessel_id, username, role, active, can_submit, updated_at, reported_at
		FROM vessel_users WHERE vessel_id = $1 ORDER BY username ASC
	`, vesselID)
	if err != nil {
		return nil, fmt.Errorf("list vessel users for vessel %s: %w", vesselID, err)
	}
	defer func() { _ = rows.Close() }() // rows.Err(), checked below, covers the meaningful failure

	var out []VesselUser
	for rows.Next() {
		var u VesselUser
		if err := rows.Scan(&u.VesselID, &u.Username, &u.Role, &u.Active, &u.CanSubmit, &u.UpdatedAt, &u.ReportedAt); err != nil {
			return nil, fmt.Errorf("scan vessel user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vessel users: %w", err)
	}
	return out, nil
}
