// SPDX-License-Identifier: AGPL-3.0-only

package store

import (
	"context"
	"testing"
	"time"
)

func TestStore_QueueAndListUserCommands(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 99)

	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	cmd := &UserCommand{
		ID: "user-cmd-1", VesselID: v.ID, Action: "resetPassword", Username: "second-officer",
		TemporaryPassword: "correct-horse-battery-staple", IssuedBy: "admin", IssuedAt: now,
	}
	if err := st.QueueUserCommand(ctx, cmd); err != nil {
		t.Fatalf("QueueUserCommand: %v", err)
	}

	since, err := st.ListUserCommandsSince(ctx, v.ID, 0)
	if err != nil {
		t.Fatalf("ListUserCommandsSince: %v", err)
	}
	if len(since) != 1 || since[0].Command.TemporaryPassword != "correct-horse-battery-staple" {
		t.Fatalf("since = %+v, want the queued command with its plaintext password intact", since)
	}

	if err := st.MarkUserCommandFetched(ctx, "user-cmd-1", now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkUserCommandFetched: %v", err)
	}

	// The office-facing status view never carries the temporary password
	// back regardless of storage state, but fetched_at is now stamped.
	list, err := st.ListUserCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListUserCommandsForVessel: %v", err)
	}
	if len(list) != 1 || list[0].FetchedAt == nil {
		t.Fatalf("list = %+v, want fetched_at set", list)
	}

	// Crucially, the plaintext must SURVIVE fetch: a lost PullInbox
	// response leaves the vessel's cursor un-advanced, so the same command
	// gets re-delivered and must still carry its password (the §3.1 bug
	// was wiping it here, breaking every re-delivery).
	since, err = st.ListUserCommandsSince(ctx, v.ID, 0)
	if err != nil {
		t.Fatalf("ListUserCommandsSince (after fetch): %v", err)
	}
	if len(since) != 1 || since[0].Command.TemporaryPassword != "correct-horse-battery-staple" {
		t.Fatalf("since (after fetch) = %+v, want password still intact for re-delivery", since)
	}

	// Only once the vessel confirms it applied the command is the
	// plaintext cleared from storage.
	appliedAt := now.Add(2 * time.Minute)
	if err := st.MarkUserCommandsApplied(ctx, v.ID, []string{"user-cmd-1"}, appliedAt); err != nil {
		t.Fatalf("MarkUserCommandsApplied: %v", err)
	}
	list, err = st.ListUserCommandsForVessel(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListUserCommandsForVessel (after apply): %v", err)
	}
	if len(list) != 1 || list[0].AppliedAt == nil {
		t.Fatalf("list = %+v, want applied_at set", list)
	}
	since, err = st.ListUserCommandsSince(ctx, v.ID, 0)
	if err != nil {
		t.Fatalf("ListUserCommandsSince (after apply): %v", err)
	}
	if len(since) != 1 || since[0].Command.TemporaryPassword != "" {
		t.Fatalf("since (after apply) = %+v, want password cleared once applied", since)
	}
}

func TestStore_ReplaceAndListVesselUsers(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	v := createTestVessel(t, st, 98)

	first := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	if err := st.ReplaceVesselUsers(ctx, v.ID, []VesselUser{
		{Username: "master", Role: "master", Active: true, CanSubmit: true, UpdatedAt: first},
		{Username: "second-officer", Role: "secondOfficer", Active: true, CanSubmit: false, UpdatedAt: first},
	}, first); err != nil {
		t.Fatalf("ReplaceVesselUsers: %v", err)
	}

	list, err := st.ListVesselUsers(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListVesselUsers: %v", err)
	}
	if len(list) != 2 || list[0].Username != "master" || list[1].Username != "second-officer" {
		t.Fatalf("list = %+v, want [master, second-officer]", list)
	}

	// A second report with a since-deleted account must not linger.
	second := first.Add(time.Hour)
	if err := st.ReplaceVesselUsers(ctx, v.ID, []VesselUser{
		{Username: "master", Role: "master", Active: true, CanSubmit: true, UpdatedAt: first},
	}, second); err != nil {
		t.Fatalf("ReplaceVesselUsers (second): %v", err)
	}
	list, err = st.ListVesselUsers(ctx, v.ID)
	if err != nil {
		t.Fatalf("ListVesselUsers (after second replace): %v", err)
	}
	if len(list) != 1 || list[0].Username != "master" {
		t.Fatalf("list = %+v, want only master (second-officer removed)", list)
	}
}
